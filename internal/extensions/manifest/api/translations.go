// SPDX-License-Identifier: Apache-2.0

package api

import contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

// TranslationsInput asks for a universe's translation bundle in one language.
type TranslationsInput struct {
	// Language is the language code wanted (en, fr).
	Language string `json:"language" desc:"Language code wanted (en or fr)" jsonschema:"required: true"`
}

// TranslationsOutput is a universe's translation bundle in one language: every
// translated text of every contribution whose kind declares translatable
// texts, keyed `<uri>#<path>` (contribapi.TranslationKey). A text absent from
// the bundle shows in the contribution's own language.
type TranslationsOutput struct {
	// Language is the language of the texts.
	Language string `json:"language" desc:"Language of the texts"`
	// Texts maps <uri>#<path> to the translated text.
	Texts map[string]string `json:"texts" desc:"Translated texts keyed <uri>#<path> (path: label, actions.<label>.label, steps.<id>.say)"`
}

// TranslationBundle builds the translation bundle of a set of manifests in
// `lang`: for every contribution whose kind (declared in the same manifests)
// lists translatable texts, its translations layer's texts in `lang`. A
// layer not carried inline is skipped — the kinds declare it resident, so a
// manifest built under them carries it.
func TranslationBundle(manifests []*Manifest, lang string) map[string]string {
	var all []contribapi.Contribution
	for _, m := range manifests {
		if m != nil {
			all = append(all, m.Contributions...)
		}
	}
	kinds := contribapi.KindsFrom(all)
	out := map[string]string{}
	for _, c := range all {
		ref, ok := c.Layers[contribapi.LayerTranslations]
		if !ok || len(ref.Inline) == 0 {
			continue
		}
		for k, v := range contribapi.BundleTexts(c.ResolvedURI(), kinds.Translatable(c.Type), ref.Inline, lang) {
			out[k] = v
		}
	}
	return out
}
