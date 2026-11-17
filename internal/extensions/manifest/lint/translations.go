// SPDX-License-Identifier: Apache-2.0

package contentlint

// translations.go — the one translation lint, for every kind.
//
// A kind declares the texts a person reads (`translatable`, contribapi
// translations.go); an entry carries their other languages in its
// `translations` layer (`<name>.i18n.yaml`). This lint judges the two
// together, for any kind, from the kind's declaration alone:
//
//   - translation-unknown (error): a translation names a text the kind does
//     not declare, or one the entry does not have (a step id it lacks) — it
//     would never show;
//   - translation-source (error): a translation into the language the entry
//     is written in (the entry itself is that language);
//   - translation-language-invalid / translations-invalid (error): a key that
//     is not a two-letter language code, a file that does not decode;
//   - translation-missing (warn): a supported language (profile
//     SupportedLanguages, the one list) other than the source leaves texts
//     out — the reader sees the source text there.
//
// The source language is the entry's `source_lang`, else its pack's
// (sidecar-info `sourceLang`, from `embed/pack.yaml`), else English.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// LintTranslations lints one entry's translations layer (nil: it has none)
// against the texts its kind declares translatable and its data layer holds.
// packLang is its pack's source language ("" = English).
func LintTranslations(paths []contribapi.TranslatablePath, data, translations []byte, packLang string) []verify.LintFinding {
	source := contribapi.SourceTexts(paths, data)
	var t contribapi.Translations
	if len(translations) > 0 {
		if len(paths) == 0 {
			return []verify.LintFinding{errf("translation-unknown", "the entry carries translations, but its kind declares nothing translatable")}
		}
		var err error
		if t, err = contribapi.ParseTranslations(translations); err != nil {
			return []verify.LintFinding{errf("translations-invalid", "%v", err)}
		}
	}
	if len(source) == 0 && len(t.Languages) == 0 {
		return nil
	}
	var f []verify.LintFinding
	srcLang := contribapi.EntrySourceLang(t, packLang)
	if t.SourceLang != "" && !contribapi.IsLanguageCode(t.SourceLang) {
		f = append(f, errf("translation-language-invalid", "source_lang %q is not a two-letter code", t.SourceLang))
	}
	texts := map[string]map[string]string{}
	for _, lang := range sortedKeys(t.Languages) {
		switch {
		case !contribapi.IsLanguageCode(lang):
			f = append(f, errf("translation-language-invalid", "language %q is not a two-letter code", lang))
			continue
		case lang == srcLang:
			f = append(f, errf("translation-source", "`%s` is the language the entry is written in; translate into other languages only (or declare source_lang)", lang))
			continue
		}
		got, unknown := contribapi.FlattenTranslation(paths, t.Languages[lang])
		for _, u := range unknown {
			f = append(f, errf("translation-unknown", "%s: %q is not a translatable text of the kind (translatable: %s)", lang, u, declared(paths)))
		}
		for _, p := range sortedKeys(got) {
			if _, ok := source[p]; !ok {
				f = append(f, errf("translation-unknown", "%s: %q translates a text the entry does not have", lang, p))
			}
		}
		texts[lang] = got
	}
	for _, lang := range profileapi.SupportedLanguages {
		if lang == srcLang || len(source) == 0 {
			continue
		}
		var missing []string
		for _, p := range sortedKeys(source) {
			if texts[lang][p] == "" {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			f = append(f, warnf("translation-missing", "%s: %d text(s) not translated (%s); the %s shows", lang, len(missing), strings.Join(missing, ", "), srcLang))
		}
	}
	return f
}

// LintTranslationsFiles lints an entry on disk: its YAML file and the
// `<name>.i18n.yaml` beside it (nil when there is none), by its kind.
func LintTranslationsFiles(paths []contribapi.TranslatablePath, entryYAML, i18nYAML []byte, packLang string) []verify.LintFinding {
	data, err := yamlToJSON(entryYAML)
	if err != nil {
		return nil // the entry's own lint names an unreadable file
	}
	var tr []byte
	if i18nYAML != nil {
		if tr, err = yamlToJSON(i18nYAML); err != nil {
			return []verify.LintFinding{errf("translations-invalid", "%v", err)}
		}
	}
	return LintTranslations(paths, data, tr, packLang)
}

// PackSourceLangs is the source language each pack of a manifest declares,
// by provider: its sidecar-info's `sourceLang` ("" = English). An entry is
// written in its own provider's language — never another pack's: a
// `system:` entry the manifest carries (no sidecar-info) is English.
func PackSourceLangs(m *mbapi.Manifest) map[string]string {
	out := map[string]string{}
	if m == nil {
		return out
	}
	for _, c := range m.Contributions {
		if c.Type != contribapi.SidecarInfoKind {
			continue
		}
		var info struct {
			SourceLang string `json:"sourceLang"`
		}
		if json.Unmarshal(c.Data(), &info) == nil && info.SourceLang != "" {
			if _, seen := out[c.Provider]; !seen {
				out[c.Provider] = info.SourceLang
			}
		}
	}
	return out
}

func yamlToJSON(raw []byte) ([]byte, error) {
	var tree any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return json.Marshal(tree)
}

func declared(paths []contribapi.TranslatablePath) string {
	s := make([]string, len(paths))
	for i, p := range paths {
		s[i] = p.String()
	}
	return strings.Join(s, ", ")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func errf(rule, format string, a ...any) verify.LintFinding {
	return verify.LintFinding{Rule: rule, Message: fmt.Sprintf(format, a...), Severity: verify.SeverityError}
}

func warnf(rule, format string, a ...any) verify.LintFinding {
	return verify.LintFinding{Rule: rule, Message: fmt.Sprintf(format, a...), Severity: verify.SeverityWarn}
}
