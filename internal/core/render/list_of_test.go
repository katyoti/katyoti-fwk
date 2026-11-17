// SPDX-License-Identifier: Apache-2.0

package render

import "testing"

// A CARD IS A VARIANT OF A DATA TYPE: a list names its
// elements' data type and the variant they are drawn as - its card by
// default - and needs no item template; a list with neither is refused.
func TestAListOfATypeNeedsNoItemTemplate(t *testing.T) {
	typed := ListOf([]any{map[string]any{"id": "RL-033"}}, "ktw-agents:/data-type/release-card", "")
	if typed["itemType"] != "ktw-agents:/data-type/release-card" || typed["item"] != nil {
		t.Fatalf("ListOf = %v, want the type and no template", typed)
	}
	if _, set := typed["itemVariant"]; set {
		t.Fatalf("ListOf with no variant set one: %v (the card is the drawer's default)", typed)
	}
	if v := ListOf(nil, "ktw-agents:/data-type/release-card", "row")["itemVariant"]; v != "row" {
		t.Fatalf("a variant named is %v, want row", v)
	}
	if err := ValidatePage(Node{"kind": "vbox", "children": []any{typed}}); err != nil {
		t.Fatalf("a list of a type is refused: %v", err)
	}
	if err := ValidatePage(Node{"kind": "list", "data": []any{}}); err == nil {
		t.Fatal("a list with neither an item template nor an item type passed")
	}
	if err := ValidatePage(List([]any{}, Node{"kind": "text", "content": "x"}, true)); err != nil {
		t.Fatalf("a list with its template is refused: %v", err)
	}
}
