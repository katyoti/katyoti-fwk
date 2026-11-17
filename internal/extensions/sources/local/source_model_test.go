// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// tickets is a connector contributed tomorrow: it declares the whole source
// model - who it is, its scopes, its fetch, what it reads from another
// source - and nothing else in KaTyoti changes for it.
const tickets = `name: tickets
label: Tickets
description: The tickets of the help desk.
icon: ticket
color: grape
sync: ktw-apps:tickets/source-sync
search: ktw-apps:tickets/search
fetch: ktw-apps:tickets/fetch
scopes: {unit: projet, list: ktw-apps:tickets/scopes, add: true}
from: {connector: apps, attr: code}
picker: {unit: ticket}
`

// : a source-connector declares who it is (label, icon
// colour), its scopes, its operations and what it reads from another source;
// the kind carries them to every view, and a search answers the item roles
// (path, detail, attrs, modified) as they are, in the scopes asked. A source
// is drawn by its connector's declared label, never its name.
func TestASourceDeclaresTheSourceModel(t *testing.T) {
	ctx, _, svc, asked := pickWorldWith(t, map[string]string{"tickets.yaml": tickets})

	list, err := svc.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		t.Fatal(err)
	}
	var decl api.ConnectorDecl
	for _, c := range list.Connectors {
		if c.Name == "tickets" {
			decl = c
		}
	}
	want := api.ConnectorDecl{Name: "tickets", Label: "Tickets", Description: "The tickets of the help desk.", Icon: "ticket", Color: "grape",
		Sync: "ktw-apps:tickets/source-sync", Search: "ktw-apps:tickets/search", Fetch: "ktw-apps:tickets/fetch",
		Scopes: &api.ConnectorScopes{Unit: "projet", List: "ktw-apps:tickets/scopes", Add: true},
		From:   &api.ConnectorFrom{Connector: "apps", Attr: "code"}, Picker: &api.ConnectorPicker{Unit: "ticket"}}
	if !reflect.DeepEqual(decl, want) {
		t.Fatalf("the declaration read back:\n got %+v\nwant %+v", decl, want)
	}

	pk, err := svc.Pickers(ctx, api.PickersInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var view *api.ConnectorPickerView
	for i := range pk.Connectors {
		if pk.Connectors[i].Name == "tickets" {
			view = &pk.Connectors[i]
		}
	}
	if view == nil || view.Label != "Tickets" || view.Icon != "ticket" || view.Color != "grape" || !view.Fetch ||
		view.Scopes == nil || view.Scopes.Unit != "projet" || !view.Scopes.Add || view.From == nil || view.From.Connector != "apps" || view.From.Attr != "code" {
		t.Fatalf("the tickets picker does not carry its source model: %+v", view)
	}

	got, err := svc.Search(ctx, api.SearchInput{Connector: "tickets", Query: "acme", Scopes: []string{"HELP", "OPS"}})
	if err != nil {
		t.Fatal(err)
	}
	if last := (*asked)[len(*asked)-1]; !reflect.DeepEqual(last.Scopes, []string{"HELP", "OPS"}) {
		t.Errorf("the search was not run in the scopes asked: %+v", last)
	}
	it := got.Items[0]
	if !reflect.DeepEqual(it.Path, []string{"Map"}) || it.Detail != "RDY · Actif" || it.Attrs["code"] != "RDY" || it.Modified == "" {
		t.Errorf("the item roles did not come through: %+v", it)
	}

	// A source is drawn by its connector's label - its name only when it
	// declares none.
	for _, c := range []string{"apps", "drive"} {
		if _, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: c}); err != nil {
			t.Fatal(err)
		}
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{}
	for _, s := range of.Sources {
		labels[s.Connector] = s.ConnectorLabel
	}
	if labels["apps"] != "Applications" || labels["drive"] != "drive" {
		t.Errorf("the sources are not drawn by their connector's label: %v", labels)
	}
}
