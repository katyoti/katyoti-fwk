// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : the board's picker searches on Enter or its search icon
// - it sends no live search and its hint says Enter or the
// icon, never « deux lettres ».
func TestTheBoardPickerSearchesOnEnter(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	if _, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"}); err != nil {
		t.Fatal(err)
	}
	picker := func() map[string]any {
		t.Helper()
		got, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: "apps"})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range got.DSLTree["children"].([]any) {
			if n, ok := c.(map[string]any); ok && n["kind"] == "picker" {
				return n
			}
		}
		t.Fatalf("the board panel has no picker: %+v", got.DSLTree)
		return nil
	}

	on := picker()
	if _, ok := on["live"]; ok {
		t.Errorf("the picker sends no live search: %+v", on)
	}
	hint, _ := on["hint"].(string)
	if hint == "" || !containsAny(hint, "Entrée", "loupe") || containsAny(hint, "deux lettres") {
		t.Errorf("the hint says Enter or the search icon: %q", hint)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
