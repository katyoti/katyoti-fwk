// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A board laid out as cards validates with its card fields
// ValidatePage refuses a layout other than list or cards.
func TestABoardLaidOutAsCardsValidates(t *testing.T) {
	board := render.Node{"kind": "board", "select": "connector", "layout": "cards", "openLabel": "Choisir…", "backLabel": "← Toutes les sources",
		"moreLabel": "et {n} autres", "itemsMax": 5,
		"data": []any{map[string]any{"key": "confluence", "label": "Confluence", "state": map[string]any{"text": "2 erreurs", "color": "red"},
			"meta": "12 pages lues", "items": []any{"ERP Acme"}, "note": "Demandez l'accès."}},
		"actions": []render.Action{{Label: "Actualiser", URI: "ktw-agents:sources/source-sync", Args: map[string]any{"id": "${item.source}"}}},
		"panel":   render.Text("the panel")}
	if err := render.ValidatePage(board); err != nil {
		t.Fatalf("a cards board must validate: %v", err)
	}
	if err := render.ValidatePage(board.With("layout", "list")); err != nil {
		t.Fatalf("a list board must validate: %v", err)
	}
	if err := render.ValidatePage(board.With("layout", "tiles")); err == nil || !strings.Contains(err.Error(), "list or cards") {
		t.Errorf("a layout other than list or cards: %v", err)
	}
}
