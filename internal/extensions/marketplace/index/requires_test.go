// SPDX-License-Identifier: Apache-2.0

package index

import (
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// The programs, keys and logins a sidecar's describe declares are read back
// from the catalog the indexer writes: what it needs is known before install.
func TestAssembleItem_CarriesToolsKeysAndLogins(t *testing.T) {
	desc := appcore.SidecarDescription{
		Name: "ktw-docs", Version: "2.1.0",
		Formats: map[string]string{"skill": "1.0"},
		Tools: []publish.ToolRequirement{{
			Name: "Pandoc", Required: true, Commands: []string{"pandoc"}, Version: "3.1",
			InstallSkill: "ktw-docs:/skill/install-pandoc",
			Recipes: []publish.InstallRecipe{
				{Platform: "windows", Kind: "winget", Package: "JohnMacFarlane.Pandoc"},
				{Platform: "darwin", Kind: "brew", Package: "pandoc"},
			},
		}, {Name: "Typst", Required: false, Commands: []string{"typst"}}},
		APIKeys: []publish.APIKeyRequirement{{EnvName: "KROKI_TOKEN", Label: "Kroki token", Required: false}},
		Auth:    []publish.AuthRequirement{{Name: "google", Label: "Google login", LoginHint: "ktw-example auth login"}},
	}
	item := assembleItem(desc, []mktapi.Asset{{OS: "windows", Arch: "amd64", URL: "x", Sha256: "00"}})
	out, err := MarshalCatalog(mktapi.Catalog{APIVersion: 1, Items: []mktapi.Item{item}})
	if err != nil {
		t.Fatal(err)
	}
	var parsed mktapi.Catalog
	if err := config.DecodeBytes(out, marshal.YAML, &parsed); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	req := parsed.Items[0].Versions[0].Requires
	if req.Formats["skill"] != "1.0" {
		t.Errorf("formats lost: %+v", req.Formats)
	}
	if len(req.Tools) != 2 {
		t.Fatalf("tools = %+v, want Pandoc and Typst\n%s", req.Tools, out)
	}
	p := req.Tools[0]
	if p.Name != "Pandoc" || !p.Required || p.Version != "3.1" || len(p.Recipes) != 2 ||
		!reflect.DeepEqual(p.Platforms, []string{"windows", "darwin"}) || !reflect.DeepEqual(p.NeededBy, []string{"ktw-docs"}) {
		t.Errorf("Pandoc = %+v", p)
	}
	if req.Tools[1].Name != "Typst" || req.Tools[1].Required {
		t.Errorf("Typst = %+v, want optional", req.Tools[1])
	}
	if len(req.APIKeys) != 1 || req.APIKeys[0].Name != "KROKI_TOKEN" || req.APIKeys[0].Required ||
		!reflect.DeepEqual(req.APIKeys[0].NeededBy, []string{"ktw-docs"}) {
		t.Errorf("keys = %+v", req.APIKeys)
	}
	if len(req.Auth) != 1 || req.Auth[0].Name != "google" || req.Auth[0].LoginHint != "ktw-example auth login" {
		t.Errorf("logins = %+v", req.Auth)
	}
}
