// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// With no themes in reach, a deck naming a theme gets one info finding and
// no variant or icon finding; the default theme needs no word.
func TestLintThemeUnchecked(t *testing.T) {
	deck := "---\ntitle: T\ntheme: brand\n---\n\n<!-- slide: variant=nope -->\n## A\n\n![x](icon:nope)\n"
	fs := api.Lint(deck, api.Grammar{})
	if len(fs) != 1 || fs[0].Rule != "theme-unchecked" || fs[0].Severity != api.SeverityInfo || fs[0].Line != 3 {
		t.Fatalf("want one info theme-unchecked on line 3, got %+v", fs)
	}
	if api.HasErrors(fs) {
		t.Fatal("an unchecked theme must not block the deck")
	}
	if fs := api.Lint("---\ntitle: T\ntheme: katyoti\n---\n\n## A\n", api.Grammar{}); len(fs) != 0 {
		t.Fatalf("the default theme needs no finding, got %+v", fs)
	}
	// Some themes in reach, but not every theme there is (one sidecar's): a
	// theme they lack is unchecked, not unknown.
	partial := api.Grammar{Themes: []api.ThemeBody{{Name: "katyoti"}}}
	if fs := api.Lint(deck, partial); len(fs) != 1 || fs[0].Rule != "theme-unchecked" {
		t.Fatalf("want theme-unchecked under a partial grammar, got %+v", fs)
	}
	partial.ThemesComplete = true
	if fs := api.Lint(deck, partial); len(fs) != 1 || fs[0].Rule != "theme-unknown" || fs[0].Severity != api.SeverityError {
		t.Fatalf("want theme-unknown under a complete grammar, got %+v", fs)
	}
}

// Assets is every file the declaration names — what the deck lint holds the
// theme's folder to and what ResolveTheme fetches.
func TestThemeAssets(t *testing.T) {
	th := api.ThemeBody{
		Fonts:  []api.ThemeFont{{Family: "Skeena", Src: "fonts/skeena.woff2"}},
		Chrome: &api.ThemeChrome{Logo: &api.ThemeImage{Asset: "logo.png"}},
		Layouts: map[string]api.ThemeLayout{
			"cover": {
				Default:  &api.ThemeVariant{Background: "bg/bordeaux.png", Logo: &api.ThemeImage{Asset: "logo-white.png"}},
				Variants: map[string]api.ThemeVariant{"cream": {Background: "#F2EBE2"}, "sand": {Background: "bg/sand.jpg"}},
			},
		},
		Icons: map[string]api.ThemeIcon{"truck": {Asset: "icons/truck.png"}, "cart": {Asset: "logo.png"}},
	}
	want := []string{"bg/bordeaux.png", "bg/sand.jpg", "fonts/skeena.woff2", "icons/truck.png", "logo-white.png", "logo.png"}
	if got := th.Assets(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Assets = %v, want %v", got, want)
	}
}

// Version names the content: equal for equal themes, different when the
// declaration or any file changes — what lets a host skip a re-send and a
// page cache the files for good.
func TestResolvedThemeVersion(t *testing.T) {
	mk := func(desc string, logo string) api.ResolvedTheme {
		return api.ResolvedTheme{
			Body:   api.ThemeBody{Name: "brand", Description: desc},
			Assets: map[string][]byte{"logo.png": []byte(logo), "bg/a.jpg": []byte("a")},
		}
	}
	a, b := mk("x", "L"), mk("x", "L")
	if a.Version() != b.Version() || len(a.Version()) != 16 {
		t.Fatalf("equal themes must share a 16-char version: %q %q", a.Version(), b.Version())
	}
	if a.Version() == mk("y", "L").Version() {
		t.Fatal("a declaration change must change the version")
	}
	if a.Version() == mk("x", "M").Version() {
		t.Fatal("a file change must change the version")
	}
}

func TestIsColour(t *testing.T) {
	for bg, want := range map[string]bool{
		"#C8B39F": true, "#fff": true, "rgb(1, 2, 3)": true, "white": true,
		"bg/sand.jpg": false, "sand.png": false, "assets/x": false,
	} {
		if got := api.IsColour(bg); got != want {
			t.Errorf("IsColour(%q) = %v, want %v", bg, got, want)
		}
	}
}

