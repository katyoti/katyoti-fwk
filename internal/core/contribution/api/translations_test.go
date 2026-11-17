// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseTranslatablePath(t *testing.T) {
	good := map[string][]PathSegment{
		"label":                    {{Field: "label"}},
		"steps[id].say":            {{Field: "steps", Key: "id"}, {Field: "say"}},
		"actions[label].label":     {{Field: "actions", Key: "label"}, {Field: "label"}},
		"pane.badges[field].label": {{Field: "pane"}, {Field: "badges", Key: "field"}, {Field: "label"}},
		"types.*.fields.*.label":   {{Field: "types"}, {Any: true}, {Field: "fields"}, {Any: true}, {Field: "label"}},
	}
	for s, want := range good {
		got, err := ParseTranslatablePath(s)
		if err != nil || !reflect.DeepEqual(got.Segments, want) || got.String() != s {
			t.Errorf("%q: got %+v %v", s, got, err)
		}
	}
	for _, s := range []string{"", "a..b", "steps[].say", "steps[id]", "*.x", "x.*", "1x", "a b"} {
		if _, err := ParseTranslatablePath(s); err == nil {
			t.Errorf("%q parsed", s)
		}
	}
	// A position read as a text by one path and stepped into by another.
	if _, errs := ParseTranslatablePaths([]string{"label", "label.x"}); len(errs) != 1 {
		t.Errorf("contradiction not refused: %v", errs)
	}
}

// Nested fields and maps: a data-type's field labels, an object type's
// pane badges.
func TestSourceTextsAndFlatten_NestedAndMaps(t *testing.T) {
	paths, errs := ParseTranslatablePaths([]string{"types.*.fields.*.label", "types.*.fields.*.description", "pane.badges[field].label"})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	data := json.RawMessage(`{"types":{"T":{"fields":{"a":{"label":"A","description":"Da"},"b":{"type":"string"}}}},"pane":{"badges":[{"field":"state","label":"St"}]}}`)
	want := map[string]string{"types.T.fields.a.label": "A", "types.T.fields.a.description": "Da", "pane.badges.state.label": "St"}
	if got := SourceTexts(paths, data); !reflect.DeepEqual(got, want) {
		t.Fatalf("source texts: %v", got)
	}
	var tree map[string]any
	_ = json.Unmarshal([]byte(`{"types":{"T":{"fields":{"a":{"label":"a-fr","x":"?"}}}},"pane":{"badges":{"state":"shorthand"}}}`), &tree)
	texts, unknown := FlattenTranslation(paths, tree)
	if !reflect.DeepEqual(texts, map[string]string{"types.T.fields.a.label": "a-fr", "pane.badges.state.label": "shorthand"}) {
		t.Errorf("texts: %v", texts)
	}
	if strings.Join(unknown, ",") != "types.T.fields.a.x" {
		t.Errorf("unknown: %v", unknown)
	}
	for flat, wantFile := range map[string]string{
		"types.T.fields.a.label":            "types/T/fields/a|label",
		"pane.badges.state.label":           "pane/badges|state",
		"pane.badges.fields.severity.label": "pane/badges|fields.severity",
		"types.T.fields.a.b.label":          "types/T/fields/a.b|label",
		"nope":                              "",
	} {
		parents, leaf, ok := TranslationFilePath(paths, flat)
		got := ""
		if ok {
			got = strings.Join(parents, "/") + "|" + leaf
		}
		if got != wantFile {
			t.Errorf("file path of %s: %q, want %q", flat, got, wantFile)
		}
	}
}

func TestSourceTextsAndFlatten(t *testing.T) {
	paths, _ := ParseTranslatablePaths([]string{"name", "hashtags", "steps[id].say", "steps[id].tip"})
	data := json.RawMessage(`{"name":"N","hashtags":["a","b"],"steps":[{"id":"x","say":"X"},{"id":"y","say":"Y","tip":"T"},{"say":"no id"}]}`)
	want := map[string]string{"name": "N", "hashtags": "a, b", "steps.x.say": "X", "steps.y.say": "Y", "steps.y.tip": "T"}
	if got := SourceTexts(paths, data); !reflect.DeepEqual(got, want) {
		t.Fatalf("source texts: %v", got)
	}
	var tree map[string]any
	_ = json.Unmarshal([]byte(`{"name":"n","other":"?","steps":{"x":{"say":"x","bad":"?"},"y":"shorthand refused: two fields"}}`), &tree)
	texts, unknown := FlattenTranslation(paths, tree)
	if !reflect.DeepEqual(texts, map[string]string{"name": "n", "steps.x.say": "x"}) {
		t.Errorf("texts: %v", texts)
	}
	if strings.Join(unknown, ",") != "other,steps.x.bad,steps.y" {
		t.Errorf("unknown: %v", unknown)
	}
}

