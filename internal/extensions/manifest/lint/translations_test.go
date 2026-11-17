// SPDX-License-Identifier: Apache-2.0

package contentlint

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

const recipeYAML = `
name: Add a capability to my universe
description: Install a sidecar and wire it into this universe.
hashtags: [marketplace, add capability]
steps:
  - id: open
    say: Open the Marketplace from the ribbon.
  - id: install
    say: Press Install on the capability you want.
  - id: wired
    say: The capability is now wired.
`

const recipeI18n = `
fr:
  name: Ajouter une capacité à mon univers
  description: Installer un sidecar et le brancher sur cet univers.
  hashtags: [capacité, marketplace]
  steps:
    open: Ouvrez la Marketplace depuis le ruban.
    install: {say: Appuyez sur Installer sur la capacité voulue.}
    wired: La capacité est branchée.
`

func systemPaths(t *testing.T, kind string) []contribapi.TranslatablePath {
	t.Helper()
	k, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	p := k.Translatable(kind)
	if len(p) == 0 {
		t.Fatalf("kind %s declares nothing translatable", kind)
	}
	return p
}

func ruleSet(fs []verify.LintFinding) map[string]verify.LintSeverity {
	out := map[string]verify.LintSeverity{}
	for _, f := range fs {
		out[f.Rule] = f.Severity
	}
	return out
}

// The guide recipe is the first user of the generic lint: its translations
// are judged from the kind's declaration alone — each rule tripped by the
// smallest change to a clean pair. A rule removed from the lint fails here.
func TestLintTranslations_GuideRecipe(t *testing.T) {
	paths := systemPaths(t, "guide-recipe")
	if fs := LintTranslationsFiles(paths, []byte(recipeYAML), []byte(recipeI18n), ""); len(fs) != 0 {
		t.Fatalf("clean translations: %+v", fs)
	}
	cases := []struct {
		rule, from, to string
		sev            verify.LintSeverity
	}{
		{"translation-unknown", "    wired: La", "    gone: La", verify.SeverityError},
		{"translation-unknown", "  name: Ajouter", "  title: Ajouter", verify.SeverityError},
		{"translation-unknown", "{say: Appuyez", "{tip: Appuyez", verify.SeverityError},
		{"translation-source", "fr:\n", "en:\n", verify.SeverityError},
		{"translation-language-invalid", "fr:\n", "fra:\n", verify.SeverityError},
		{"translation-missing", "    wired: La capacité est branchée.\n", "", verify.SeverityWarn},
		{"translation-missing", "  name: Ajouter une capacité à mon univers\n", "", verify.SeverityWarn},
		{"translations-invalid", "fr:\n", "fr: [\n", verify.SeverityError},
	}
	for _, c := range cases {
		if !strings.Contains(recipeI18n, c.from) {
			t.Fatalf("fixture no longer contains %q", c.from)
		}
		fs := LintTranslationsFiles(paths, []byte(recipeYAML), []byte(strings.Replace(recipeI18n, c.from, c.to, 1)), "")
		if got := ruleSet(fs); got[c.rule] != c.sev {
			t.Errorf("%s (%q to %q): got %v", c.rule, c.from, c.to, got)
		}
	}
	// No translations at all: one warning per missing language, never an error.
	fs := LintTranslationsFiles(paths, []byte(recipeYAML), nil, "")
	if len(fs) != 1 || fs[0].Rule != "translation-missing" || fs[0].Severity != verify.SeverityWarn {
		t.Fatalf("untranslated recipe: %+v", fs)
	}
}