// A theme with a palette (a brand charter's) allows no other background
// colour; an image background and a theme without a palette are not checked.
func TestLintThemePalette(t *testing.T) {
	brand := api.ThemeBody{Name: "brand", Palette: []api.ThemeColour{{Name: "Rouge cavalier", Value: "#82072A"}, {Name: "Blanc", Value: "#FFFFFF"}}}
	g := api.Grammar{Themes: []api.ThemeBody{brand}, ThemesComplete: true}
	deck := func(bg string) string {
		return "---\ntitle: T\ntheme: brand\n---\n\n<!-- slide: background=" + bg + " -->\n## A\n"
	}
	fs := api.Lint(deck("#0000ff"), g)
	if len(fs) != 1 || fs[0].Rule != "theme-palette" || fs[0].Severity != api.SeverityError || fs[0].Line != 6 {
		t.Fatalf("want one theme-palette error on line 6, got %+v", fs)
	}
	for _, ok := range []string{"#82072a", "#FFFFFF", "photos/lac.jpg"} {
		if fs := api.Lint(deck(ok), g); len(fs) != 0 {
			t.Errorf("background %s: want no finding, got %+v", ok, fs)
		}
	}
	free := api.Grammar{Themes: []api.ThemeBody{{Name: "brand"}}, ThemesComplete: true}
	if fs := api.Lint(deck("#0000ff"), free); len(fs) != 0 {
		t.Errorf("a theme without a palette allows any colour, got %+v", fs)
	}
}

// A deck's watermark and mark are declared settings; `mark:` with no value is
// not set (a deck object composes empty fields so); a wrong mark is refused.
func TestLintWatermarkSettings(t *testing.T) {
	ok := "---\ntitle: T\nwatermark: BROUILLON\nmark:\n---\n\n## A\n"
	if fs := api.Lint(ok, api.Grammar{}); len(fs) != 0 {
		t.Fatalf("want no finding, got %+v", fs)
	}
	bad := "---\ntitle: T\nmark: maybe\n---\n\n## A\n"
	if fs := api.Lint(bad, api.Grammar{}); len(fs) != 1 || fs[0].Rule != "deck-value-invalid" {
		t.Fatalf("want deck-value-invalid, got %+v", fs)
	}
}

// The files a deck uses beside it: relative images and backgrounds; not URLs,
// icons, absolute paths, code, or a path leaving the folder.
func TestLocalAssets(t *testing.T) {
	src := "---\ntitle: T\n---\n\n<!-- slide: background=photos/lac.jpg -->\n## A\n![topo](assets/topology.svg) ![w](https://x.test/y.png) ![i](icon:truck)\n`![no](code.png)`\n\n---\n\n<!-- slide: background=#fff -->\n![dup](./assets/topology.svg \"t\") ![up](../secret.png) ![abs](/etc/x.png)\n\n```\n![no](fence.png)\n```\n"
	got := api.LocalAssets(src)
	want := []string{"assets/topology.svg", "photos/lac.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if p, ok := api.LocalAsset("../secret.png"); !ok || p != "" {
		t.Errorf("a path leaving the folder: %q %v", p, ok)
	}
	if _, ok := api.LocalAsset("C:/x.png"); ok {
		t.Error("a drive path is not beside the deck")
	}
}

// The lint names a missing file at its line, and a path leaving the folder.
func TestLintAssets(t *testing.T) {
	src := "---\ntitle: T\n---\n\n<!-- slide: background=bg/x.jpg -->\n## A\n![a](assets/here.png)\n![b](assets/gone.png)\n![c](../out.png)\n"
	fs := api.LintAssets(src, func(rel string) bool { return rel == "assets/here.png" })
	if len(fs) != 3 || fs[0].Line != 5 || fs[0].Rule != "asset-missing" || fs[1].Line != 8 || fs[2].Rule != "asset-outside" || fs[2].Severity != api.SeverityError {
		t.Fatalf("got %+v", fs)
	}
}

// A deck that sets an aspect other than its theme's is warned: the theme's
// chrome is placed for its own shape. One that sets none, or the same, is not.
func TestLintThemeAspect(t *testing.T) {
	wide := api.ThemeBody{Name: "wide", Aspect: "16:9"}
	g := api.Grammar{Themes: []api.ThemeBody{wide, {Name: "any"}}, ThemesComplete: true}
	deck := func(theme, aspect string) string {
		fm := "---\ntitle: T\ntheme: " + theme + "\n"
		if aspect != "" {
			fm += "aspect: \"" + aspect + "\"\n"
		}
		return fm + "---\n\n## A\n"
	}
	fs := api.Lint(deck("wide", "4:3"), g)
	if len(fs) != 1 || fs[0].Rule != "theme-aspect" || fs[0].Severity != api.SeverityWarn || fs[0].Line != 4 {
		t.Fatalf("want one theme-aspect warning on line 4, got %+v", fs)
	}
	for _, c := range [][2]string{{"wide", ""}, {"wide", "16:9"}, {"any", "4:3"}} {
		if fs := api.Lint(deck(c[0], c[1]), g); len(fs) != 0 {
			t.Errorf("theme %s aspect %q: want no finding, got %+v", c[0], c[1], fs)
		}
	}
}
