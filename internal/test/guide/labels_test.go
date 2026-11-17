// SPDX-License-Identifier: Apache-2.0

package guide

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
)

var (
	reBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	// A literal the GUI shows in every language (a folder not yet translated):
	// JSX text, a user-visible prop given a string, or an action's label.
	reJSXText     = regexp.MustCompile(`>([^<>{}]*[A-Za-z][^<>{}]*)<`)
	reLiteralProp = regexp.MustCompile(`\b(?:title|label|aria-label|placeholder)\s*=\s*(?:"([^"]+)"|\{\s*'([^']+)'\s*\})`)
	reLabelField  = regexp.MustCompile(`\blabel:\s*'([^']+)'`)
)

// TestRecipeTextsNameControlsByAnchor holds what a person reads in every
// shipped recipe (modules/guide, OD-007): a step names a control as
// `{control:<anchor>}` (`|<name>` for an instance), never by its label, and
// the player says the label the GUI shows. So, in the English of a recipe and
// in each of its translations:
//   - every placeholder is well formed and names an anchor of the GUI's
//     registry (frontend/src/guide/anchors.ts);
//   - that anchor has a `label` key present in the en AND fr locales (its
//     `{name}` filled by the placeholder's instance), or is declared
//     `contributed` (its text is a contribution's, read from the control);
//   - a translation names the same controls as the English, so no language
//     loses one;
//   - a name in bold that is a label the GUI shows in that language (the old
//     form) is refused.
func TestRecipeTextsNameControlsByAnchor(t *testing.T) {
	root := repoRoot(t)
	reg := readRegistry(t, root)
	keys := localeKeys(t, root)
	labels := guiLabels(t, root)
	refs := 0
	for _, rt := range shippedRecipeTexts(t, root) {
		for lang, steps := range rt.texts {
			for id, say := range steps {
				where := fmt.Sprintf("%s [%s] step %s", rt.file, lang, id)
				got, malformed := guideapi.ControlRefs(say)
				if malformed > 0 {
					t.Errorf("%s: a {control:…} placeholder is malformed: %q", where, say)
				}
				for _, ref := range got {
					refs++
					a, ok := reg[ref.Anchor]
					if !ok && strings.HasPrefix(ref.Anchor, "objtab.tab:") {
						continue // a contributed tab (one family, objtab.<entry id>): its label is its panel's, read from the control
					}
					if !ok {
						t.Errorf("%s: {control:%s} names no anchor of the GUI registry", where, ref.Anchor)
						continue
					}
					if a.label == "" {
						if !a.contributed {
							t.Errorf("%s: anchor %s has neither a label key nor `contributed: true`", where, ref.Anchor)
						}
						continue
					}
					key := a.label
					if strings.Contains(key, "{name}") {
						if ref.Name == "" {
							t.Errorf("%s: {control:%s} needs |<name>: its label key %s names the instance", where, ref.Anchor, key)
							continue
						}
						key = strings.ReplaceAll(key, "{name}", ref.Name)
					}
					for _, l := range []string{"en", "fr"} {
						if _, ok := keys[l][key]; !ok {
							t.Errorf("%s: {control:%s}: label key %s is not in the %s locales", where, ref.Anchor, key, l)
						}
					}
				}
				if lang != "en" {
					if want := refsOf(rt.texts["en"][id]); refsOf(say) != want {
						t.Errorf("%s names the controls [%s], the English [%s]", where, refsOf(say), want)
					}
				}
				for _, m := range reBold.FindAllStringSubmatch(say, -1) {
					if labels[lang][strings.TrimSpace(m[1])] {
						t.Errorf("%s: **%s** is a control's label written as text; name the control {control:<anchor>}", where, m[1])
					}
				}
			}
		}
	}
	if refs < 60 {
		t.Fatalf("checked %d placeholders — the walk is wrong", refs)
	}
}

