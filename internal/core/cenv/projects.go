// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// AttachedProjectsFile is the attached-projects registry of a universe,
// `<config root>/projects.yaml`: the project folders whose content the
// universe serves. Written by the orchestrator (attach-project /
// detach-project); read here, at the bootstrap layer, like universes.yaml.
const AttachedProjectsFile = "projects.yaml"

// AttachedProjects returns the project folders attached to the universe whose
// config root is `configRoot` (the `.katyoti` folder), in registry order.
// Nil when the file is absent or unreadable — a universe with no attached
// project is the ordinary self-contained layout, not a fault.
//
// ONE READER. The orchestrator writes this file and used to be its only
// reader, so the CWM object scan never looked at it: it derived the project
// from the PARENT of the config root. That is right for a universe that lives
// in a repository checkout and wrong for one that lives under
// `<GlobalHomeDir>/universes/<name>/` with a repository attached — the scan
// then walked an empty `modules/` beside the config while the attached
// project's corpus sat unread. Anything that needs "the project this universe
// serves" reads it from here.
func AttachedProjects(configRoot string) []string {
	if configRoot == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(configRoot, AttachedProjectsFile))
	if err != nil {
		return nil
	}
	var pf struct {
		Projects []AttachedEntry `yaml:"projects"`
	}
	if yaml.Unmarshal(data, &pf) != nil {
		return nil
	}
	out := make([]string, 0, len(pf.Projects))
	for _, e := range pf.Projects {
		out = append(out, e.Path)
	}
	return out
}

// AttachedEntry is one attached project: its folder, and whether the
// universe trusts it — what the universe's operator says, never the project
// itself. A trusted project's overrides of EXECUTABLE content (a task) are
// served; an untrusted one's are refused and reported. A bare path is an
// untrusted entry, and is written back as one.
type AttachedEntry struct {
	Path    string
	Trusted bool
}

// UnmarshalYAML reads a bare path or a {path, trusted} mapping.
func (e *AttachedEntry) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		e.Path, e.Trusted = n.Value, false
		return nil
	}
	var m struct {
		Path    string `yaml:"path"`
		Trusted bool   `yaml:"trusted"`
	}
	if err := n.Decode(&m); err != nil {
		return err
	}
	e.Path, e.Trusted = m.Path, m.Trusted
	return nil
}

// MarshalYAML writes an untrusted entry as its bare path.
func (e AttachedEntry) MarshalYAML() (any, error) {
	if !e.Trusted {
		return e.Path, nil
	}
	return struct {
		Path    string `yaml:"path"`
		Trusted bool   `yaml:"trusted"`
	}{e.Path, true}, nil
}

// ProjectTrusted reports whether the universe whose config root is
// configRoot trusts the attached project at path (compared as the file
// system does); false when it does not attach it.
func ProjectTrusted(configRoot, path string) bool {
	if configRoot == "" || path == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(configRoot, AttachedProjectsFile))
	if err != nil {
		return false
	}
	var pf struct {
		Projects []AttachedEntry `yaml:"projects"`
	}
	if yaml.Unmarshal(data, &pf) != nil {
		return false
	}
	want := filepath.Clean(path)
	for _, e := range pf.Projects {
		got := filepath.Clean(e.Path)
		if got == want || (runtime.GOOS == "windows" && strings.EqualFold(got, want)) {
			return e.Trusted
		}
	}
	return false
}

// AttachedProject is the project the universe whose config root is
// `configRoot` serves: the first attached project folder that exists on disk;
// "" when it attaches none.
func AttachedProject(configRoot string) string {
	for _, p := range AttachedProjects(configRoot) {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}
