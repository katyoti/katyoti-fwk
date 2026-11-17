// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

// TestPublicationNamesWhatAndWhere: a publication publishes one object or one
// management view, through one connector or one converter - a study's
// publication and a planification view's both pass; one naming neither, or
// both, is refused.
func TestPublicationNamesWhatAndWhere(t *testing.T) {
	ok := []api.PublicationFields{
		{Object: "dossier-x.ST-001", Connector: "confluence", Settings: map[string]any{"space": "ARCHI", "parent": "123"}},
		{View: "stream", Scope: "KaTyoti", Converter: "ktw-x:x/publish-documents"},
	}
	for _, f := range ok {
		if err := f.Check(); err != nil {
			t.Errorf("%+v refused: %v", f, err)
		}
	}
	bad := []api.PublicationFields{
		{Connector: "confluence"},
		{Object: "x", View: "stream", Connector: "confluence"},
		{Object: "x"},
		{Object: "x", Connector: "confluence", Converter: "a:b/c"},
	}
	for _, f := range bad {
		if err := f.Check(); err == nil {
			t.Errorf("%+v accepted", f)
		}
	}
}

// TestPublicationDestination: a connector publication's destination is its
// connector with the space and parent page of its settings - the target a
// writing publish passes; no space or no parent page
// is no destination.
func TestPublicationDestination(t *testing.T) {
	f := api.PublicationFields{Object: "s", Connector: "confluence", Settings: map[string]any{"space": "ARCHI", "parent": "123", "labels": "x"}}
	got, ok := f.Destination()
	if !ok || got != (api.ReceiptTarget{Connection: "confluence", Space: "ARCHI", Parent: "123"}) {
		t.Errorf("Destination = %+v, %v", got, ok)
	}
	for _, s := range []map[string]any{{"space": "ARCHI"}, {"parent": "123"}, nil} {
		if _, ok := (api.PublicationFields{Connector: "confluence", Settings: s}).Destination(); ok {
			t.Errorf("settings %v gave a destination", s)
		}
	}
}
