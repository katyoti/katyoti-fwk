// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// The catalog source: a kind's entries in reach become records a form field
// chooses from — its picker fields, its id and its body.
func TestCatalogOptions(t *testing.T) {
	theme := func(provider, name, desc, aspect string) contribapi.Contribution {
		return contribapi.Contribution{
			Provider: provider, Type: "deck-theme", ID: name,
			Metadata: &contribapi.ContributionMeta{Name: name, Description: desc, Hashtags: []string{"theme"}},
			Layers:   contribapi.InlineData(json.RawMessage(`{"name":"` + name + `","aspect":"` + aspect + `"}`)),
		}
	}
	cat := &queryCatalog{entries: []contribapi.Contribution{
		theme("ktw-docs", "katyoti", "The KaTyoti look", "16:9"),
		theme("ktw-demo", "demo", "Demo theme", "4:3"),
		theme("ktw-acme", "acme", "Acme corporate", "16:9"),
		{Provider: "ktw-docs", Type: "deck-layout", ID: "cover", Layers: contribapi.InlineData(json.RawMessage(`{"name":"cover"}`))},
	}}
	src := CatalogOptions(Q{Reach: ReachUniverse, Catalog: cat})
	ctx := context.Background()

	records, err := src.Records(ctx, map[string]any{"kind": "deck-theme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("want the 3 themes, not the layout: %v", records)
	}
	var demo map[string]any
	for _, r := range records {
		if r["id"] == "demo" {
			demo = r
		}
	}
	if demo["uri"] != "ktw-demo:/deck-theme/demo" || demo["provider"] != "ktw-demo" || demo["name"] != "demo" ||
		demo["description"] != "Demo theme" || schemaapi.Lookup(demo, "data.aspect") != "4:3" {
		t.Fatalf("record = %v", demo)
	}

	// The whole chain a form field declares: kind + providers + where, then
	// value / label.
	spec, err := schemaapi.ParseOptionsFrom(map[string]any{
		"source": "catalog", "kind": "deck-theme", "providers": []any{"ktw-docs", "ktw-acme"},
		"where": map[string]any{"data.aspect": "16:9"}, "value": "name", "label": "description",
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err = src.Records(ctx, spec.Params)
	if err != nil {
		t.Fatal(err)
	}
	got := spec.Options(records)
	want := []map[string]any{
		{"value": "acme", "label": "Acme corporate"},
		{"value": "katyoti", "label": "The KaTyoti look"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %v", got)
	}

	if _, err := src.Records(ctx, map[string]any{}); err == nil {
		t.Fatal("a catalog source without a kind must be refused")
	}
	if _, err := src.Records(ctx, map[string]any{"kind": "deck-theme", "providers": 3}); err == nil {
		t.Fatal("providers must be names")
	}
}
