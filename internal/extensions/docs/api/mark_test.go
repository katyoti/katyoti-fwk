// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

func TestProductionMark(t *testing.T) {
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	off := false
	cases := []struct {
		name string
		cfg  ProductionMarkConfig
		doc  DocMark
		ver  string
		want string // "" = no mark
	}{
		{"on by default", ProductionMarkConfig{}, DocMark{}, "2026.10", "Une production ArchiFactory · 2026.10 · 11/10/2026"},
		{"a dev build names no version", ProductionMarkConfig{}, DocMark{}, "dev", "Une production ArchiFactory · 11/10/2026"},
		{"the universe customises it", ProductionMarkConfig{Text: "Produit par l'équipe", Classification: "Diffusion restreinte"}, DocMark{}, "2026.10", "Produit par l'équipe · 2026.10 · 11/10/2026 · Diffusion restreinte"},
		{"the universe switches it off", ProductionMarkConfig{Enabled: &off}, DocMark{}, "2026.10", ""},
		{"a document switches it back on", ProductionMarkConfig{Enabled: &off}, DocMark{Mark: "on"}, "2026.10", "Une production ArchiFactory · 2026.10 · 11/10/2026"},
		{"a document switches it off", ProductionMarkConfig{}, DocMark{Mark: "off"}, "2026.10", ""},
		{"a document's own text and classification", ProductionMarkConfig{Classification: "Interne"}, DocMark{Mark: "Étude Acme", Classification: "Confidentiel"}, "", "Étude Acme · 11/10/2026 · Confidentiel"},
		{"a draft deliverable says so", ProductionMarkConfig{}, DocMark{State: "draft"}, "2026.10", "Une production ArchiFactory · 2026.10 · 11/10/2026 · Brouillon"},
		{"a validated deliverable says nothing more", ProductionMarkConfig{}, DocMark{State: "validated"}, "2026.10", "Une production ArchiFactory · 2026.10 · 11/10/2026"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, ok := ResolveMark(c.cfg, c.doc, c.ver, now)
			if c.want == "" {
				if ok {
					t.Fatalf("want no mark, got %q", m.Line())
				}
				return
			}
			if !ok || m.Line() != c.want {
				t.Fatalf("want %q, got %q (ok=%v)", c.want, m.Line(), ok)
			}
		})
	}
}

func TestDocMarkOfFrontmatter(t *testing.T) {
	d := DocMarkOf(map[string]any{"mark": false, "classification": "C2", "state": "in-review"})
	if d.Mark != "off" || d.Classification != "C2" || d.State != "in-review" {
		t.Fatalf("got %+v", d)
	}
	if got := DocMarkOf(nil); got != (DocMark{}) {
		t.Fatalf("nil meta: got %+v", got)
	}
	if !strings.Contains(DefaultMarkText, "ArchiFactory") {
		t.Fatal("the default names ArchiFactory")
	}
}

func TestMarkProvenance(t *testing.T) {
	now := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	off := false
	study := DocMark{Origin: "m.ET-001", OriginVersion: "3"}
	cases := []struct {
		name    string
		cfg     ProductionMarkConfig
		doc     DocMark
		binary  []string
		visible bool
		gen     string
	}{
		{"the line on", ProductionMarkConfig{}, study, nil, true, "ArchiFactory"},
		{"the universe switches the line off", ProductionMarkConfig{Enabled: &off}, study, nil, false, "ArchiFactory"},
		{"the document switches the line off", ProductionMarkConfig{}, DocMark{Mark: "off", Origin: study.Origin, OriginVersion: study.OriginVersion}, nil, false, "ArchiFactory"},
		{"the sidecar is named", ProductionMarkConfig{}, study, []string{"ktw-acme"}, true, "ArchiFactory (ktw-acme)"},
		{"the universe names its product", ProductionMarkConfig{Generator: "KaTyoti", Enabled: &off}, study, []string{"ktw-docs"}, false, "KaTyoti (ktw-docs)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, ok := ResolveMark(c.cfg, c.doc, "2026.10", now, c.binary...)
			if ok != c.visible {
				t.Fatalf("visible line: want %v, got %v", c.visible, ok)
			}
			want := Provenance{Generator: c.gen, Origin: "m.ET-001", OriginVersion: "3", Date: "2026-10-13T09:30:00Z"}
			if m.Provenance != want {
				t.Fatalf("provenance: want %+v, got %+v", want, m.Provenance)
			}
		})
	}
	d := DocMarkOf(map[string]any{"origin": "m.ET-001", "origin_version": 3})
	if d.Origin != "m.ET-001" || d.OriginVersion != "3" {
		t.Fatalf("a document's origin passes through its metadata: got %+v", d)
	}
}

