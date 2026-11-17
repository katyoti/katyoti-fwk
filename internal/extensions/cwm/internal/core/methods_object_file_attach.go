// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ATTACHING A FOLDER. A file is linked to an object by an edge whose kind
// targets a path (`attaches`), never by a file object: the edge is the only
// link. A folder that grows and shrinks (a study's sections, a deck's media)
// is kept in step by this verb rather than by hand-written edges: the object
// attaches exactly the files the patterns match, and the edges the patterns do
// not cover are not touched.

// ObjectFileAttach keeps an object's path edges in step with files of its
// module.
func (s *Service) ObjectFileAttach(ctx stdctx.Context, in api.ObjectFileAttachInput) (api.ObjectFileAttachOutput, error) {
	if err := s.refuseInChangeSet(ctx, "ObjectFileAttach"); err != nil {
		return api.ObjectFileAttachOutput{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = api.EdgeKindAttaches
	}
	if s.Storage == nil {
		return api.ObjectFileAttachOutput{}, fmt.Errorf("%w: ObjectFileAttach: no edge kinds are declared", api.ErrInvalidInput)
	}
	decl, ok := s.Storage.Types().EdgeType(api.EdgeType(kind))
	if !ok {
		return api.ObjectFileAttachOutput{}, fmt.Errorf("%w: ObjectFileAttach: edge kind %q is declared by no pack", api.ErrInvalidInput, kind)
	}
	if decl.TargetKind() != api.EdgeTargetPath {
		return api.ObjectFileAttachOutput{}, fmt.Errorf("%w: ObjectFileAttach: edge kind %q targets %s, not a path", api.ErrInvalidInput, kind, decl.TargetKind())
	}
	root, module, err := s.objectFileRoot(in.ID)
	if err != nil {
		return api.ObjectFileAttachOutput{}, err
	}
	patterns, err := attachPatterns(root, in.Patterns)
	if err != nil {
		return api.ObjectFileAttachOutput{}, err
	}
	covers := func(rel string) bool {
		for _, p := range patterns {
			if ok, _ := path.Match(p, rel); ok {
				return true
			}
		}
		return false
	}

	// The files the patterns match now.
	want := map[string]bool{}
	for _, rel := range filesUnder(root, patterns) {
		if covers(rel) {
			want[rel] = true
		}
	}

	got, err := s.ObjectGet(ctx, api.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.ObjectFileAttachOutput{}, err
	}
	obj := got.Object
	prefix := s.projectRelPath(module, "") + "/"
	out := api.ObjectFileAttachOutput{ID: obj.ID, Kind: kind, Added: []string{}, Removed: []string{}, Attached: []string{}}
	have := map[string]bool{}
	edges := make([]api.ObjectEdge, 0, len(obj.Edges)+len(want))
	for _, e := range obj.Edges {
		if e.Kind != kind {
			edges = append(edges, e)
			continue
		}
		rel, inModule := strings.CutPrefix(path.Clean(filepath.ToSlash(e.To)), prefix)
		if !inModule || !covers(rel) {
			edges = append(edges, e) // not this call's: another folder, another module's file
			continue
		}
		if !want[rel] || have[rel] {
			out.Removed = append(out.Removed, e.To) // its file is gone, or a duplicate
			continue
		}
		have[rel] = true
		edges = append(edges, e)
	}
	rels := make([]string, 0, len(want))
	for rel := range want {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		to := s.projectRelPath(module, rel)
		out.Attached = append(out.Attached, to)
		if !have[rel] {
			out.Added = append(out.Added, to)
			edges = append(edges, api.ObjectEdge{Kind: kind, To: to})
		}
	}
	sort.Strings(out.Removed)
	if len(out.Added) == 0 && len(out.Removed) == 0 || in.DryRun {
		return out, nil
	}
	obj.Edges = edges
	if _, err := s.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: obj, Revision: got.Revision}); err != nil {
		return api.ObjectFileAttachOutput{}, err
	}
	out.Changed = true
	return out, nil
}

// attachPatterns checks and normalises the globs: module-relative, no `..`,
// a valid path.Match pattern; a folder stands for the files directly in it.
func attachPatterns(root string, in []string) ([]string, error) {
	var out []string
	for _, raw := range in {
		p := strings.TrimSpace(filepath.ToSlash(raw))
		if p == "" {
			continue
		}
		if path.IsAbs(p) || filepath.IsAbs(raw) {
			return nil, fmt.Errorf("%w: pattern %q must be relative to the module folder", api.ErrInvalidInput, raw)
		}
		for _, seg := range strings.Split(p, "/") {
			if seg == ".." {
				return nil, fmt.Errorf("%w: pattern %q must not contain '..'", api.ErrInvalidInput, raw)
			}
		}
		p = strings.TrimSuffix(path.Clean(p), "/")
		if _, err := path.Match(p, ""); err != nil {
			return nil, fmt.Errorf("%w: pattern %q: %v", api.ErrInvalidInput, raw, err)
		}
		if !strings.ContainsAny(p, `*?[\`) {
			if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil && fi.IsDir() {
				p = path.Join(p, "*")
			}
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one pattern is required", api.ErrInvalidInput)
	}
	return out, nil
}

// filesUnder lists the module's files the patterns could match: the files of
// each pattern's folder when it is literal, else every file of the module.
// Object files and sidecar metadata are never listed (the byte API never
// reaches them, so they are never attached).
func filesUnder(root string, patterns []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(rel string) {
		if seen[rel] || isObjectFileName(rel) || strings.HasSuffix(rel, metaSuffix) {
			return
		}
		seen[rel] = true
		out = append(out, rel)
	}
	walkAll := false
	for _, p := range patterns {
		dir := path.Dir(p)
		if strings.ContainsAny(dir, `*?[\`) {
			walkAll = true
			break
		}
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				add(path.Join(dir, e.Name()))
			}
		}
	}
	if walkAll {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if rel, rerr := filepath.Rel(root, p); rerr == nil {
				add(filepath.ToSlash(rel))
			}
			return nil
		})
	}
	return out
}
