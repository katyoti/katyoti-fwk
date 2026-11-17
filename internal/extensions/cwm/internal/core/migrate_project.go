// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	pcapi "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	pclocal "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/local"
)

// PROJECT-LEVEL STEPS. A step whose change is not one of the project's
// objects - a tree of the project's files that retires into objects - works
// in two times: it reads the project and says what it would do (a dry run
// stops there, writing nothing), then, once every step has planned and the
// changed objects are written, its Apply writes through the service's own
// write path (ObjectCreate, ObjectFileWrite: checked and indexed like any
// write) before the project's data version is stamped. A step that cannot
// do all it must fails while planning: nothing is written, the data version
// stays.

// ProjectMigration is what a project-level step is given.
type ProjectMigration struct {
	// Root is the project's root folder.
	Root string
	// BackupDir is where the step keeps a copy of what it removes (under the
	// universe's data folder); empty: the step may remove nothing.
	BackupDir string
	// Kinds are the contribution kinds the project's files are read against.
	Kinds func() (contribapi.Kinds, error)
}

// ProjectChange is what a project-level step does, or would do.
type ProjectChange struct {
	// Lines say each thing it does, one per line.
	Lines []string
	// Changed and Removed are the project files it writes and removes, by
	// project-relative path.
	Changed, Removed []string
	// Apply does it, through cwm - the service's write path. nil: nothing to do.
	Apply func(ctx stdctx.Context, cwm api.CWMService) error
}

// retireOverwritesTrees converts the project's `overwrites/<provider>/`
// trees into override objects (decision
// superseding) and removes them. All or
// nothing: one file of any tree that maps to no layer of an override object
// fails the step, naming every such file, and nothing is written for any
// provider. Each tree is copied to the backup folder, the copy checked, and
// only then removed. No tree: nothing to do.
func retireOverwritesTrees(ctx stdctx.Context, pm ProjectMigration) (ProjectChange, error) {
	var out ProjectChange
	f, err := projectfile.Read(pm.Root)
	if err != nil {
		return out, err
	}
	base := filepath.Dir(f.OverwritesDir(pm.Root, "x"))
	entries, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	rel := func(p string) string {
		r, err := filepath.Rel(pm.Root, p)
		if err != nil {
			return filepath.ToSlash(p)
		}
		return filepath.ToSlash(r)
	}
	kindsOf := pm.Kinds
	if kindsOf == nil {
		kindsOf = builtin.Kinds
	}
	kinds, err := kindsOf()
	if err != nil {
		return out, fmt.Errorf("the contribution kinds: %w", err)
	}
	type tree struct {
		dir  string
		conv pclocal.OverwritesConversion
	}
	var trees []tree
	var unmapped []string
	for _, e := range entries {
		p := filepath.Join(base, e.Name())
		if !e.IsDir() {
			unmapped = append(unmapped, rel(p)+": not in a provider's tree")
			continue
		}
		conv, err := pclocal.ConvertOverwrites(ctx, p, e.Name(), kinds, "overrides/"+e.Name())
		if err != nil {
			return out, err
		}
		for _, u := range conv.Unmapped {
			unmapped = append(unmapped, rel(p)+"/"+u)
		}
		trees = append(trees, tree{dir: p, conv: conv})
	}
	if len(unmapped) > 0 {
		return out, fmt.Errorf("%d file(s) of the project's overwrites trees map to no layer of an override object - nothing is written and every tree is kept; move or remove each, then migrate again:\n  %s",
			len(unmapped), strings.Join(unmapped, "\n  "))
	}
	if len(trees) == 0 {
		return out, nil
	}
	if pm.BackupDir == "" {
		return out, fmt.Errorf("%w: no backup folder to keep the overwrites trees in before removing them", api.ErrInvalidInput)
	}
	module := pcapi.ModuleID
	modulePath := filepath.ToSlash(filepath.Join(f.ModulesDir, module))
	backup := filepath.Join(pm.BackupDir, "overwrites-"+time.Now().UTC().Format("20060102T150405Z"))
	for _, t := range trees {
		for _, o := range t.conv.Overrides {
			out.Lines = append(out.Lines, fmt.Sprintf("override object %s.%s in %s overrides %s (layers %s)",
				module, pcapi.OverrideObjectID(t.conv.Provider, o.Kind, o.ID), module, o.Overrides, strings.Join(layerNames(o.Layers), ", ")))
			for file := range o.Files {
				out.Changed = append(out.Changed, modulePath+"/"+file)
			}
		}
		out.Lines = append(out.Lines, t.conv.Widenings...)
		out.Lines = append(out.Lines, fmt.Sprintf("tree %s removed, kept in %s", rel(t.dir), filepath.Join(backup, filepath.Base(t.dir))))
		out.Removed = append(out.Removed, rel(t.dir))
	}
	sort.Strings(out.Changed)
	out.Apply = func(ctx stdctx.Context, cwm api.CWMService) error {
		if err := ensureContributionsModule(ctx, cwm); err != nil {
			return err
		}
		for _, t := range trees {
			for _, o := range t.conv.Overrides {
				if err := writeOverrideObject(ctx, cwm, module, t.conv.Provider, o); err != nil {
					return err
				}
			}
		}
		for _, t := range trees {
			kept := filepath.Join(backup, filepath.Base(t.dir))
			if err := backUpTree(t.dir, kept); err != nil {
				return fmt.Errorf("back up %s: %w (the tree is kept)", rel(t.dir), err)
			}
			if err := sameTree(t.dir, kept); err != nil {
				return fmt.Errorf("back up %s: the copy in %s differs: %w (the tree is kept)", rel(t.dir), kept, err)
			}
			if err := os.RemoveAll(t.dir); err != nil {
				return fmt.Errorf("remove %s: %w (its copy is in %s)", rel(t.dir), err, kept)
			}
		}
		if left, err := os.ReadDir(base); err == nil && len(left) == 0 {
			_ = os.Remove(base)
		}
		return nil
	}
	return out, nil
}

