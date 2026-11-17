// SPDX-License-Identifier: Apache-2.0

package api

// A SEARCH-BACKED PROPOSAL: a connector whose search finds
// items by free text proposes from a context by running it once per term -
// and once for the free text - and merging what they find. The merge lives
// here once; a connector's propose method hands it its search.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ProposeMax is the most items a search-backed proposal answers: the wizard
// pre-ticks every item proposed, so only the best are.
const ProposeMax = 20

// ErrNoContext is a proposal asked with no term and no text.
var ErrNoContext = errors.New("a proposal needs terms or a text")

// ProposeBySearch answers the items in.Terms and in.Text find through search:
// an item found by several searches is one item, its why naming what found
// it in mock-up v6's words (« mot-clé « x » », « mots-clés « x », « y » »;
// the free text « trouvée par « … » »), its score the share of the terms
// that found it (the free text adds to its why, never to the share it is
// measured against), the best first, at most in.Max (ProposeBound). With
// in.ProposeFrom, each kept item's title is searched too: an item only such
// a title finds carries By (the other connector) and ByItem (that title),
// its why left to the caller, who knows the other source's words. A search
// that fails is skipped and named in Errors; the proposal fails only when
// every search does.
func ProposeBySearch(ctx context.Context, in ConnectorProposeInput, search func(ctx context.Context, query string) ([]PickItem, error)) (ConnectorProposeOutput, error) {
	out := ConnectorProposeOutput{Items: []ProposedItem{}}
	var terms []string
	seen := map[string]bool{}
	for _, t := range in.Terms {
		if t = strings.TrimSpace(t); t != "" && !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			terms = append(terms, t)
		}
	}
	text := strings.TrimSpace(in.Text)
	if seen[strings.ToLower(text)] {
		text = ""
	}
	queries := append([]string{}, terms...)
	if text != "" {
		queries = append(queries, text)
	}
	// The kept items' titles come last: an item a term or the text found
	// keeps that why (mock-up v6: the keyword first).
	byQuery := map[string]string{} // a kept item's title -> its value
	if f := in.ProposeFrom; f != nil {
		for _, v := range f.Values {
			t := strings.TrimSpace(f.Title(v))
			if t == "" || seen[strings.ToLower(t)] || strings.EqualFold(t, text) {
				continue
			}
			seen[strings.ToLower(t)] = true
			byQuery[t] = v
			queries = append(queries, t)
		}
	}
	if len(queries) == 0 {
		return out, ErrNoContext
	}
	byKey := map[string]*ProposedItem{}
	foundBy := map[string][]string{}
	termHits := map[string]int{}
	var order []string
	failed := 0
	for i, q := range queries {
		if err := ctx.Err(); err != nil {
			return ConnectorProposeOutput{Items: []ProposedItem{}}, fmt.Errorf("proposal stopped: %w", err)
		}
		got, err := search(ctx, q)
		if err != nil {
			failed++
			out.Errors = append(out.Errors, fmt.Sprintf("« %s » : %v", q, err))
			continue
		}
		counted := map[string]bool{} // a key once per search, whatever its paging returns
		for _, it := range got {
			if counted[it.Key] {
				continue
			}
			counted[it.Key] = true
			if byKey[it.Key] == nil {
				byKey[it.Key] = &ProposedItem{PickItem: it}
				order = append(order, it.Key)
			}
			foundBy[it.Key] = append(foundBy[it.Key], q)
			if i < len(terms) {
				termHits[it.Key]++
			}
		}
	}
	if failed == len(queries) {
		return ConnectorProposeOutput{Items: []ProposedItem{}, Errors: out.Errors}, fmt.Errorf("every search failed: %s", strings.Join(out.Errors, " ; "))
	}
	for _, key := range order {
		it := byKey[key]
		var kws, others []string
		for _, q := range foundBy[key] {
			switch {
			case byQuery[q] != "":
			case q == text:
				others = append(others, q)
			default:
				kws = append(kws, q)
			}
		}
		if len(kws) == 0 && len(others) == 0 { // only a kept item's title
			// The connector of the source that kept that item.
			it.By, it.ByItem = in.ProposeFrom.ConnectorOf(byQuery[foundBy[key][0]]), foundBy[key][0]
		}
		it.Why = whyFound(kws, others)
		switch {
		case it.By != "": // proposed from another source's kept item
			it.Score = 0.5
			if len(terms) > 0 {
				it.Score = 0.5 / float64(len(terms))
			}
		case len(terms) == 0: // the text alone
			it.Score = 1
		case termHits[key] == 0: // found by the text only
			it.Score = 0.5 / float64(len(terms))
		default:
			it.Score = float64(termHits[key]) / float64(len(terms))
		}
		out.Items = append(out.Items, *it)
	}
	sort.SliceStable(out.Items, func(i, j int) bool { return out.Items[i].Score > out.Items[j].Score })
	if bound := ProposeBound(in.Max); len(out.Items) > bound {
		out.Items = out.Items[:bound]
	}
	return out, nil
}

// whyFound is a proposed item's why in mock-up v6's words: the keywords that
// found it (« mot-clé « x » », « mots-clés « x », « y » »), then the free
// text (« trouvée par « … » »); empty when neither did.
func whyFound(kws, text []string) string {
	quote := func(qs []string) string {
		out := make([]string, len(qs))
		for i, q := range qs {
			out[i] = "« " + q + " »"
		}
		return strings.Join(out, ", ")
	}
	var parts []string
	switch len(kws) {
	case 0:
	case 1:
		parts = append(parts, "mot-clé "+quote(kws))
	default:
		parts = append(parts, "mots-clés "+quote(kws))
	}
	if len(text) > 0 {
		if len(parts) == 0 {
			parts = append(parts, "Trouvée par "+quote(text))
		} else {
			parts = append(parts, "trouvée par "+quote(text))
		}
	}
	return strings.Join(parts, " · ")
}
