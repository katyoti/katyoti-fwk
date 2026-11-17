// SPDX-License-Identifier: Apache-2.0

package guide

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// Everything a person reads while following a recipe is in their language
// : every shipped recipe has its translations file, and in it
// for the other language, its name, its description and the text of every
// step that speaks - so no step falls back on the language it was written in.
func TestEveryRecipeIsTranslated(t *testing.T) {
	root := repoRoot(t)
	var recipes []string
	for _, pattern := range []string{
		filepath.Join(root, "app", "*", "embed", "guide", "*.yaml"),
		filepath.Join(root, "internal", "extensions", "*", "contrib", "embed", "guide", "*.yaml"),
	} {
		got, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range got {
			if !strings.HasSuffix(f, ".i18n.yaml") {
				recipes = append(recipes, f)
			}
		}
	}
	if len(recipes) == 0 {
		t.Fatal("no shipped recipe found")
	}
	type stepText struct {
		ID  string `yaml:"id"`
		Say string `yaml:"say"`
	}
	type recipe struct {
		Name    string     `yaml:"name"`
		Concept bool       `yaml:"concept"`
		Body    string     `yaml:"body"`
		Steps   []stepText `yaml:"steps"`
	}
	type translation struct {
		Name        string            `yaml:"name"`
		Description string            `yaml:"description"`
		Body        string            `yaml:"body"`
		Steps       map[string]string `yaml:"steps"`
	}
	for _, f := range recipes {
		rel, _ := filepath.Rel(root, f)
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var r recipe
		if err := config.DecodeBytes(raw, marshal.YAML, &r); err != nil || r.Name == "" {
			continue // not a recipe (a guide's other files)
		}
		tr, err := os.ReadFile(strings.TrimSuffix(f, ".yaml") + ".i18n.yaml")
		if err != nil {
			t.Errorf("%s: no translations file beside it", rel)
			continue
		}
		var langs map[string]any
		if err := config.DecodeBytes(tr, marshal.YAML, &langs); err != nil {
			t.Errorf("%s: its translations do not decode: %v", rel, err)
			continue
		}
		found := 0
		for lang, block := range langs {
			if lang == "source_lang" {
				continue
			}
			found++
			body, _ := marshal.Marshal(block, marshal.YAML)
			var x translation
			if err := config.DecodeBytes(body, marshal.YAML, &x); err != nil {
				t.Errorf("%s (%s): %v", rel, lang, err)
				continue
			}
			if x.Name == "" || x.Description == "" {
				t.Errorf("%s (%s): its name or description is not translated", rel, lang)
			}
			// A concept document is read in the person's language.
			if r.Concept && strings.TrimSpace(r.Body) != "" && strings.TrimSpace(x.Body) == "" {
				t.Errorf("%s (%s): the concept document's body is not translated", rel, lang)
			}
			for _, s := range r.Steps {
				if s.Say != "" && strings.TrimSpace(x.Steps[s.ID]) == "" {
					t.Errorf("%s (%s): step %q is not translated", rel, lang, s.ID)
				}
			}
		}
		if found == 0 {
			t.Errorf("%s: its translations file holds no language", rel)
		}
		if _, fr := langs["fr"]; r.Concept && !fr {
			t.Errorf("%s: a concept document is given in French and English first: no fr", rel)
		}
	}
}