// ensureContributionsModule gives the project its sys-contributions module
// when it has none, as Start contributing does.
func ensureContributionsModule(ctx stdctx.Context, cwm api.CWMService) error {
	_, err := cwm.ObjectGet(ctx, api.ObjectGetInput{Module: pcapi.ModuleID, ID: pcapi.ModuleID})
	if err == nil {
		return nil
	}
	if !errors.Is(err, api.ErrNotFound) {
		return err
	}
	mod := api.Object{ID: pcapi.ModuleID, Type: api.SystemModuleType, Name: pcapi.ModuleID,
		Description:  "The project's own contributions: its skills, tasks, kinds and packs, published to its universe under its provider token.",
		Capabilities: map[string]map[string]any{api.CapSystemModule: {"operates": "contributions"}}}
	if _, err := cwm.ObjectCreate(ctx, api.ObjectCreateInput{Module: pcapi.ModuleID, Object: mod}); err != nil && !errors.Is(err, api.ErrAlreadyExists) {
		return fmt.Errorf("create %s: %w", pcapi.ModuleID, err)
	}
	return nil
}

// writeOverrideObject writes one override object and its layer files. An
// object already there (a run cut short after writing it) is kept and its
// files written again: the conversion is the same.
func writeOverrideObject(ctx stdctx.Context, cwm api.CWMService, module, provider string, o pcapi.ConvertedOverride) error {
	obj := pcapi.OverrideObject(provider, o)
	created, err := cwm.ObjectCreate(ctx, api.ObjectCreateInput{Module: module, Object: obj})
	id := module + "." + obj.ID
	switch {
	case err == nil:
		id = created.Object.ID
	case errors.Is(err, api.ErrAlreadyExists):
	default:
		return fmt.Errorf("override object of %s: %w", o.Overrides, err)
	}
	for _, file := range sortedFileKeys(o.Files) {
		if _, err := cwm.ObjectFileWrite(ctx, api.ObjectFileWriteInput{ID: id, Path: file, Content: o.Files[file]}); err != nil {
			return fmt.Errorf("override object of %s: file %s: %w", o.Overrides, file, err)
		}
	}
	return nil
}

func layerNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedFileKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// backUpTree copies the folder src to dst, which it creates.
func backUpTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		r, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, r)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		o, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(o, in); err != nil {
			_ = o.Close()
			return err
		}
		return o.Close()
	})
}

// sameTree checks that dst holds every file of src, each of its size, and no
// other.
func sameTree(src, dst string) error {
	sizes := func(root string) (map[string]int64, error) {
		out := map[string]int64{}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			r, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(r)] = info.Size()
			return nil
		})
		return out, err
	}
	a, err := sizes(src)
	if err != nil {
		return err
	}
	b, err := sizes(dst)
	if err != nil {
		return err
	}
	if len(a) != len(b) {
		return fmt.Errorf("%d files, the copy %d", len(a), len(b))
	}
	for p, n := range a {
		if m, ok := b[p]; !ok || m != n {
			return fmt.Errorf("%s: %d bytes, the copy %d", p, n, m)
		}
	}
	return nil
}
