// SPDX-License-Identifier: Apache-2.0

package schemaapi_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	api "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

var themes = []map[string]any{
	{"id": "acme", "name": "acme", "description": "Neutral corporate look (Acme)", "hashtags": []any{"corporate", "acme"}, "data": map[string]any{"aspect": "16:9"}},
	{"id": "katyoti", "name": "katyoti", "description": "The KaTyoti look", "hashtags": []any{"default"}, "data": map[string]any{"aspect": "16:9"}},
	{"id": "demo", "name": "demo", "description": "Demo theme", "hashtags": []any{"demo"}, "data": map[string]any{"aspect": "4:3"}},
	{"id": "dup", "name": "demo", "description": "A second demo — dropped"},
	{"id": "nameless", "description": "No name — dropped"},
}

func spec(t *testing.T, raw map[string]any) api.OptionsFrom {
	t.Helper()
	o, err := api.ParseOptionsFrom(raw)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func values(opts []map[string]any) []string {
	var out []string
	for _, o := range opts {
		out = append(out, o["value"].(string))
	}
	return out
}

func TestParseOptionsFrom(t *testing.T) {
	o := spec(t, map[string]any{"source": "catalog", "kind": "deck-theme", "providers": []any{"ktw-docs"}, "limit": float64(3)})
	if o.Value != "id" || o.Label != "id" || o.Sort != "label" || o.Limit != 3 {
		t.Fatalf("defaults: %+v", o)
	}
	if !reflect.DeepEqual(o.Params, map[string]any{"kind": "deck-theme", "providers": []any{"ktw-docs"}}) {
		t.Fatalf("the source's own keys pass through as Params: %+v", o.Params)
	}
	for _, bad := range []any{
		"catalog", map[string]any{"kind": "x"}, map[string]any{"source": "catalog", "where": "x"},
		map[string]any{"source": "catalog", "limit": -1}, map[string]any{"source": 3},
	} {
		if _, err := api.ParseOptionsFrom(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}

func TestOptionsProjectSortFilter(t *testing.T) {
	o := spec(t, map[string]any{"source": "x", "value": "name", "label": "description", "description": "data.aspect"})
	opts := o.Options(themes)
	// Sorted by label; the duplicate value and the record without one dropped.
	if got := values(opts); !reflect.DeepEqual(got, []string{"demo", "acme", "katyoti"}) {
		t.Fatalf("values = %v", got)
	}
	if opts[0]["label"] != "Demo theme" || opts[0]["description"] != "4:3" {
		t.Fatalf("projection: %v", opts[0])
	}

	by := func(raw map[string]any) []string {
		raw["source"], raw["value"] = "x", "name"
		return values(spec(t, raw).Options(themes))
	}
	if got := by(map[string]any{"where": map[string]any{"data.aspect": "16:9"}, "sort": "value"}); !reflect.DeepEqual(got, []string{"acme", "katyoti"}) {
		t.Errorf("where on a nested path: %v", got)
	}
	if got := by(map[string]any{"where": map[string]any{"hashtags": "corporate"}}); !reflect.DeepEqual(got, []string{"acme"}) {
		t.Errorf("where on a list (contains): %v", got)
	}
	if got := by(map[string]any{"where": map[string]any{"name": []any{"demo", "katyoti"}}, "sort": "value"}); !reflect.DeepEqual(got, []string{"demo", "katyoti"}) {
		t.Errorf("where any-of: %v", got)
	}
	if got := by(map[string]any{"sort": "-value", "limit": 2}); !reflect.DeepEqual(got, []string{"katyoti", "demo"}) {
		t.Errorf("descending + limit: %v", got)
	}
}

func formSchema() *api.Schema {
	return &api.Schema{Name: "f", Root: "Form", Types: map[string]*api.Type{
		"Form": {Kind: api.KindObject, Fields: map[string]*api.Type{
			"theme": {Kind: api.KindString, Content: "text", ContentParams: map[string]any{
				"options_from": map[string]any{"source": "catalog", "kind": "deck-theme", "value": "name", "label": "description"},
			}},
			"broken": {Kind: api.KindString, Content: "text", ContentParams: map[string]any{
				"options_from": map[string]any{"source": "nowhere"},
			}},
			"subject": {Kind: api.KindString},
		}},
	}}
}

func TestResolveOptions(t *testing.T) {
	s := formSchema()
	var asked map[string]any
	catalog := api.OptionSourceFunc(func(_ context.Context, p map[string]any) ([]map[string]any, error) {
		asked = p
		return themes, nil
	})
	errs := api.ResolveOptions(context.Background(), s, map[string]api.OptionSource{"catalog": catalog})
	if asked["kind"] != "deck-theme" {
		t.Fatalf("the source got its params: %v", asked)
	}
	theme := s.Types["Form"].Fields["theme"]
	if got := values(asMaps(theme.ContentParams["options"])); !reflect.DeepEqual(got, []string{"demo", "acme", "katyoti"}) {
		t.Fatalf("options = %v", got)
	}
	if !reflect.DeepEqual(theme.Enum, []any{"demo", "acme", "katyoti"}) {
		t.Fatalf("enum = %v", theme.Enum)
	}
	// An unknown source: reported, the field left free text.
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), `Form.broken: options_from: no source "nowhere"`) {
		t.Fatalf("errs = %v", errs)
	}
	if b := s.Types["Form"].Fields["broken"]; b.Enum != nil || b.ContentParams["options"] != nil {
		t.Fatalf("a field without options must stay free text: %+v", b)
	}
}

func TestResolveOptionsSourceFailure(t *testing.T) {
	s := formSchema()
	failing := api.OptionSourceFunc(func(context.Context, map[string]any) ([]map[string]any, error) {
		return nil, errors.New("catalog unreachable")
	})
	errs := api.ResolveOptions(context.Background(), s, map[string]api.OptionSource{"catalog": failing, "nowhere": failing})
	if len(errs) != 2 || s.Types["Form"].Fields["theme"].Enum != nil {
		t.Fatalf("a failing source leaves the field as it was: errs=%v", errs)
	}
}

func asMaps(v any) []map[string]any {
	var out []map[string]any
	for _, x := range v.([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}