// TestControlLabelKeysAreTheComponentsOwn: an anchor's `label` key is the
// key the control renders (one source), so the guide says what the control
// shows — the key is used by the file that places the anchor (by any GUI
// file for the object pane's strip entries, whose labels their providers
// give). A templated key (`tabs.{name}`) is held by the recipes test above.
func TestControlLabelKeysAreTheComponentsOwn(t *testing.T) {
	root := repoRoot(t)
	front := filepath.Join(root, "app", "ktw-gui", "frontend", "src")
	reg := readRegistry(t, root)
	var all strings.Builder
	_ = filepath.WalkDir(front, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".tsx") || strings.HasSuffix(p, ".ts")) && !strings.HasSuffix(p, "anchors.ts") {
			src, _ := os.ReadFile(p)
			all.Write(src)
		}
		return nil
	})
	checked := 0
	for id, a := range reg {
		if a.label == "" || strings.Contains(a.label, "{name}") {
			continue
		}
		checked++
		key := a.label[strings.Index(a.label, ":")+1:]
		src := placingSource(front, a.file)
		used := strings.Contains(src, "'"+key+"'")
		if !used && strings.HasPrefix(id, "objtab.") {
			used = strings.Contains(all.String(), "'"+key+"'")
		}
		if !used {
			t.Errorf("anchor %s: its label key %s is not what %s renders", id, a.label, a.file)
		}
	}
	if checked < 20 {
		t.Fatalf("checked %d label keys — the registry parse is wrong", checked)
	}
}

// refsOf is a text's placeholders, sorted: what a translation must keep.
func refsOf(text string) string {
	got, _ := guideapi.ControlRefs(text)
	out := make([]string, 0, len(got))
	for _, r := range got {
		out = append(out, r.Anchor+"|"+r.Name)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

type recipeTexts struct {
	file  string
	texts map[string]map[string]string // lang → step id → text
}

// shippedRecipeTexts is every recipe a binary ships (app/*/embed/guide), its
// English and its translations.
func shippedRecipeTexts(t *testing.T, root string) []recipeTexts {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(root, "app", "*", "embed", "guide", "*.yaml"))
	var out []recipeTexts
	for _, p := range paths {
		if strings.HasSuffix(p, ".i18n.yaml") {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		r, err := guideapi.ParseRecipe(raw)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		rel, _ := filepath.Rel(filepath.Join(root, "app"), p)
		rt := recipeTexts{file: filepath.ToSlash(rel), texts: map[string]map[string]string{"en": {}}}
		for _, s := range r.Steps {
			rt.texts["en"][s.ID] = s.Say
		}
		if raw, err := os.ReadFile(strings.TrimSuffix(p, ".yaml") + ".i18n.yaml"); err == nil {
			tr, err := guideapi.ParseTranslations(raw)
			if err != nil {
				t.Fatalf("%s translations: %v", p, err)
			}
			for lang, text := range tr {
				rt.texts[lang] = text.Steps
			}
		}
		out = append(out, rt)
	}
	if len(out) < 15 {
		t.Fatalf("found %d shipped recipes — the walk is wrong", len(out))
	}
	return out
}

// localeKeys is every key of the GUI's locales, `<namespace>:<dotted key>`,
// by language.
func localeKeys(t *testing.T, root string) map[string]map[string]string {
	t.Helper()
	dir := filepath.Join(root, "app", "ktw-gui", "frontend", "src", "locales")
	out := map[string]map[string]string{}
	for _, lang := range []string{"en", "fr"} {
		out[lang] = map[string]string{}
		files, _ := filepath.Glob(filepath.Join(dir, lang, "*.json"))
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var tree map[string]any
			if err := json.Unmarshal(raw, &tree); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			flatten(strings.TrimSuffix(filepath.Base(f), ".json")+":", tree, out[lang])
		}
	}
	return out
}

func flatten(prefix string, tree map[string]any, out map[string]string) {
	for k, v := range tree {
		switch x := v.(type) {
		case string:
			out[prefix+k] = x
		case map[string]any:
			flatten(prefix+k+".", x, out)
		}
	}
}

// guiLabels is every text the GUI shows, by language.
func guiLabels(t *testing.T, root string) map[string]map[string]bool {
	t.Helper()
	front := filepath.Join(root, "app", "ktw-gui", "frontend", "src")
	out := map[string]map[string]bool{}
	add := func(lang, s string) {
		if out[lang] == nil {
			out[lang] = map[string]bool{}
		}
		out[lang][strings.TrimSpace(s)] = true
	}
	// The locales.
	langs, err := os.ReadDir(filepath.Join(front, "locales"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range langs {
		files, _ := filepath.Glob(filepath.Join(front, "locales", l.Name(), "*.json"))
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var tree any
			if err := json.Unmarshal(raw, &tree); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			eachString(tree, func(s string) { add(l.Name(), s) })
		}
	}
	if len(out["en"]) < 200 || len(out["fr"]) < 200 {
		t.Fatalf("read %d English and %d French labels — the parse is wrong", len(out["en"]), len(out["fr"]))
	}
	// The literals of files not yet translated: every language shows them.
	var literals []string
	_ = filepath.WalkDir(front, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".tsx") {
			return nil
		}
		src, _ := os.ReadFile(p)
		for _, m := range reJSXText.FindAllStringSubmatch(string(src), -1) {
			literals = append(literals, m[1])
		}
		for _, m := range reLiteralProp.FindAllStringSubmatch(string(src), -1) {
			literals = append(literals, m[1]+m[2])
		}
		for _, m := range reLabelField.FindAllStringSubmatch(string(src), -1) {
			literals = append(literals, m[1])
		}
		return nil
	})
	for lang := range out {
		for _, s := range literals {
			add(lang, s)
		}
	}
	// The texts of the contributions the GUI draws (panels, types, workflows):
	// a contribution file is its English, its .i18n.yaml the other languages.
	for _, base := range []string{filepath.Join(root, "app"), filepath.Join(root, "internal")} {
		_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "frontend") {
				return filepath.SkipDir
			}
			slash := filepath.ToSlash(p)
			if d.IsDir() || !strings.Contains(slash, "/embed/cwm/") || !strings.HasSuffix(slash, ".yaml") {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			var tree any
			if yaml.Unmarshal(raw, &tree) != nil {
				return nil
			}
			if !strings.HasSuffix(slash, ".i18n.yaml") {
				eachString(tree, func(s string) { add("en", s) })
				return nil
			}
			if m, ok := tree.(map[string]any); ok {
				for lang, sub := range m {
					if out[lang] != nil {
						eachString(sub, func(s string) { add(lang, s) })
					}
				}
			}
			return nil
		})
	}
	return out
}

