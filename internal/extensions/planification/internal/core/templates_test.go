// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// requestOn is a request object of a kind and depth (no lifecycle).
func requestOn(t *testing.T, kind, depth string) cwmapi.Object {
	t.Helper()
	o := cwmapi.Object{ID: "billing.RQ-001", Type: "request"}
	if err := o.SetCapability(api.CapRequest, api.RequestFields{Kind: kind, Depth: depth}); err != nil {
		t.Fatal(err)
	}
	return o
}

// A request's template is chosen by its kind, or by the type and lifecycle state
// of the object it is on, at its depth — falling back to the full template when
// its kind has no quick one.
func TestATemplateIsChosenByKindTypeStateAndDepth(t *testing.T) {
	recipes, err := loadRecipes()
	if err != nil {
		t.Fatal(err)
	}
	pick := func(o cwmapi.Object) string {
		t.Helper()
		r, err := selectRecipe(recipes, o)
		if err != nil {
			t.Fatal(err)
		}
		return r.Name
	}
	for _, c := range []struct{ kind, depth, want string }{
		{api.KindBuild, "", "build"},
		{api.KindBuild, api.DepthQuick, "build-quick"},
		{api.KindFix, api.DepthQuick, "fix-quick"},
		{api.KindReview, api.DepthQuick, "review"}, // no quick review: the full one
		{api.KindDiscover, "", "discover"},
		{"", "", "build"},
	} {
		if got := pick(requestOn(t, c.kind, c.depth)); got != c.want {
			t.Errorf("kind %q depth %q: %s, want %s", c.kind, c.depth, got, c.want)
		}
	}
	feature := cwmapi.Object{ID: "billing.F-001", Type: "feature"}
	_ = feature.SetCapability(api.CapRequest, api.RequestFields{Kind: api.KindBuild})
	_ = feature.SetCapability("lifecycle", map[string]any{"state": "accepted"})
	if got := pick(feature); got != "feature-specify" {
		t.Errorf("a feature accepted: %s, want feature-specify", got)
	}
}

// A module overrides a template by name and adds its own.
func TestAModuleOverridesATemplate(t *testing.T) {
	base, err := loadRecipes()
	if err != nil {
		t.Fatal(err)
	}
	own, err := readRecipes(fstest.MapFS{
		"build.yaml": {Data: []byte("name: build\nkind: build\ntransition: done\nsteps:\n  - {name: implement, role: coder}\n  - {name: security, role: reviewer, after: [implement]}\n")},
		"audit.yaml": {Data: []byte("name: audit\nkind: review\ndepth: quick\ntransition: done\nsteps:\n  - {name: scan, role: reviewer}\n")},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	merged := mergeTemplates(base, own)
	r, err := selectRecipe(merged, requestOn(t, api.KindBuild, ""))
	if err != nil || len(r.Steps) != 2 || r.Steps[1].Name != "security" {
		t.Fatalf("build in the module: %+v, %v; want the module's own (implement, security)", r.Steps, err)
	}
	if r, _ := selectRecipe(merged, requestOn(t, api.KindReview, api.DepthQuick)); r.Name != "audit" {
		t.Errorf("a quick review in the module: %s, want the module's audit", r.Name)
	}
	if len(merged) != len(base)+1 {
		t.Errorf("merged %d templates, want %d (one replaced, one added)", len(merged), len(base)+1)
	}
}

// A template's skill is a skill URI and its depth full or quick; every shipped
// step says what it does.
func TestTemplatesAreChecked(t *testing.T) {
	for _, bad := range []string{
		"name: x\nkind: build\ntransition: done\nsteps:\n  - {name: a, role: coder, skill: not-a-uri}\n",
		"name: x\nkind: build\ndepth: deep\ntransition: done\nsteps:\n  - {name: a, role: coder}\n",
	} {
		if _, err := readRecipes(fstest.MapFS{"x.yaml": {Data: []byte(bad)}}, "."); err == nil {
			t.Errorf("accepted:\n%s", bad)
		}
	}
	recipes, err := loadRecipes()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recipes {
		for _, st := range r.Steps {
			if strings.TrimSpace(st.Instructions) == "" {
				t.Errorf("%s/%s says nothing of what it does (instructions)", r.Name, st.Name)
			}
		}
	}
}
