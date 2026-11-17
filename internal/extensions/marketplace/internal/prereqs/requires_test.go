// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// A solution's needs are its members' summed: each program, key and login
// once, at the highest version, required when any member requires it.
func TestSumRequires_TwoMembers(t *testing.T) {
	a := mktapi.NeedsOf("ktw-docs",
		[]publish.ToolRequirement{
			{Name: "Pandoc", Required: false, Version: "3.1", Recipes: []publish.InstallRecipe{{Platform: "windows", Kind: "winget", Package: "JohnMacFarlane.Pandoc"}}},
			{Name: "Typst", Required: false},
		},
		[]publish.APIKeyRequirement{{EnvName: "KROKI_TOKEN", Required: false}},
		[]publish.AuthRequirement{{Name: "google", LoginHint: "ktw-example auth login"}})
	b := mktapi.NeedsOf("ktw-kb",
		[]publish.ToolRequirement{
			{Name: "pandoc", Required: true, Version: "3.6", Recipes: []publish.InstallRecipe{
				{Platform: "windows", Kind: "winget", Package: "JohnMacFarlane.Pandoc"},
				{Platform: "linux", Kind: "apt", Package: "pandoc"},
			}},
		},
		[]publish.APIKeyRequirement{{EnvName: "KROKI_TOKEN", Required: true}, {EnvName: "OPENAI_API_KEY", Required: true}},
		[]publish.AuthRequirement{{Name: "google"}})

	s := SumRequires(a, b)

	if len(s.Tools) != 2 {
		t.Fatalf("tools = %+v, want Pandoc once and Typst", s.Tools)
	}
	p := s.Tools[0]
	if p.Name != "Pandoc" || !p.Required || p.Version != "3.6" {
		t.Errorf("Pandoc = %+v, want required at 3.6", p)
	}
	if !reflect.DeepEqual(p.NeededBy, []string{"ktw-docs", "ktw-kb"}) {
		t.Errorf("Pandoc needed by %v", p.NeededBy)
	}
	if len(p.Recipes) != 2 || !reflect.DeepEqual(p.Platforms, []string{"windows", "linux"}) {
		t.Errorf("Pandoc recipes = %+v, platforms = %v (the shared winget recipe once)", p.Recipes, p.Platforms)
	}
	if s.Tools[1].Name != "Typst" || s.Tools[1].Required {
		t.Errorf("Typst = %+v, want optional", s.Tools[1])
	}
	if len(s.APIKeys) != 2 || s.APIKeys[0].Name != "KROKI_TOKEN" || !s.APIKeys[0].Required ||
		!reflect.DeepEqual(s.APIKeys[0].NeededBy, []string{"ktw-docs", "ktw-kb"}) {
		t.Errorf("keys = %+v", s.APIKeys)
	}
	if len(s.Auth) != 1 || s.Auth[0].LoginHint != "ktw-example auth login" ||
		!reflect.DeepEqual(s.Auth[0].NeededBy, []string{"ktw-docs", "ktw-kb"}) {
		t.Errorf("logins = %+v", s.Auth)
	}
}