func eachString(v any, fn func(string)) {
	switch x := v.(type) {
	case string:
		fn(x)
	case map[string]any:
		for _, e := range x {
			eachString(e, fn)
		}
	case []any:
		for _, e := range x {
			eachString(e, fn)
		}
	}
}

var reCheckKey = regexp.MustCompile(`\.WithMsg\("(checks\.[A-Za-z]+)"`)

// TestCheckKeysAreTranslated: every message key a guide check (or a step's do
// method) gives its detail is in the GUI's `guide` namespace — in English, and
// so (TestLocales_SameKeysInEveryLanguage) in every language — or the player
// would show a French reader the English sentence.
func TestCheckKeysAreTranslated(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "app", "ktw-gui", "frontend", "src", "locales", "en", "guide.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ns struct {
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(raw, &ns); err != nil {
		t.Fatal(err)
	}
	found := 0
	_ = filepath.WalkDir(filepath.Join(root, "internal", "extensions"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, _ := os.ReadFile(p)
		for _, m := range reCheckKey.FindAllStringSubmatch(string(src), -1) {
			found++
			if _, ok := ns.Checks[strings.TrimPrefix(m[1], "checks.")]; !ok {
				t.Errorf("%s: check message key %q is not in locales/en/guide.json", filepath.ToSlash(p), m[1])
			}
		}
		return nil
	})
	if found < 20 {
		t.Fatalf("found %d check message keys — the scan is wrong", found)
	}
}

// relImport is a component's import of a module beside it (`from './x'`).
var relImport = regexp.MustCompile(`from '\./([A-Za-z0-9_]+)'`)

// placingSource is the text of the file that places an anchor, with the
// modules beside it that it imports: a key the component renders may be held
// by its own helper (the Inbox's actions, sections/inbox/inboxAnswer.ts).
func placingSource(front, file string) string {
	path := filepath.Join(front, filepath.FromSlash(file))
	src, _ := os.ReadFile(path)
	var b strings.Builder
	b.Write(src)
	for _, m := range relImport.FindAllStringSubmatch(string(src), -1) {
		for _, ext := range []string{".ts", ".tsx"} {
			if dep, err := os.ReadFile(filepath.Join(filepath.Dir(path), m[1]+ext)); err == nil {
				b.Write(dep)
			}
		}
	}
	return b.String()
}