// In a universe installed from a solution, the mark names that
// solution and its version - in the line and the provenance; with none, the
// universe's setting and the binary's version apply as before.
func TestMarkNamesTheInstalledSolution(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	inst := &cenv.Installation{Name: "ArchiFactory", Solution: "archi-factory@2026.11"}
	cases := []struct {
		name, universe string
		inst           *cenv.Installation
		want           Solution
	}{
		{"the installation names it", "archi-factory@2026.11", inst, Solution{Name: "ArchiFactory", Version: "2026.11"}},
		{"another solution keeps its own name", "acme@1.004", inst, Solution{Name: "acme", Version: "1.004"}},
		{"no installation", "archi-factory@2026.11", nil, Solution{Name: "archi-factory", Version: "2026.11"}},
		{"a universe made by hand", "", inst, Solution{}},
	}
	for _, c := range cases {
		if got := SolutionOf(c.universe, c.inst); got != c.want {
			t.Errorf("%s: want %+v, got %+v", c.name, c.want, got)
		}
	}
	sol := SolutionOf("archi-factory@2026.11", inst)
	m, ok := ResolveMarkFor(ProductionMarkConfig{Generator: "Autre"}, sol, DocMark{}, "1.009", now, "ktw-example")
	if !ok || m.Line() != "Une production ArchiFactory · 2026.11 · 12/11/2026" {
		t.Fatalf("the line names the solution and its version: %q (ok=%v)", m.Line(), ok)
	}
	if m.Provenance.Generator != "ArchiFactory (ktw-example)" {
		t.Fatalf("the provenance names the solution: %q", m.Provenance.Generator)
	}
	if m, _ := ResolveMarkFor(ProductionMarkConfig{Text: "Produit par l'équipe"}, sol, DocMark{}, "1.009", now); m.Line() != "Produit par l'équipe · 2026.11 · 12/11/2026" {
		t.Fatalf("the universe's own text stays, the solution's version shown: %q", m.Line())
	}
	m, _ = ResolveMarkFor(ProductionMarkConfig{}, Solution{}, DocMark{}, "1.009", now)
	if m.Line() != DefaultMarkText+" · 1.009 · 12/11/2026" || m.Provenance.Generator != DefaultGenerator {
		t.Fatalf("no solution: the default and the binary's version: %q %q", m.Line(), m.Provenance.Generator)
	}
}

type centralUniverse struct{ dir string }

func (u centralUniverse) UniverseDir() string { return u.dir }

// TestMarkReadsTheUniverseDir: the solution is read from the universe's own
// config root - a central universe's, not the attached project's home
func TestMarkReadsTheUniverseDir(t *testing.T) {
	universe := filepath.Join(t.TempDir(), "universes", "archi", ".katyoti")
	if err := os.MkdirAll(filepath.Join(universe, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(universe, "config", "Univers.yaml"), []byte("name: archi\nsolution: archi-factory@2026.11\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := installedSolution(centralUniverse{universe}); got.Version != "2026.11" || got.Name == "" {
		t.Fatalf("the central universe's solution: %+v", got)
	}
	if got := installedSolution(centralUniverse{t.TempDir()}); got != (Solution{}) {
		t.Fatalf("a universe with no Univers.yaml names no solution: %+v", got)
	}
}
