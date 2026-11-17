// SPDX-License-Identifier: Apache-2.0

// Package lint is the format contract of the slide-deck kinds a sidecar
// ships in its pack: a deck theme (`deck-themes/<name>.yaml`), the files it
// names (`deck-assets/<name>/<path>`) and a slide group
// (`slide-groups/<name>.yaml`). LintThemeBytes and LintSlideGroupBytes check
// one file - the rows the manifest lint reads a kind's check from - and
// CheckPack holds a pack's deck folders together: what no single file can
// tell (a theme's files shipped and named, within the control band, its
// sample deck clean). No contributor links it: the lint and the census run it.
package lint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
)

// The pack folders of the deck kinds, as their declarations' source blocks
// name them (slidedeck/contrib/deckkind).
const (
	ThemesFolder = "deck-themes"
	AssetsFolder = "deck-assets"
	GroupsFolder = "slide-groups"
)

// The rules a finding names.
const (
	RuleNotATheme   = "deck-theme-format"
	RuleNotKebab    = "deck-theme-name"
	RuleNotAGroup   = "slide-group-format"
	RuleGroupSlides = "slide-group-slides"
)

// kebab is a name a deck writes: an icon, a variant, a theme.
var kebab = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// LintThemeBytes checks one theme declaration: it decodes as a deck theme
// (the YAML's structure read by the JSON names the data-type publishes, no
// unknown key), and its theme, variant and icon names are kebab-case ASCII.
func LintThemeBytes(src []byte) verify.LintResult {
	t, err := decodeTheme(src)
	if err != nil {
		return verify.Result(verify.LintFinding{Rule: RuleNotATheme, Message: "not a deck theme: " + err.Error(), Severity: verify.SeverityError})
	}
	var out []verify.LintFinding
	bad := func(format string, a ...any) {
		out = append(out, verify.LintFinding{Rule: RuleNotKebab, Message: fmt.Sprintf(format, a...), Severity: verify.SeverityError})
	}
	if !kebab.MatchString(t.Name) {
		bad("theme name %q is not kebab-case ASCII", t.Name)
	}
	for _, l := range sortedKeys(t.Layouts) {
		for _, v := range sortedKeys(t.Layouts[l].Variants) {
			if !kebab.MatchString(v) {
				bad("layout %s: variant %q is not kebab-case ASCII", l, v)
			}
		}
	}
	for _, i := range sortedKeys(t.Icons) {
		if !kebab.MatchString(i) {
			bad("icon %q is not kebab-case ASCII", i)
		}
	}
	return verify.Result(out...)
}

// LintSlideGroupBytes checks one slide group: it decodes as a group (no
// unknown key), has a description and slides, no slide includes another
// group, and its slides lint clean against the built-in layouts.
func LintSlideGroupBytes(src []byte) verify.LintResult {
	g, err := decodeGroup(src)
	if err != nil {
		return verify.Result(verify.LintFinding{Rule: RuleNotAGroup, Message: "not a slide group: " + err.Error(), Severity: verify.SeverityError})
	}
	var out []verify.LintFinding
	bad := func(format string, a ...any) {
		out = append(out, verify.LintFinding{Rule: RuleGroupSlides, Message: fmt.Sprintf(format, a...), Severity: verify.SeverityError})
	}
	if g.Description == "" || strings.TrimSpace(g.Slides) == "" {
		bad("a group has a description and slides")
	}
	md := "---\ntitle: \"" + g.Name + "\"\n---\n\n" + g.Slides
	if len(deckapi.Includes(deckapi.Parse(md))) > 0 {
		bad("a slide of a group includes another group")
	}
	layouts, err := contrib.BuiltinLayouts()
	if err != nil {
		return verify.ErrorResult(RuleGroupSlides, "built-in layouts: "+err.Error())
	}
	for _, f := range deckapi.Lint(md, deckapi.Grammar{Layouts: layouts}) {
		if f.Severity == deckapi.SeverityError {
			bad("line %d: %s", f.Line-4, f.Message)
		}
	}
	return verify.Result(out...)
}

