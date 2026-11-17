// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"reflect"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The bundle reads the kinds and the translations layers of the manifests it
// is given — nothing else: a kind that declares no translatable text gives
// nothing, a language no entry translates gives nothing, an entry written in
// the language asked gives nothing (its own text shows).
func TestTranslationBundle(t *testing.T) {
	kind := func(name string, translatable ...string) contribapi.Contribution {
		d := contribapi.KindDeclaration{Name: name, Translatable: translatable}
		b, _ := json.Marshal(d)
		c := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: name}
		c.SetData(b)
		return c
	}
	entry := func(typ, id, tr string) contribapi.Contribution {
		c := contribapi.Contribution{Provider: "p", Type: typ, ID: id}
		c.SetData(json.RawMessage(`{}`))
		c.SetLayer(contribapi.LayerTranslations, []byte(tr), "application/json")
		return c
	}
	m := &Manifest{Provider: "p", Contributions: []contribapi.Contribution{
		kind("object-panel", "name", "description"),
		kind("plain"),
		entry("object-panel", "links", `{"fr":{"name":"Liens","bogus":"x"}}`),
		entry("object-panel", "apercu", `{"source_lang":"fr","en":{"name":"Overview"}}`),
		entry("plain", "x", `{"fr":{"name":"ignored"}}`),
	}}
	got := TranslationBundle([]*Manifest{m}, "fr")
	if want := map[string]string{"p:/object-panel/links#name": "Liens"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fr bundle: %v", got)
	}
	got = TranslationBundle([]*Manifest{m}, "en")
	if want := map[string]string{"p:/object-panel/apercu#name": "Overview"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("en bundle: %v", got)
	}
}
