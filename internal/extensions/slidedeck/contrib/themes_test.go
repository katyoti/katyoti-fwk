// SPDX-License-Identifier: Apache-2.0

package contrib_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
	decklint "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/lint"
)

// The built-in themes are sound, and laid out as a pack: a declaration under
// deck-themes/ and its files under deck-assets/.
func TestBuiltinThemesAreSound(t *testing.T) {
	for _, err := range decklint.CheckPack(contrib.BuiltinThemesFS(), "themes") {
		t.Error(err)
	}
	if _, err := fs.Stat(contrib.BuiltinThemesFS(), "themes/deck-themes/katyoti.yaml"); err != nil {
		t.Errorf("the built-in theme is not a pack's deck-themes/ entry: %v", err)
	}
}

const soundTheme = `name: brand
description: A brand.
chrome:
  logo: {asset: logo.png, box: {x: 10, y: 10, w: 100, h: 50}}
  slideNumber: {template: "{n}", box: {x: 0, y: 700, w: 40, h: 20}}
layouts:
  cover:
    chrome: false
    variants:
      sand: {background: bg/sand.jpg}
      plain: {background: "#F2EBE2"}
icons:
  truck: {asset: icons/truck.png, label: Camion}
`

// themeFS lays a theme out as a pack under t/: its declaration in
// deck-themes/, its files (named <theme>/<path>) in deck-assets/.
func themeFS(decl string, files map[string]int) fstest.MapFS {
	m := fstest.MapFS{"t/deck-themes/brand.yaml": {Data: []byte(decl)}}
	for f, n := range files {
		m["t/deck-assets/"+f] = &fstest.MapFile{Data: make([]byte, n)}
	}
	return m
}

var soundFiles = map[string]int{"brand/logo.png": 10, "brand/bg/sand.jpg": 10, "brand/icons/truck.png": 10}

func check(m fstest.MapFS) string {
	var msgs []string
	for _, e := range decklint.CheckPack(m, "t") {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "\n")
}

func TestCheckPackAcceptsASoundTheme(t *testing.T) {
	if got := check(themeFS(soundTheme, soundFiles)); got != "" {
		t.Fatalf("a sound theme was refused:\n%s", got)
	}
}

// Every refusal is a real mistake a theme author makes; each names the file.
func TestCheckPackRefuses(t *testing.T) {
	with := func(extra map[string]int, drop ...string) map[string]int {
		m := map[string]int{}
		for k, v := range soundFiles {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		for _, d := range drop {
			delete(m, d)
		}
		return m
	}
	cases := []struct {
		name, decl string
		files      map[string]int
		want       string
	}{
		{"a named file is missing", soundTheme, with(nil, "brand/logo.png"), `names "logo.png", which is not shipped`},
		{"a shipped file is unnamed", soundTheme, with(map[string]int{"brand/stray.png": 1}), "brand/stray.png: shipped but not named"},
		{"a file above the control band", soundTheme, with(map[string]int{"brand/logo.png": contribapi.BandControl + 1}), "above the control band"},
		{"the name is not the file's", strings.Replace(soundTheme, "name: brand", "name: other", 1), soundFiles, `name "other" must be the file's name "brand"`},
		{"an icon name is not kebab", strings.Replace(soundTheme, "truck:", "Camion:", 1), soundFiles, `icon "Camion" is not kebab-case`},
		{"a variant name is not kebab", strings.Replace(soundTheme, "sand:", "Sand_1:", 1), soundFiles, `variant "Sand_1" is not kebab-case`},
		{"an unknown key", soundTheme + "colours: {}\n", soundFiles, "not a deck theme"},
		{"an asset folder without a declaration", soundTheme, with(map[string]int{"ghost/a.png": 1}), "ghost/: files of a theme without a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := check(themeFS(tc.decl, tc.files)); !strings.Contains(got, tc.want) {
				t.Fatalf("want a finding containing %q, got:\n%s", tc.want, got)
			}
		})
	}
}
