// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
)

// A TEST UNIVERSE ON REAL DATA: a case whose project is a copy
// of a live repository's data - the project object and the modules folder of
// its root and of each project folder below it - runs unreleased code on real
// objects through its own hub and store. The live repository is only ever
// read: the copy is the case's, and so is every write.

// seedLive replaces the case project's data with the live repository's: the
// project.cwm.yaml and the modules folder of its root and of every project
// folder below it (a folder holding a project object), taken afresh
// (what the case wrote there is gone, and so is a project folder the live
// repository no longer holds). Code, .katyoti/, .git/ and node_modules/ are
// not copied.
func (e *Engine) seedLive() error {
	src := e.source(e.Case.Project.Live)
	dst := e.RT.Project()
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return fmt.Errorf("project.live: %s is not a folder", src)
	}
	if within(src, dst) || within(dst, src) {
		return fmt.Errorf("project.live: %s and the case's project %s overlap - the live project must stay apart", src, dst)
	}
	f, err := projectfile.Read(src)
	if err != nil {
		return fmt.Errorf("project.live: %w", err)
	}
	if _, err := os.Stat(filepath.Join(src, f.ModulesDir)); err != nil {
		return fmt.Errorf("project.live: %s has no %s folder", src, f.ModulesDir)
	}
	live, err := projectFolders(src)
	if err != nil {
		return fmt.Errorf("project.live: %w", err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	// What an earlier seed copied goes first: the case's own project folders
	// and the live ones (a folder the case no longer holds removes nothing).
	held, err := projectFolders(dst)
	if err != nil {
		return err
	}
	for _, pf := range append(held, live...) {
		for _, p := range []string{projectfile.FileName, pf.modules} {
			if err := os.RemoveAll(filepath.Join(dst, pf.rel, p)); err != nil {
				return err
			}
		}
	}
	for _, pf := range live {
		from, to := filepath.Join(src, pf.rel), filepath.Join(dst, pf.rel)
		if _, err := os.Stat(filepath.Join(from, projectfile.FileName)); err == nil {
			if err := copyFile(filepath.Join(from, projectfile.FileName), filepath.Join(to, projectfile.FileName)); err != nil {
				return fmt.Errorf("project.live: %w", err)
			}
		}
		if st, err := os.Stat(filepath.Join(from, pf.modules)); err == nil && st.IsDir() {
			if err := copyTree(filepath.Join(from, pf.modules), filepath.Join(to, pf.modules)); err != nil {
				return fmt.Errorf("project.live: %w", err)
			}
		}
	}
	return nil
}

// projectFolder is a project folder of a repository: its path from the root
// ("." for the root) and its modules folder.
type projectFolder struct {
	rel, modules string
}

// projectFolders lists the root and every folder below it holding a project
// object, root first. The walk never enters a dot folder (.git, .katyoti),
// node_modules, nor a project's modules folder: projects live beside the code,
// never inside a module.
func projectFolders(root string) ([]projectFolder, error) {
	var out []projectFolder
	skip := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if rel != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || skip[rel]) {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(p, projectfile.FileName)); err != nil && rel != "." {
			return nil
		}
		f, err := projectfile.Read(p)
		if err != nil {
			return err
		}
		modules := filepath.FromSlash(f.ModulesDir)
		skip[filepath.Join(rel, modules)] = true
		out = append(out, projectFolder{rel: rel, modules: modules})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rel == "." && out[j].rel != "." })
	return out, nil
}

// RefreshProject stops the case's universe and takes the live project's data
// again: the step before trying a migration on today's objects. Checkpoints
// are kept; a git project commits the refresh.
func (e *Engine) RefreshProject(ctx context.Context) error {
	if e.Case.Project.Live == "" {
		return fmt.Errorf("case %s has no project.live to refresh from", e.Case.Name)
	}
	if err := e.Stop(ctx); err != nil {
		return err
	}
	if err := e.seedLive(); err != nil {
		return err
	}
	if e.Case.Project.Git {
		if err := gitIn(e.RT.Project(), "add", "-A"); err != nil {
			return err
		}
		if err := gitIn(e.RT.Project(), "commit", "-q", "--allow-empty", "-m", "refresh-project: the live project's data"); err != nil {
			return err
		}
	}
	fmt.Fprintf(e.Out, "project refreshed from %s\n", e.source(e.Case.Project.Live))
	return nil
}
