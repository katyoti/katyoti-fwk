// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EVERY capabilities/ folder on disk must actually be LOADABLE from its pack.
//
// This guard exists because declaring a capability is not the same as shipping
// one, and the gap is invisible. Two packs proved it on the day the kind landed:
//
//   - ktw-agents' `//go:embed` directive is an explicit ALLOW-LIST
//     (`item_types edge-types node_types data-types`), so a new `capabilities/`
//     folder was simply not compiled in;
//   - a sidecar's root directive omitted `embed/cwm` entirely, AND the pack
//     wired no CWM type provider at all — despite shipping the only
//     capability-panel in the repo.
//
// Both built, both tested green, and both published nothing. The census caught
// them after the fact; this catches them at the pack.
func TestEveryShippedCapabilityFolderLoads(t *testing.T) {
	up := filepath.Join("..", "..", "..", "..", "..")
	dirs, err := filepath.Glob(filepath.Join(up, "app", "*", "embed", "cwm", "capabilities"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	// A floor: this guard is worthless if it finds nothing.
	if len(dirs) < 3 {
		t.Fatalf("found only %d capability folders — the glob is wrong, not the corpus", len(dirs))
	}

	total := 0
	for _, d := range dirs {
		pack := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(d))))
		all, _ := filepath.Glob(filepath.Join(d, "*.yaml"))
		var files []string // declarations: a `<name>.i18n.yaml` is one's translations
		for _, f := range all {
			if !strings.HasSuffix(f, ".i18n.yaml") {
				files = append(files, f)
			}
		}
		caps, err := LoadCapabilitiesLayered(os.DirFS(filepath.Dir(d)))
		if err != nil {
			t.Errorf("%s: load failed: %v", pack, err)
			continue
		}
		if len(caps) != len(files) {
			t.Errorf("%s: %d files on disk but %d loaded — a capability that does not "+
				"load is a declaration nothing can reach", pack, len(files), len(caps))
		}
		for _, c := range caps {
			if c.Name == "" {
				t.Errorf("%s: a capability loaded with no name", pack)
			}
			total++
		}
	}
	t.Logf("capabilities loadable across %d packs: %d", len(dirs), total)
}

// privatePanelOnly are the panel-only capabilities of the private build's packs.
var privatePanelOnly []string

