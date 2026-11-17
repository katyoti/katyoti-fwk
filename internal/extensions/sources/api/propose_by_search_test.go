// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : a search-backed proposal - one search per term and one
// for the text, merged into items with why and score, the best first,
// capped; a failing search skipped and named; every one failing an error.
func TestProposeBySearch(t *testing.T) {
	ctx := context.Background()
	index := map[string][]string{
		"facturation":                     {"P1", "P2"},
		"acme erp":                        {"P2"},
		"quel est le si de facturation ?": {"P3", "P2"},
	}
	var asked []string
	search := func(_ context.Context, q string) ([]api.PickItem, error) {
		asked = append(asked, q)
		if q == "C++" {
			return nil, errors.New("bad query")
		}
		var out []api.PickItem
		for _, k := range index[strings.ToLower(q)] {
			out = append(out, api.PickItem{Key: k, Title: "Page " + k})
		}
		return out, nil
	}

	out, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{
		Terms: []string{"facturation", " Acme ERP ", "FACTURATION", "C++"},
		Text:  "Quel est le SI de facturation ?",
	}, search)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 4 {
		t.Errorf("one search per distinct term and one for the text: %v", asked)
	}
	keys := []string{}
	for _, it := range out.Items {
		keys = append(keys, it.Key)
	}
	if strings.Join(keys, ",") != "P2,P1,P3" {
		t.Fatalf("the best first: %v", keys)
	}
	p2, p1, p3 := out.Items[0], out.Items[1], out.Items[2]
	if p2.Score != 2.0/3 || p2.Why != "mots-clés « facturation », « Acme ERP » · trouvée par « Quel est le SI de facturation ? »" {
		t.Errorf("found by two of three terms and the text: %+v", p2)
	}
	if p1.Score != 1.0/3 || p1.Why != "mot-clé « facturation »" {
		t.Errorf("found by one term: %+v", p1)
	}
	if p3.Score <= 0 || p3.Score >= p1.Score {
		t.Errorf("found by the text only, below any term: %+v", p3)
	}
	if len(out.Errors) != 1 || !strings.Contains(out.Errors[0], "« C++ »") {
		t.Errorf("the failing search is named: %v", out.Errors)
	}

	// The text alone.
	alone, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{Text: "facturation"}, search)
	if err != nil || len(alone.Items) != 2 || alone.Items[0].Score != 1 {
		t.Errorf("the text alone: %v %+v", err, alone.Items)
	}
	// No match: none, never null.
	none, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{Terms: []string{"astronomie"}}, search)
	if err != nil || none.Items == nil || len(none.Items) != 0 {
		t.Errorf("no match proposes nothing: %v %+v", err, none.Items)
	}
	// Every search failing.
	if _, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{Terms: []string{"C++"}}, search); err == nil {
		t.Error("every search failed and no error")
	}
	// No context.
	if _, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{Terms: []string{" "}}, search); !errors.Is(err, api.ErrNoContext) {
		t.Errorf("no term and no text: %v", err)
	}
	// A key a search returns twice counts once; the text equal to a term is
	// searched once.
	twice := func(_ context.Context, q string) ([]api.PickItem, error) {
		return []api.PickItem{{Key: "P1"}, {Key: "P1"}}, nil
	}
	asked = nil
	dup, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{Terms: []string{"a", "b"}, Text: "A"}, func(c context.Context, q string) ([]api.PickItem, error) {
		asked = append(asked, q)
		return twice(c, q)
	})
	if err != nil || len(dup.Items) != 1 || dup.Items[0].Score != 1 || dup.Items[0].Why != "mots-clés « a », « b »" || len(asked) != 2 {
		t.Errorf("a key returned twice, the text equal to a term: %v %+v %v", err, dup.Items, asked)
	}
	one, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{Terms: []string{"a"}}, twice)
	if err != nil || one.Items[0].Score != 1 {
		t.Errorf("one term, a key twice: score %v", one.Items)
	}
	// A cancelled context is the error.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := api.ProposeBySearch(cctx, api.ConnectorProposeInput{Terms: []string{"a"}}, twice); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled proposal: %v", err)
	}
	// Capped.
	many := func(_ context.Context, q string) ([]api.PickItem, error) {
		var out []api.PickItem
		for i := 0; i < api.ProposeMax+15; i++ {
			out = append(out, api.PickItem{Key: fmt.Sprint(q, i)})
		}
		return out, nil
	}
	capped, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{Terms: []string{"a", "b"}}, many)
	if err != nil || len(capped.Items) != api.ProposeMax {
		t.Errorf("at most ProposeMax: %v %d", err, len(capped.Items))
	}
}

