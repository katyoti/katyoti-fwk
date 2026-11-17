// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
)

// pb builds a baseline PersonaBody.
func pb(name, body string, skills, nodes []string, cfg map[string]any) mbapi.PersonaBody {
	out := mbapi.PersonaBody{
		TaskBody: mbapi.TaskBody{Name: name, Body: body, Types: nodes},
		Skills:   skills,
	}
	if cfg != nil {
		raw, _ := json.Marshal(cfg)
		out.Config = raw
	}
	return out
}

// aug builds an augmentation PersonaBody for base.
func aug(base, body string, skills, nodes []string, cfg map[string]any) mbapi.PersonaBody {
	out := pb(base, body, skills, nodes, cfg)
	out.Augments = base
	return out
}

func findByName(bodies []mbapi.PersonaBody, name string) *mbapi.PersonaBody {
	for i := range bodies {
		if bodies[i].Name == name {
			return &bodies[i]
		}
	}
	return nil
}

func TestMerge_BaseOnly_Unchanged(t *testing.T) {
	in := []mbapi.PersonaBody{pb("coder", "base body", []string{"s1"}, []string{"module"}, nil)}
	out := mergeAugmentations(in)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].Body != "base body" {
		t.Errorf("body mutated: %q", out[0].Body)
	}
}

func TestMerge_BasePlusOneAug(t *testing.T) {
	in := []mbapi.PersonaBody{
		pb("coder", "universal role", []string{"spec"}, []string{"module"}, map[string]any{"max_turns": 10}),
		aug("coder", "Go specifics", []string{"go-uc"}, []string{"user-story"}, map[string]any{"profileName": "coding"}),
	}
	out := mergeAugmentations(in)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1 (aug folded in)", len(out))
	}
	c := out[0]
	if !strings.Contains(c.Body, "universal role") || !strings.Contains(c.Body, "Go specifics") {
		t.Errorf("body missing a part: %q", c.Body)
	}
	if !strings.Contains(c.Body, "\n\n---\n\n") {
		t.Errorf("body missing separator: %q", c.Body)
	}
	if got := strings.Index(c.Body, "universal role"); got > strings.Index(c.Body, "Go specifics") {
		t.Error("baseline must precede augmentation")
	}
	if want := []string{"spec", "go-uc"}; !equalStrs(c.Skills, want) {
		t.Errorf("skills = %v, want %v", c.Skills, want)
	}
	if want := []string{"module", "user-story"}; !equalStrs(c.Types, want) {
		t.Errorf("allowed = %v, want %v", c.Types, want)
	}
	var cfg map[string]any
	_ = json.Unmarshal(c.Config, &cfg)
	if cfg["max_turns"] == nil {
		t.Error("baseline config key max_turns lost")
	}
	if cfg["profileName"] != "coding" {
		t.Errorf("augmentation config key not merged: %v", cfg["profileName"])
	}
}

func TestMerge_TwoAugs_UnionDedup_Deterministic(t *testing.T) {
	in := []mbapi.PersonaBody{
		pb("coder", "base", []string{"s1"}, []string{"module"}, nil),
		aug("coder", "zeta", []string{"s1", "s2"}, nil, nil), // name suffix forces order
		aug("coder", "alpha", []string{"s3"}, []string{"module", "task"}, nil),
	}
	// Give the augs distinct Names so the sort is observable; both
	// still Augment "coder".
	in[1].Name = "coder-z"
	in[2].Name = "coder-a"

	out := mergeAugmentations(in)
	c := findByName(out, "coder")
	if c == nil {
		t.Fatal("coder missing")
	}
	// Skills union dedups s1; order = base, then sorted-by-name augs
	// (coder-a "s3" before coder-z "s2").
	if want := []string{"s1", "s3", "s2"}; !equalStrs(c.Skills, want) {
		t.Errorf("skills = %v, want %v (sorted apply order)", c.Skills, want)
	}
	if want := []string{"module", "task"}; !equalStrs(c.Types, want) {
		t.Errorf("allowed = %v, want %v", c.Types, want)
	}
	// alpha (coder-a) applied before zeta (coder-z).
	if strings.Index(c.Body, "alpha") > strings.Index(c.Body, "zeta") {
		t.Errorf("apply order not sorted by name: %q", c.Body)
	}
}

func TestMerge_OrphanAug_Dropped(t *testing.T) {
	in := []mbapi.PersonaBody{
		pb("coder", "base", nil, nil, nil),
		aug("ghost", "no base exists", nil, nil, nil),
	}
	out := mergeAugmentations(in)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1 (orphan dropped)", len(out))
	}
	if findByName(out, "ghost") != nil {
		t.Error("orphan augmentation surfaced standalone")
	}
}

func TestMerge_ConfigPluginsUnion(t *testing.T) {
	in := []mbapi.PersonaBody{
		pb("coder", "base", nil, nil, map[string]any{"plugins": []any{"cwm", "augmented"}}),
		aug("coder", "go", nil, nil, map[string]any{"plugins": []any{"cwm", "gocode-next-step"}}),
	}
	out := mergeAugmentations(in)
	var cfg map[string]any
	_ = json.Unmarshal(out[0].Config, &cfg)
	plugins, _ := cfg["plugins"].([]any)
	got := make([]string, len(plugins))
	for i, p := range plugins {
		got[i] = p.(string)
	}
	if want := []string{"cwm", "augmented", "gocode-next-step"}; !equalStrs(got, want) {
		t.Errorf("plugins = %v, want %v (union, dedup cwm)", got, want)
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A scoped layer merges only for the work it concerns: a Go layer for work
// touching Go files, not for docs work; with no context — the work unknown —
// it merges as every layer always did. An unscoped layer merges always.
func TestMerge_AScopedLayerMergesOnlyForItsWork(t *testing.T) {
	goLayer := aug("coder", "Go specifics", nil, nil, nil)
	goLayer.AppliesTo = &mbapi.PersonaScope{Files: []string{"**/*.go"}}
	billingOnly := aug("coder", "Billing conventions", nil, nil, nil)
	billingOnly.AppliesTo = &mbapi.PersonaScope{Modules: []string{"billing"}}
	always := aug("coder", "House style", nil, nil, nil)
	in := func() []mbapi.PersonaBody {
		return []mbapi.PersonaBody{pb("coder", "universal role", nil, nil, nil), goLayer, billingOnly, always}
	}
	body := func(sc *api.SpawnContext) string { return findByName(mergeAugmentationsFor(in(), sc), "coder").Body }

	goWork := body(&api.SpawnContext{Files: []string{"internal/billing/invoice.go"}, Module: "shipping"})
	if !strings.Contains(goWork, "Go specifics") || strings.Contains(goWork, "Billing conventions") || !strings.Contains(goWork, "House style") {
		t.Errorf("Go work in shipping = %q; want the Go layer and the house style, not billing's", goWork)
	}
	docs := body(&api.SpawnContext{Files: []string{"docs/guide.md"}, Module: "billing"})
	if strings.Contains(docs, "Go specifics") || !strings.Contains(docs, "Billing conventions") {
		t.Errorf("docs work in billing = %q; want billing's layer, not the Go one", docs)
	}
	if unknown := body(nil); !strings.Contains(unknown, "Go specifics") || !strings.Contains(unknown, "Billing conventions") {
		t.Errorf("no context = %q; want every layer (the work is unknown)", unknown)
	}
}
