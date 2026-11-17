// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Every GUI namespace (`frontend/src/locales/<lang>/<ns>.json`) exists in every
// language with the same keys: English holds every key, and a language that
// lacks one shows English where the person chose French (modules/gui/i18n.md).
func TestLocales_SameKeysInEveryLanguage(t *testing.T) {
	root := filepath.Join("frontend", "src", "locales")
	langs, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]map[string][]string{} // lang → ns → keys
	for _, l := range langs {
		if !l.IsDir() {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(root, l.Name(), "*.json"))
		keys[l.Name()] = map[string][]string{}
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var tree map[string]any
			if err := json.Unmarshal(raw, &tree); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			ns := filepath.Base(f[:len(f)-len(".json")])
			keys[l.Name()][ns] = flattenKeys("", tree)
		}
	}
	en, ok := keys["en"]
	if !ok || len(en) == 0 {
		t.Fatal("no English locale")
	}
	for lang, nss := range keys {
		if lang == "en" {
			continue
		}
		for ns, want := range en {
			got, ok := nss[ns]
			if !ok {
				t.Errorf("%s: namespace %s missing", lang, ns)
				continue
			}
			if d := missingKeys(want, got); len(d) > 0 {
				t.Errorf("%s/%s.json lacks %v", lang, ns, d)
			}
			if d := missingKeys(got, want); len(d) > 0 {
				t.Errorf("%s/%s.json has keys English does not: %v", lang, ns, d)
			}
		}
		for ns := range nss {
			if _, ok := en[ns]; !ok {
				t.Errorf("%s: namespace %s has no English", lang, ns)
			}
		}
	}
}

func flattenKeys(prefix string, tree map[string]any) []string {
	var out []string
	for k, v := range tree {
		if sub, ok := v.(map[string]any); ok {
			out = append(out, flattenKeys(prefix+k+".", sub)...)
			continue
		}
		out = append(out, prefix+k)
	}
	sort.Strings(out)
	return out
}

// missingKeys is what a holds and b lacks.
func missingKeys(a, b []string) []string {
	have := map[string]bool{}
	for _, k := range b {
		have[k] = true
	}
	var out []string
	for _, k := range a {
		if !have[k] {
			out = append(out, k)
		}
	}
	return out
}