// A connector declaring propose_from is handed what
// another source kept: each kept item's title is searched after the terms;
// an item only such a title finds carries By and ByItem, its why left to
// the caller; an item a term finds too keeps the keyword's why (mock-up v6:
// the keyword first). The kept items alone are context enough.
func TestProposeBySearchFromAnotherSource(t *testing.T) {
	ctx := context.Background()
	index := map[string][]string{"facturation": {"P1"}, "gestion erp": {"P1", "P2"}, "relation client": {}}
	var asked []string
	search := func(_ context.Context, q string) ([]api.PickItem, error) {
		asked = append(asked, q)
		var out []api.PickItem
		for _, k := range index[strings.ToLower(q)] {
			out = append(out, api.PickItem{Key: k})
		}
		return out, nil
	}
	kept := &api.FromItems{Connector: "apps", Attr: "code", Values: []string{"ERP", "CRM", "FAC"},
		Titles: map[string]string{"ERP": "Gestion ERP", "CRM": "Relation client", "FAC": "Facturation"}}
	out, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{Terms: []string{"facturation"}, ProposeFrom: kept}, search)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, "|") != "facturation|Gestion ERP|Relation client" {
		t.Errorf("the terms, then each kept title not already a term: %v", asked)
	}
	if len(out.Items) != 2 {
		t.Fatalf("items: %+v", out.Items)
	}
	p1, p2 := out.Items[0], out.Items[1]
	if p1.Key != "P1" || p1.Why != "mot-clé « facturation »" || p1.By != "" || p1.Score != 1 {
		t.Errorf("found by a keyword and a kept title, the keyword wins: %+v", p1)
	}
	if p2.Key != "P2" || p2.By != "apps" || p2.ByItem != "Gestion ERP" || p2.Why != "" || p2.Score <= 0 || p2.Score >= p1.Score {
		t.Errorf("found by a kept title only, marked by the other connector, below a keyword: %+v", p2)
	}
	alone, err := api.ProposeBySearch(ctx, api.ConnectorProposeInput{ProposeFrom: kept}, search)
	if err != nil || len(alone.Items) != 2 || alone.Items[0].By != "apps" {
		t.Errorf("the kept items alone: %v %+v", err, alone.Items)
	}
}

// : a source's own proposal bound caps what a search-backed
// proposal answers; none keeps ProposeMax.
func TestProposeBySearchHonoursTheSourcesBound(t *testing.T) {
	search := func(_ context.Context, q string) ([]api.PickItem, error) {
		var out []api.PickItem
		for i := 0; i < 30; i++ {
			out = append(out, api.PickItem{Key: fmt.Sprintf("%s-%d", q, i), Title: q})
		}
		return out, nil
	}
	for max, want := range map[int]int{0: api.ProposeMax, 3: 3, 25: 25} {
		out, err := api.ProposeBySearch(context.Background(), api.ConnectorProposeInput{Terms: []string{"x"}, Max: max}, search)
		if err != nil || len(out.Items) != want {
			t.Errorf("bound %d: %v %d items, want %d", max, err, len(out.Items), want)
		}
	}
}