func TestBundleTexts(t *testing.T) {
	paths, _ := ParseTranslatablePaths([]string{"label", "states[name].name"})
	tr := []byte(`{"fr":{"label":"Étiquette","states":{"draft":"Brouillon"}}}`)
	got := BundleTexts("p:/workflow/w", paths, tr, "fr")
	want := map[string]string{"p:/workflow/w#label": "Étiquette", "p:/workflow/w#states.draft.name": "Brouillon"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle: %v", got)
	}
	if got := BundleTexts("p:/workflow/w", paths, tr, "de"); got != nil {
		t.Errorf("no German: %v", got)
	}
	// An entry written in the language asked needs no bundle entry.
	src := []byte(`{"source_lang":"fr","fr":{"label":"x"}}`)
	if got := BundleTexts("u", paths, src, "fr"); got != nil {
		t.Errorf("source language: %v", got)
	}
}

// A kind declaring translatable texts declares the layer that carries them,
// and only paths the grammar reads.
func TestKindsCheck_Translatable(t *testing.T) {
	d := complete("thing")
	d.Translatable = []string{"label", "bad path"}
	var msgs []string
	for _, f := range KindsFrom([]Contribution{kindEntry(t, d)}).Check() {
		msgs = append(msgs, f.Message)
	}
	all := strings.Join(msgs, "\n")
	if !strings.Contains(all, "bad path") || !strings.Contains(all, "no `translations` layer") {
		t.Fatalf("findings: %s", all)
	}
	d.Translatable = []string{"label"}
	d.Layers[LayerTranslations] = LayerDecl{MediaType: "application/json"}
	if fs := KindsFrom([]Contribution{kindEntry(t, d)}).Check(); len(fs) != 0 {
		t.Fatalf("complete translatable kind: %v", fs)
	}
	if got := KindsFrom([]Contribution{kindEntry(t, d)}).Translatable("thing"); len(got) != 1 || got[0].String() != "label" {
		t.Fatalf("Translatable: %v", got)
	}
}

// A data-type field's options: the list form gives each option's label and
// description keyed by its value; the short map form {value: label} holds no
// translatable text.
func TestSourceTextsAndFlatten_FieldOptions(t *testing.T) {
	paths, errs := ParseTranslatablePaths([]string{"types.*.fields.*.content.options[value].label", "types.*.fields.*.content.options[value].description"})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	data := json.RawMessage(`{"types":{"T":{"fields":{"k":{"content":{"options":[{"value":"svc","label":"Services","description":"D"},{"value":"host","label":"Hosts"}]}},"m":{"content":{"options":{"x":"Short"}}}}}}}`)
	want := map[string]string{"types.T.fields.k.content.options.svc.label": "Services", "types.T.fields.k.content.options.svc.description": "D", "types.T.fields.k.content.options.host.label": "Hosts"}
	if got := SourceTexts(paths, data); !reflect.DeepEqual(got, want) {
		t.Fatalf("source texts: %v", got)
	}
	tr := []byte(`{"fr":{"types":{"T":{"fields":{"k":{"content":{"options":{"svc":{"label":"Les services"},"host":{"label":"Les hôtes"},"nope":{"tip":"?"}}}}}}}}}`)
	got := BundleTexts("p:/data-type/t", paths, tr, "fr")
	wantFr := map[string]string{"p:/data-type/t#types.T.fields.k.content.options.svc.label": "Les services", "p:/data-type/t#types.T.fields.k.content.options.host.label": "Les hôtes"}
	if !reflect.DeepEqual(got, wantFr) {
		t.Fatalf("bundle: %v", got)
	}
}