// An object type is translated by label, description and each action's label
// (an action is keyed by its label); a pack written in French ships `en:`.
func TestLintTranslations_FrenchSourcePack(t *testing.T) {
	paths := systemPaths(t, "object-type")
	entry := []byte("name: dossier\nlabel: Dossier d'architecture\nactions:\n  - uri: x:tasks/a\n    label: Raffiner le périmètre\n  - uri: x:tasks/b\n")
	en := []byte("en:\n  label: Architecture file\n  actions:\n    Raffiner le périmètre: Refine the scope\n")
	if fs := LintTranslationsFiles(paths, entry, en, "fr"); len(fs) != 0 {
		t.Fatalf("French pack with en: %+v", fs)
	}
	// The same file in an English pack: `en` is then the source language.
	if got := ruleSet(LintTranslationsFiles(paths, entry, en, "")); got["translation-source"] != verify.SeverityError {
		t.Fatalf("en translation in an English pack: %v", got)
	}
	// source_lang on the entry overrides its pack.
	withSrc := append([]byte("source_lang: fr\n"), en...)
	if fs := LintTranslationsFiles(paths, entry, withSrc, ""); len(fs) != 0 {
		t.Fatalf("entry source_lang: %+v", fs)
	}
}

// A kind that declares nothing translatable refuses a translations layer.
func TestLintTranslations_UndeclaredKind(t *testing.T) {
	fs := LintTranslations(nil, []byte(`{"name":"x"}`), []byte(`{"fr":{"name":"y"}}`), "")
	if got := ruleSet(fs); got["translation-unknown"] != verify.SeverityError {
		t.Fatalf("translations on an untranslatable kind: %+v", fs)
	}
}

// : a concept document is an Explain me whose markdown body is read
// in the person's language - its translation is a text of the kind, and a
// language that leaves it out is told it falls back to English.
func TestLintTranslations_ConceptBodyIsTranslatable(t *testing.T) {
	paths := systemPaths(t, "guide-recipe")
	const concept = "name: Projects\ndescription: What a project is.\nconcept: true\nbody: |\n  # Projects\n  A project is a folder.\n"
	clean := LintTranslationsFiles(paths, []byte(concept), []byte("fr:\n  name: Projets\n  description: Ce qu'est un projet.\n  body: |\n    # Projets\n    Un projet est un dossier.\n"), "")
	if len(clean) != 0 {
		t.Fatalf("a translated concept body is refused: %v", clean)
	}
	missing := ruleSet(LintTranslationsFiles(paths, []byte(concept), []byte("fr:\n  name: Projets\n  description: Ce qu'est un projet.\n"), ""))
	if missing["translation-missing"] != verify.SeverityWarn {
		t.Fatalf("an untranslated concept body is not told: %v", missing)
	}
}

// : an entry is written in its own provider's language
// never another pack's. A French pack's manifest also carries `system:`
// entries (English, no sidecar-info): they miss French, never English.
func TestManifestTranslations_SourceLangIsTheEntrysProvider(t *testing.T) {
	mk := func(provider, typ, id, data string) contribapi.Contribution {
		c := contribapi.Contribution{Provider: provider, Type: typ, ID: id}
		c.SetData(json.RawMessage(data))
		return c
	}
	m := &mbapi.Manifest{Provider: "ktw-x", Contributions: []contribapi.Contribution{
		mk("ktw-x", "sidecar-info", "default", `{"name":"ktw-x","sourceLang":"fr"}`),
		mk("ktw-x", "guide-recipe", "own", `{"name":"Ajouter une capacité"}`),
		mk("system", "guide-recipe", "sys", `{"name":"Add a capability"}`),
	}}
	langs := map[string]string{}
	for _, lang := range []string{"en", "fr"} {
		for _, e := range ManifestTranslations(m, lang) {
			if len(e.Missing) > 0 {
				langs[e.Entry] += lang
			}
			if e.Entry == "system:/guide-recipe/sys" && e.SourceLang != "en" {
				t.Fatalf("system entry written in %q, want en", e.SourceLang)
			}
		}
	}
	if want := map[string]string{"ktw-x:/guide-recipe/own": "en", "system:/guide-recipe/sys": "fr"}; !reflect.DeepEqual(langs, want) {
		t.Fatalf("languages missing by entry: %v, want %v", langs, want)
	}
}
