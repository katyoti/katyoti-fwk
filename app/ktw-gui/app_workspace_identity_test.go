// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// TestTheHeaderSavesTheCoreAndLeavesTheBody: the header's save writes the
// object core — no capability group appears — and the body, which is its
// type's, is left as it is even when the save says nothing about it.
func TestTheHeaderSavesTheCoreAndLeavesTheBody(t *testing.T) {
	o := cwmapi.Object{ID: "m.x", Type: "note", Name: "X", Description: "about x", Tags: []string{"a"}, Body: "prose"}
	if err := setIdentityFrom(&o, map[string]any{"name": "Renamed", "description": "about x", "icon": "star", "tags": []any{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	if o.Name != "Renamed" || o.Icon != "star" || len(o.Tags) != 2 || o.Body != "prose" || len(o.Capabilities) != 0 {
		t.Fatalf("saved: %+v", o)
	}
}

// The detail carries the body and the type's declaration of it, on the type's
// section, so the pane shows it as the type says.
func TestTheDetailCarriesTheBodyAsItsTypeSays(t *testing.T) {
	resp := paneapi.DetailOutput{Object: cwmapi.Object{ID: "m.d", Type: "slide-deck", Name: "D", Icon: "star", Body: "# One"},
		Panels: []paneapi.ObjectPanel{{ID: "core:" + cwmapi.FieldsPanel, Name: "Slide deck", Kind: paneapi.PanelKindFields, Section: true,
			Capabilities: []string{cwmapi.FieldsPanel}, Body: &cwmapi.BodyDecl{Label: "Slides", Format: "deck", Panel: "deck"}}}}
	d := objectDetailView(resp)
	if d.Body != "# One" || d.Icon != "star" || len(d.Tabs) != 1 || d.Tabs[0].Body == nil ||
		d.Tabs[0].Body.Label != "Slides" || d.Tabs[0].Body.Panel != "deck" {
		t.Fatalf("detail: body %q icon %q tabs %+v", d.Body, d.Icon, d.Tabs)
	}
}

// A declared panel that is the object's header reaches the page as one
// : drawn above the tabs, never a tab the
// universe's view would hide.
func TestTheDetailKeepsAHeaderPanelAsTheHeader(t *testing.T) {
	resp := paneapi.DetailOutput{Object: cwmapi.Object{ID: "m.s", Type: "etude-existant", Name: "S"},
		Panels: []paneapi.ObjectPanel{{ID: "tab:etude-header", Name: "En-tête", Kind: "content", Header: true}, {ID: "tab:sources", Name: "Sources", Kind: "content"}}}
	d := objectDetailView(resp)
	if len(d.Tabs) != 2 || !d.Tabs[0].Header || d.Tabs[1].Header {
		t.Fatalf("tabs: %+v", d.Tabs)
	}
}

// The header's line: the times are the pane's derived ones (the journal's),
// and what the object was derived from is its outgoing derived-from edge — an
// incoming one, or another kind, says nothing.
func TestTheHeaderLineIsDerived(t *testing.T) {
	at := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	resp := paneapi.DetailOutput{Object: cwmapi.Object{ID: "m.x", Type: "note"}, CreatedAt: at, UpdatedAt: at.Add(time.Hour),
		Links: []paneapi.ObjectLink{
			{Kind: "relates-to", Direction: "out", TargetID: "m.y"},
			{Kind: string(cwmapi.EdgeTypeDerivedFrom), Direction: "in", TargetID: "m.z"},
			{Kind: string(cwmapi.EdgeTypeDerivedFrom), Direction: "out", TargetID: "docs/spec.md"},
		}}
	d := objectDetailView(resp)
	if d.CreatedAt != "2026-10-08T09:00:00Z" || d.UpdatedAt != "2026-10-08T10:00:00Z" || d.DerivedFrom != "docs/spec.md" {
		t.Fatalf("header line: created %q changed %q derived from %q", d.CreatedAt, d.UpdatedAt, d.DerivedFrom)
	}
	if d := objectDetailView(paneapi.DetailOutput{Object: cwmapi.Object{ID: "m.x"}}); d.CreatedAt != "" || d.UpdatedAt != "" || d.DerivedFrom != "" {
		t.Errorf("nothing known says nothing: %+v", d)
	}
}