// CheckPack holds the deck folders of the pack under root in fsys to what a
// deck and the host can use, and returns every problem found (none when they
// are sound): each theme and group passes its file check and is named as its
// file; every file a theme names is shipped under deck-assets/<theme>/ and
// every shipped file is named by its theme; no file is above the control
// band, which the host could not fetch from the sidecar; a theme's sample
// deck names the theme and lints clean against it and the built-in layouts.
// A pack without deck folders is sound.
func CheckPack(fsys fs.FS, root string) []error {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	themes := path.Join(root, ThemesFolder)
	assets := path.Join(root, AssetsFolder)

	shipped := map[string]map[string]bool{} // theme -> its files
	_ = fs.WalkDir(fsys, assets, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, assets+"/")
		theme, file, nested := strings.Cut(rel, "/")
		if !nested {
			fail("%s: a deck-assets file sits in its theme's folder (%s/<theme>/<path>)", p, assets)
			return nil
		}
		if shipped[theme] == nil {
			shipped[theme] = map[string]bool{}
		}
		shipped[theme][file] = true
		if info, err := d.Info(); err == nil && info.Size() > contribapi.BandControl {
			fail("%s: %d bytes, above the control band (%d); the host cannot fetch it: make it smaller", p, info.Size(), contribapi.BandControl)
		}
		return nil
	})

	declared := map[string]bool{}
	for _, name := range yamlFiles(fsys, themes) {
		file := path.Join(themes, name)
		want := strings.TrimSuffix(name, path.Ext(name))
		declared[want] = true
		raw, err := fs.ReadFile(fsys, file)
		if err != nil {
			fail("%s: %v", file, err)
			continue
		}
		for _, f := range LintThemeBytes(raw).Findings {
			fail("%s: %s", file, f.Message)
		}
		t, err := decodeTheme(raw)
		if err != nil {
			continue
		}
		if t.Name != want {
			fail("%s: name %q must be the file's name %q", file, t.Name, want)
		}
		named := map[string]bool{}
		for _, a := range t.Assets() {
			named[a] = true
			if !shipped[want][a] {
				fail("%s: names %q, which is not shipped as %s/%s/%s", file, a, assets, want, a)
			}
		}
		for _, f := range sortedKeys(shipped[want]) {
			if !named[f] {
				fail("%s/%s/%s: shipped but not named by the theme", assets, want, f)
			}
		}
		if t.Sample != "" && shipped[want][t.Sample] {
			for _, e := range checkSample(fsys, path.Join(assets, want, t.Sample), t) {
				fail("%s", e)
			}
		}
	}
	for _, theme := range sortedKeys(shipped) {
		if !declared[theme] {
			fail("%s/%s/: files of a theme without a %s/%s.yaml declaration", assets, theme, themes, theme)
		}
	}

	groups := path.Join(root, GroupsFolder)
	for _, name := range yamlFiles(fsys, groups) {
		file := path.Join(groups, name)
		raw, err := fs.ReadFile(fsys, file)
		if err != nil {
			fail("%s: %v", file, err)
			continue
		}
		for _, f := range LintSlideGroupBytes(raw).Findings {
			fail("%s: %s", file, f.Message)
		}
		if g, err := decodeGroup(raw); err == nil {
			if want := strings.TrimSuffix(name, path.Ext(name)); g.Name != want {
				fail("%s: name %q is not its file's name %q", file, g.Name, want)
			}
		}
	}
	return errs
}

// checkSample lints a theme's sample deck against the theme and the built-in
// layouts: the deck the theme preview shows names the theme and holds no
// error. The built-in layouts, not the catalog's: a pack is checked where no
// project override exists.
func checkSample(fsys fs.FS, file string, t deckapi.ThemeBody) []string {
	src, err := fs.ReadFile(fsys, file)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", file, err)}
	}
	var out []string
	if named, _ := deckapi.Parse(string(src)).Settings["theme"].(string); named != t.Name {
		out = append(out, fmt.Sprintf("%s: the sample names theme %q, not %q", file, named, t.Name))
	}
	layouts, err := contrib.BuiltinLayouts()
	if err != nil {
		return append(out, fmt.Sprintf("%s: built-in layouts: %v", file, err))
	}
	for _, f := range deckapi.Lint(string(src), deckapi.Grammar{Layouts: layouts, Themes: []deckapi.ThemeBody{t}}) {
		if f.Severity == deckapi.SeverityError {
			out = append(out, fmt.Sprintf("%s:%d: %s: %s", file, f.Line, f.Rule, f.Message))
		}
	}
	return out
}

// decodeTheme reads a theme as its data layer is read: the YAML's structure
// as JSON, by the JSON names the data-type publishes, no unknown key.
func decodeTheme(src []byte) (deckapi.ThemeBody, error) {
	var t deckapi.ThemeBody
	return t, decodeStrict(src, &t)
}

func decodeGroup(src []byte) (deckapi.SlideGroupBody, error) {
	var g deckapi.SlideGroupBody
	return g, decodeStrict(src, &g)
}

func decodeStrict(src []byte, v any) error {
	var tree any
	// Content, not configuration: the file's own structure, nothing expanded.
	if err := marshal.Unmarshal(src, marshal.YAML, &tree); err != nil {
		return fmt.Errorf("yaml: %w", err)
	}
	js, err := json.Marshal(tree)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(js))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// yamlFiles are the YAML files directly in dir, sorted; none when dir is absent.
func yamlFiles(fsys fs.FS, dir string) []string {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		ext := strings.ToLower(path.Ext(e.Name()))
		if !e.IsDir() && (ext == ".yaml" || ext == ".yml") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
