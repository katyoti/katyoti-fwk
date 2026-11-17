// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	cwmmigrate "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/migrate"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// THE PROJECTS OF EARLIER INSTALLATIONS. A new installation
// finds them through the registry of installations, each one's universes
// registry and each universe's projects registry, and imports the ones the
// person picks as copies in its projects folder. The original belongs to the
// earlier installation: it is only ever read.

// migrateCopy brings a copied project to this release's data version. A
// variable so a test makes it fail.
var migrateCopy = func(ctx context.Context, dir string) (from, to int, err error) {
	out, err := cwmmigrate.Folder(ctx, dir)
	return out.From, out.To, err
}

// knownInstallations is the registry of installations. A variable so a test
// gives its own.
var knownInstallations = controlplane.KnownInstallations

// EarlierProjects lists the projects the other installations serve.
func (s *service) EarlierProjects(_ context.Context) (mktapi.EarlierProjectsOutput, error) {
	out := mktapi.EarlierProjectsOutput{Projects: []mktapi.EarlierProject{}}
	known, err := knownInstallations()
	if err != nil {
		return out, err
	}
	current := currentInstallation()
	seen := map[string]bool{}
	for _, k := range known {
		if current != nil && sameFolder(k.Folder, current.Dir()) {
			continue
		}
		earlier := installationAt(k.Folder, k.Name)
		universes, err := universesOf(earlier.ConfigRoot())
		if err != nil {
			out.Skipped = append(out.Skipped, fmt.Sprintf("%s: %v", k.Name, err))
			continue
		}
		for _, u := range universes {
			for _, p := range cenv.AttachedProjects(application.EnvOf(u.HomePath).HomeDir()) {
				f, err := readProject(p)
				if err != nil {
					continue // gone, or unreadable: nothing to import
				}
				key := strings.ToLower(filepath.Clean(p))
				if seen[key] {
					continue
				}
				seen[key] = true
				ep := mktapi.EarlierProject{Folder: p, Name: filepath.Base(filepath.Clean(p)),
					Installation: k.Name, Universe: u.Name, DataVersion: f.DataVersion, LastUsed: lastUsed(p)}
				if dst := projectDirIn(current, ep.Name); holdsFiles(dst) {
					ep.ImportedTo = dst
				}
				out.Projects = append(out.Projects, ep)
			}
		}
	}
	return out, nil
}

// ImportProject copies an earlier project into the projects folder, migrates
// the copy, and attaches it when a universe is named.
func (s *service) ImportProject(ctx context.Context, in mktapi.ImportProjectInput) (mktapi.ImportProjectOutput, error) {
	var out mktapi.ImportProjectOutput
	src := filepath.Clean(strings.TrimSpace(in.Folder))
	f, err := readProject(src)
	if err != nil {
		return out, fmt.Errorf("%s is not a project to import: %w", in.Folder, err)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = filepath.Base(src)
	}
	if err := oneFolderName(name); err != nil {
		return out, err
	}
	inst := currentInstallation()
	dst := projectDirIn(inst, name)
	if err := inst.CheckProjectDir(dst); err != nil {
		return out, err
	}
	if sameFolder(src, dst) {
		return out, fmt.Errorf("%s is already in the projects folder", src)
	}
	if holdsFiles(dst) {
		return out, fmt.Errorf("%s already holds files: an import never overwrites a project - give it another name", dst)
	}
	if err := copyTree(src, dst); err != nil {
		_ = os.RemoveAll(dst)
		return out, fmt.Errorf("copy %s: %w", src, err)
	}
	from, to, err := migrateCopy(ctx, dst)
	if err != nil {
		_ = os.RemoveAll(dst)
		return out, fmt.Errorf("migrate the copy of %s (data version %d): %w - the copy is removed, the original is untouched", src, f.DataVersion, err)
	}
	out = mktapi.ImportProjectOutput{Project: dst, From: from, To: to}
	if u := strings.TrimSpace(in.Universe); u != "" {
		home := cenv.ResolveUniverseHome("katyoti", u)
		if home == "" {
			return out, fmt.Errorf("the copy is in %s, but no universe %q to attach it to", dst, u)
		}
		if err := attachProject(ctx, home, u, dst); err != nil {
			return out, fmt.Errorf("the copy is in %s: %w", dst, err)
		}
		out.Universe = u
	}
	return out, nil
}

// oneFolderName refuses a project name that is not one folder name: a
// separator, `.`, `..` or a volume would put the copy outside the projects
// folder.
func oneFolderName(name string) error {
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return fmt.Errorf("%q is not a project name: give one folder name, without a separator, . or ..", name)
	}
	return nil
}

// readProject reads the project object of a folder that exists.
func readProject(dir string) (projectfile.File, error) {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return projectfile.File{}, fmt.Errorf("no folder %s", dir)
	}
	return projectfile.Read(dir)
}

// projectDirIn is where a project named name goes in the installation's
// projects folder, which is never inside the installation.
func projectDirIn(inst *cenv.Installation, name string) string {
	dir, _ := inst.ProjectsFolder()
	return filepath.Join(dir, name)
}

// installationAt is the installation in folder: its katyoti.yml, or the
// default layout when it has none.
func installationAt(folder, name string) *cenv.Installation {
	if inst, err := cenv.LoadInstallation(filepath.Join(folder, cenv.InstallationFile)); err == nil {
		return inst
	}
	return cenv.NewInstallation(folder, name, "")
}

type registeredUniverse struct {
	Name     string `yaml:"name"`
	HomePath string `yaml:"home_path"`
}

// universesOf reads the universes registry of a config root (missing: none).
func universesOf(configRoot string) ([]registeredUniverse, error) {
	data, err := os.ReadFile(filepath.Join(configRoot, "universes.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r struct {
		Universes []registeredUniverse `yaml:"universes"`
	}
	if err := config.DecodeBytes(data, marshal.YAML, &r); err != nil {
		return nil, err
	}
	return r.Universes, nil
}

// lastUsed is when KaTyoti last changed the project at dir, RFC 3339: the
// newest change in its store (.katyoti, written whenever a hub opens it), else
// the folder's own; empty when neither can be read. Only the store is walked,
// never the project's own files.
func lastUsed(dir string) string {
	var newest time.Time
	_ = filepath.WalkDir(application.EnvOf(dir).HomeDir(), func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if newest.IsZero() {
		if fi, err := os.Stat(dir); err == nil {
			newest = fi.ModTime()
		}
	}
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC3339)
}

// holdsFiles: dir exists and is not empty.
func holdsFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// copyTree copies src into dst, which it creates; src is only read. Links are
// not followed.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(p, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
