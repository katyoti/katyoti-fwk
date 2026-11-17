// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"strings"
	"testing"
)

// Every installer kind validates on a platform it serves.
func TestInstallRecipe_ValidKinds(t *testing.T) {
	sum := strings.Repeat("0f", 32)
	for _, r := range []InstallRecipe{
		{Platform: PlatformWindows, Kind: RecipeWinget, Package: "JohnMacFarlane.Pandoc", MinVersion: "3.6"},
		{Platform: PlatformWindows, Kind: RecipeScoop, Package: "extras/pandoc"},
		{Platform: PlatformDarwin, Kind: RecipeBrew, Package: "pandoc"},
		{Platform: PlatformLinux, Kind: RecipeBrew, Package: "pandoc"},
		{Platform: PlatformLinux, Kind: RecipeApt, Package: "pandoc"},
		{Platform: PlatformDarwin, Kind: RecipePip, Package: "docling>=2.0"},
		{Platform: PlatformWindows, Kind: RecipeBun, Package: "@mermaid-js/mermaid-cli"},
		{Platform: PlatformLinux, Kind: RecipeDownload, URL: "https://example.org/x.tar.gz", SHA256: sum},
		{Platform: PlatformWindows, Kind: RecipeScript, Script: "scripts/install-x.ps1"},
	} {
		if err := r.Validate(); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}
}

// A recipe an installer engine could not run is refused with an error naming
// what is wrong.
func TestInstallRecipe_InvalidRefused(t *testing.T) {
	sum := strings.Repeat("0f", 32)
	for _, c := range []struct {
		r    InstallRecipe
		want string
	}{
		{InstallRecipe{Platform: PlatformWindows, Kind: "chocolatey", Package: "x"}, `unknown installer kind "chocolatey"`},
		{InstallRecipe{Platform: "freebsd", Kind: RecipePip, Package: "x"}, `unknown platform "freebsd"`},
		{InstallRecipe{Platform: PlatformLinux, Kind: RecipeWinget, Package: "x"}, "winget does not install on linux"},
		{InstallRecipe{Platform: PlatformWindows, Kind: RecipeApt, Package: "x"}, "apt does not install on windows"},
		{InstallRecipe{Platform: PlatformWindows, Kind: RecipeBrew, Package: "x"}, "brew does not install on windows"},
		{InstallRecipe{Platform: PlatformDarwin, Kind: RecipeBrew}, "no package"},
		{InstallRecipe{Platform: PlatformDarwin, Kind: RecipeBrew, Package: "x", URL: "https://example.org/x"}, "do not apply"},
		{InstallRecipe{Platform: PlatformLinux, Kind: RecipeDownload, URL: "https://example.org/x.tar.gz"}, "no sha256"},
		{InstallRecipe{Platform: PlatformLinux, Kind: RecipeDownload, URL: "https://example.org/x.tar.gz", SHA256: "abc"}, "not 64 hex digits"},
		{InstallRecipe{Platform: PlatformLinux, Kind: RecipeDownload, SHA256: sum}, "no url"},
		{InstallRecipe{Platform: PlatformLinux, Kind: RecipeDownload, URL: "ftp://example.org/x", SHA256: sum}, "not an http(s) URL"},
		{InstallRecipe{Platform: PlatformLinux, Kind: RecipeScript}, "no script path"},
		{InstallRecipe{Platform: PlatformLinux, Kind: RecipeScript, Script: "../outside.sh"}, "climbs out"},
		{InstallRecipe{Platform: PlatformWindows, Kind: RecipeScript, Script: `C:\x.ps1`}, "relative forward-slash path"},
		{InstallRecipe{Platform: PlatformLinux, Kind: RecipeScript, Script: "/abs.sh"}, "relative forward-slash path"},
	} {
		err := c.r.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: err %v, want it to say %q", c.r, err, c.want)
		}
	}
}

// A tool's validation names the tool and every faulty recipe; a nameless
// declaration is refused.
func TestToolDef_Validate(t *testing.T) {
	tool := ToolDef{Name: "pandoc", Recipes: []InstallRecipe{
		{Platform: PlatformWindows, Kind: RecipeWinget, Package: "JohnMacFarlane.Pandoc"},
		{Platform: PlatformLinux, Kind: RecipeDownload, URL: "https://example.org/p.tar.gz"},
		{Platform: PlatformDarwin, Kind: "port", Package: "pandoc"},
	}}
	err := tool.Validate()
	if err == nil {
		t.Fatal("two faulty recipes passed")
	}
	for _, want := range []string{`tool "pandoc" recipe 2`, "no sha256", `tool "pandoc" recipe 3`, `unknown installer kind "port"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
	if err := (ToolDef{Recipes: []InstallRecipe{{Platform: PlatformLinux, Kind: RecipeApt, Package: "x"}}}).Validate(); err == nil {
		t.Error("a nameless tool passed")
	}
	if err := (ToolDef{Name: "plain", Commands: []string{"plain"}}).Validate(); err != nil {
		t.Errorf("a tool without recipes is refused: %v", err)
	}
	if got := tool.RecipesFor(PlatformWindows); len(got) != 1 || got[0].Package != "JohnMacFarlane.Pandoc" {
		t.Errorf("RecipesFor(windows) = %+v", got)
	}
	if r := (ToolDef{Name: "x", Optional: true}).Requirement(); r.Required {
		t.Error("an optional tool reads back required")
	}
}

// Custom installer arguments are winget's alone: another kind refuses them.
func TestCustomArgumentsAreWingetsOnly(t *testing.T) {
	ok := InstallRecipe{Platform: PlatformWindows, Kind: RecipeWinget, Package: "Python.Python.3.12", Custom: "InstallLauncherAllUsers=0"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a winget recipe with custom arguments refused: %v", err)
	}
	bad := InstallRecipe{Platform: PlatformWindows, Kind: RecipeScoop, Package: "python", Custom: "x"}
	if err := bad.Validate(); err == nil {
		t.Fatal("a scoop recipe with custom arguments accepted")
	}
}