// The shipped set, by name, so a capability silently disappearing is a failure
// rather than a smaller number nobody reads.
func TestShippedCapabilitySet(t *testing.T) {
	up := filepath.Join("..", "..", "..", "..", "..")
	dirs, _ := filepath.Glob(filepath.Join(up, "app", "*", "embed", "cwm", "capabilities"))
	// The packs extensions contribute themselves — CWM's core pack among them.
	more, _ := filepath.Glob(filepath.Join(up, "internal", "extensions", "*", "contrib", "embed", "cwm", "capabilities"))
	dirs = append(dirs, more...)
	got := map[string]api.Capability{}
	for _, d := range dirs {
		caps, err := LoadCapabilitiesLayered(os.DirFS(filepath.Dir(d)))
		if err != nil {
			t.Fatalf("load %s: %v", d, err)
		}
		for _, c := range caps {
			if _, dup := got[c.Name]; dup {
				t.Errorf("capability %q declared by two packs — the id is the storage "+
					"key, so a duplicate is a collision, not a merge", c.Name)
			}
			got[c.Name] = c
		}
	}

	// Every shipped object-panel, so a panel-only capability can be checked
	// against the panels that name it.
	panelDirs, _ := filepath.Glob(filepath.Join(up, "app", "*", "embed", "cwm", "object-panels"))
	namedByPanel := map[string]bool{}
	for _, d := range panelDirs {
		// The object pane's kind, read here only for the capabilities its
		// filters name (the pane decodes the whole panel).
		type panelFilter struct {
			ID     string `yaml:"-"`
			Filter struct {
				Capabilities []string `yaml:"capabilities"`
			} `yaml:"filter"`
		}
		panels, err := LoadNamedLayered("object-panels", "object-panel", func(p *panelFilter) *string { return &p.ID }, os.DirFS(filepath.Dir(d)))
		if err != nil {
			t.Fatalf("load %s: %v", d, err)
		}
		for _, p := range panels {
			for _, c := range p.Filter.Capabilities {
				namedByPanel[c] = true
			}
		}
	}

	// The 4 that carry fields, and the panel-only ones (the private build's packs
	// add theirs: capabilities_private_test.go). (What every object
	// has — name, parent, body — is the object core, not a capability.) The split matters:
	// panel-only capabilities are why `cap-<id>.dsl.yaml` could never have
	// described the set — they have no schema at all.
	withFields := []string{"want", "checkable", "module", "project"}
	panelOnly := append([]string{"module-health", "file-overview"}, privatePanelOnly...)

	// Retired: an object's created and changed times are derived (the
	// journal's Times, else git), and what it was generated from is its
	// derived-from edge — no pack ships a provenance capability.
	for _, n := range []string{"provenance"} {
		if _, ok := got[n]; ok {
			t.Errorf("capability %q is shipped; it was retired", n)
		}
	}

	for _, n := range withFields {
		c, ok := got[n]
		if !ok {
			t.Errorf("field-group capability %q is not declared", n)
			continue
		}
		if c.Schema == "" {
			t.Errorf("%q declares no schema but is a field-group capability", n)
		}
	}
	for _, n := range panelOnly {
		c, ok := got[n]
		if !ok {
			t.Errorf("panel-only capability %q is not declared", n)
			continue
		}
		// Since p7p the PANEL names the capability, not the other way round:
		// a panel-only capability no shipped panel's filter names shows
		// nothing anywhere.
		if !namedByPanel[n] {
			t.Errorf("%q is panel-only but no shipped object-panel's filter names it", n)
		}
		if c.Schema != "" {
			t.Errorf("%q declares a schema; it was recorded as panel-only", n)
		}
	}
}

// Closer-wins layering, so a workspace overlay can override a shipped
// capability — the precedence rule the model requires.
func TestCapabilitiesLayerCloserWins(t *testing.T) {
	floor := fstest.MapFS{
		"capabilities/checkable.yaml": &fstest.MapFile{Data: []byte("name: checkable\nlabel: floor\n")},
		"capabilities/want.yaml":      &fstest.MapFile{Data: []byte("name: want\nlabel: Want\n")},
	}
	overlay := fstest.MapFS{
		"capabilities/checkable.yaml": &fstest.MapFile{Data: []byte("name: checkable\nlabel: overlay\n")},
	}
	got, err := LoadCapabilitiesLayered(floor, overlay)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 after layering, got %d", len(got))
	}
	for _, c := range got {
		if c.Name == "checkable" && c.Label != "overlay" {
			t.Errorf("overlay did not win: %q", c.Label)
		}
	}
}

// A malformed file is skipped, not fatal; a nameless one is dropped rather than
// registered — the name IS the storage key, so one without it can match nothing.
func TestCapabilitiesSkipBadEntries(t *testing.T) {
	src := fstest.MapFS{
		"capabilities/good.yaml":     &fstest.MapFile{Data: []byte("name: good\n")},
		"capabilities/broken.yaml":   &fstest.MapFile{Data: []byte("name: [not a string\n")},
		"capabilities/nameless.yaml": &fstest.MapFile{Data: []byte("label: ghost\n")},
		"capabilities/_index.yaml":   &fstest.MapFile{Data: []byte("skipped by convention")},
	}
	got, err := LoadCapabilitiesLayered(src)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Name != "good" {
		t.Fatalf("want only [good], got %+v", got)
	}
}

// A capability contributing nothing is a TAG, not an error — it exists to be
// selected on.
func TestTagCapabilityIsLegitimate(t *testing.T) {
	src := fstest.MapFS{"capabilities/deprecated.yaml": &fstest.MapFile{Data: []byte("name: deprecated\n")}}
	got, err := LoadCapabilitiesLayered(src)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Name != "deprecated" {
		t.Fatalf("a capability with no slots should load as a tag, got %+v", got)
	}
}
