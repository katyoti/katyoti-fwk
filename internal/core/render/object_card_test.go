// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"strings"
	"testing"
)

// : the one object card is a DSL node filled from a row; a
// producer passes rows and the page validates.
func TestObjectCardsAreFilledFromRows(t *testing.T) {
	rows := []ObjectCardRow{{
		ID: "m.G-001", Title: "Ship the release", Type: "goal",
		State:  &CardBadge{ID: "lifecycle.state", Label: "accepted", Color: "green"},
		Badges: []CardBadge{{ID: "plan.priority", Label: "P1"}},
		Next:   &Action{ID: "refine", Label: "Affiner", URI: "ktw-agents:requests/refine", Args: map[string]any{"id": "${item.id}"}},
		Menu:   []Action{{ID: "archive", Label: "Retirer", URI: "ktw-agents:cwm/object-archive", Args: map[string]any{"id": "${item.id}"}}},
	}}
	open := &Action{ID: "open", Navigate: &Navigate{Object: "${item.id}"}}
	page := VBox(ObjectCards(rows, 0, "Nothing yet.", open), ObjectCard(rows[0], nil))
	if err := ValidatePage(roundTrip(t, page)); err != nil {
		t.Fatalf("an object card page is refused: %v", err)
	}
	raw, _ := json.Marshal(page)
	for _, want := range []string{`"kind":"object-card"`, `"dataField":"$.item"`, `"title":"Ship the release"`, `"state":{"id":"lifecycle.state"`, `"next":{"id":"refine"`, `"menu":[{"id":"archive"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the page lacks %s: %s", want, raw)
		}
	}
}

func TestAnObjectCardNeedsItsRow(t *testing.T) {
	if err := ValidatePage(map[string]any{"kind": "object-card"}); err == nil {
		t.Fatal("an object card with no row and no binding passed")
	}
	if err := ValidatePage(map[string]any{"kind": "object-card", "dataField": "row"}); err != nil {
		t.Fatalf("a bound object card is refused: %v", err)
	}
}
