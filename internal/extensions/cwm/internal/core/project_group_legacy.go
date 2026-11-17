// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// AN OLD PROJECT GROUP, READ ONLY TO BE JOINED (:
// a project never holds projects). No pack ships the project-group type or
// its capability any more; a repository written before data version 41 may
// still have one at its root - its capability's field-group naming the folder
// of its embedded child projects and its links. `cwm migrate` joins it into
// one project (project_group_merge.go); this is the one reader of that old
// shape, and nothing else reads it.

// legacyCapProjectGroup is the retired capability an old group's root carries.
const legacyCapProjectGroup = "project-group"

// legacyDefaultProjectsDir is an old group's projects folder when it named none.
const legacyDefaultProjectsDir = "projects"

// legacyGroup is the retired capability's field-group.
type legacyGroup struct {
	ProjectsDir string            `json:"projects_dir,omitempty" yaml:"projects_dir,omitempty"`
	Links       []api.ProjectLink `json:"links,omitempty" yaml:"links,omitempty"`
}

// legacyChild is an embedded child of an old group: its folder, relative to
// the group's root and slash-separated.
type legacyChild struct {
	ID  string
	Dir string
}

// legacyGroupOf reads an old group's field-group from its root project
// object; ok is false when it carries none.
func legacyGroupOf(o api.Object) (legacyGroup, bool, error) {
	return api.DecodeCapability[legacyGroup](o, legacyCapProjectGroup)
}

// legacyProjectsDir is an old group's projects folder, relative to its root -
// projects when it names none; a folder not under the root is refused.
func legacyProjectsDir(g legacyGroup) (string, error) {
	dir := g.ProjectsDir
	if dir == "" {
		dir = legacyDefaultProjectsDir
	}
	clean := path.Clean(filepath.ToSlash(dir))
	if path.IsAbs(clean) || filepath.IsAbs(dir) || clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return "", fmt.Errorf("%w: project group: projects folder %q is not a folder under the group", api.ErrInvalidInput, dir)
	}
	return clean, nil
}

// legacyEmbeddedChildren lists an old group's embedded children: every
// subfolder of its projects folder holding a project object, by id. A child
// holding a Git repository of its own is refused: no repository sits inside
// another.
func legacyEmbeddedChildren(groupRoot string, g legacyGroup) ([]legacyChild, error) {
	clean, err := legacyProjectsDir(g)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(groupRoot, filepath.FromSlash(clean)))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("project group: %w", err)
	}
	var out []legacyChild
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join(groupRoot, filepath.FromSlash(clean), e.Name())
		if _, err := os.Stat(filepath.Join(root, projectfile.FileName)); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			return nil, fmt.Errorf("%w: project group: child %s holds a Git repository of its own inside the group (%s) - link it to the universe as a project of its own and remove it from the group before the join", api.ErrInvalidInput, e.Name(), root)
		}
		f, err := projectfile.Read(root)
		if err != nil {
			return nil, fmt.Errorf("project group: child %s: %w", e.Name(), err)
		}
		out = append(out, legacyChild{ID: f.ID, Dir: path.Join(clean, e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
