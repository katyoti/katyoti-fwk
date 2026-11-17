// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// ConnectorPanel is the page the « Sources » board draws beside the selected
// connector (the approved mock-up): its head, a banner with
// « Utiliser » while it is not used, then the generic picker bound to the
// carrier and the connector - search, « Parcourir », its status banner, the
// derived note, the selection with each item's state, « Enregistrer et
// lire » keeping the choice and reading it at once.
func (s *Service) ConnectorPanel(ctx context.Context, in api.ConnectorPanelInput) (api.ConnectorPanelOutput, error) {
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Connector) == "" {
		return api.ConnectorPanelOutput{}, fmt.Errorf("%w: sources.ConnectorPanel: id and connector required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ConnectorPanelOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, in.ID); err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	c, err := s.connector(ctx, in.Connector)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, in.ID)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	view := pickerView(c)
	source := ""
	for _, src := range srcs {
		if src.Via == "child" && src.Connector == c.Name {
			source = src.ID
		}
	}
	data := map[string]any{"label": view.Label, "count": pickedCount(srcs, source, c), "selected": []any{}}
	children := []any{
		map[string]any{"kind": "hbox", "gap": "sm", "children": []any{
			map[string]any{"kind": "heading", "text": view.Label, "order": 3},
			map[string]any{"kind": "badge", "text": "", "dataField": "$.count", "color": "teal"},
		}},
	}
	if source == "" {
		children = append(children, map[string]any{"kind": "section", "size": "sm", "title": "Cette source n'est pas utilisée par l'objet", "icon": "plug", "color": "gray",
			"children": []any{map[string]any{"kind": "button", "label": "Utiliser " + view.Label, "icon": "plus", "size": "xs", "variant": "filled",
				"action": map[string]any{"label": "Utiliser " + view.Label, "uri": "ktw-agents:sources/use", "args": map[string]any{"id": in.ID, "connector": c.Name}}}}})
		return api.ConnectorPanelOutput{DSLTree: map[string]any{"kind": "vbox", "gap": "md", "children": children}, Data: data, Title: view.Label}, nil
	}
	selected, err := s.pickedRows(ctx, source)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	data["selected"] = selected
	// The opened card shows the source's detail first - its state, when it
	// was read, what to do, the items it brought with their state
	// - then what to choose.
	detail, err := s.sourceDetail(ctx, cwm, source)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	data["detail"] = detail
	// The card is titled by the connector's label: no badge repeats it
	//
	children = append(children, sourceDetailTree(detail, false))
	picker := pickerNode(c, in.ID)
	children = append(children, picker)
	if form := locationForm(c, source); form != nil {
		loc, err := locationData(ctx, cwm, source)
		if err != nil {
			return api.ConnectorPanelOutput{}, err
		}
		data["location"] = loc
		children = append(children, form)
	}
	return api.ConnectorPanelOutput{DSLTree: map[string]any{"kind": "vbox", "gap": "md", "children": children}, Data: data, Title: view.Label}, nil
}

// locationForm is where a used source reads, as its connector declares it
// : the generic form drawn from the connector's location
// data-type - its ui-form groups, its secrets (string@password) and its
// folded advanced groups are the data-type's own hints, nothing here names a
// field - bound to the source's location and kept through LocationSave. A
// connector declaring no location has none, nor one with a picker: its
// selection is the one truth of what the source reads, never asked again in
// a location field - a picker that selects: one declaring
// only its words (unit, gender) keeps the form.
func locationForm(c api.ConnectorDecl, source string) map[string]any {
	if strings.TrimSpace(c.Location) == "" || source == "" || c.Picker.Selects() {
		return nil
	}
	return map[string]any{"kind": "section", "size": "sm", "title": "Où elle lit", "icon": "map-pin", "color": "gray",
		"children": []any{map[string]any{"kind": "form", "id": "location-" + c.Name, "schema": c.Location, "dataField": "$.location", "hideTitle": true,
			"submit": map[string]any{"label": "Enregistrer", "uri": api.LocationSaveURI, "args": map[string]any{"id": source, "location": "${value}"}}}}}
}

// LocationSave keeps a source's location from its connector's form
// ; the source's save gate - its connector's check - refuses
// one it cannot read.
func (s *Service) LocationSave(ctx context.Context, in api.LocationSaveInput) (api.LocationSaveOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.LocationSaveOutput{}, fmt.Errorf("%w: sources.LocationSave: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.LocationSaveOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	f, module, err := sourceFields(ctx, cwm, in.ID)
	if err != nil {
		return api.LocationSaveOutput{}, err
	}
	f.Location = api.KeepReservedLocation(f.Location, in.Location)
	if err := s.recordSource(ctx, cwm, module, in.ID, f); err != nil {
		return api.LocationSaveOutput{}, err
	}
	return api.LocationSaveOutput{OK: true, Message: "Lieu enregistré : la prochaine synchronisation lit là."}, nil
}

// pickedRows is a source's choice as its picker binds it: each item picked or
// held, with its state and why, and where a tree picker places it - the item
// it sits under and whether its descendants come with it.
func (s *Service) pickedRows(ctx context.Context, source string) ([]any, error) {
	picked, err := s.SourcePicked(ctx, api.SourcePickedInput{ID: source})
	if err != nil {
		return nil, err
	}
	selected := make([]any, 0, len(picked.Selected))
	for _, p := range picked.Selected {
		item := map[string]any{"key": p.Key, "title": p.Title}
		if p.State != "" {
			item["state"] = p.State
		}
		if p.Why != "" {
			item["why"] = p.Why
		}
		if p.Parent != "" {
			item["parent"] = p.Parent
		}
		if p.WithChildren {
			item["with_children"] = true
		}
		selected = append(selected, item)
	}
	return selected, nil
}

// pickerNode is the generic picker of a connector bound to a carrier, as its
// connector declares it. It searches on Enter or its search icon
// : no live search, the hint saying so.
func pickerNode(c api.ConnectorDecl, carrier string) map[string]any {
	view := pickerView(c)
	picker := map[string]any{"kind": "picker", "id": "source-" + c.Name, "object": carrier,
		"args": map[string]any{"connector": c.Name, "read": true}, "layout": view.Layout, "apply": "ktw-agents:sources/hold",
		"columns": true, "hint": "Tapez votre recherche puis Entrée ou la loupe.",
		"dataField": "$.selected", "labels": map[string]any{"apply": "Enregistrer et lire", "empty": "Rien de sélectionné. Cherchez et ajoutez."}}
	if _, ok := view.Badges["ai"]; ok {
		picker["removeBadge"] = "ai"
	}
	// A search and a browse only where the connector declares them
	//
	if view.Search != "" {
		picker["search"] = view.Search
	}
	if view.Browse != "" {
		picker["browse"] = view.Browse
	}
	if view.Status != "" {
		picker["status"] = view.Status
	}
	if view.Derived != "" {
		picker["derived"] = view.Derived
	}
	if len(view.Badges) > 0 {
		picker["badges"] = view.Badges
	}
	// Its item kinds - their labels and slots.
	if len(view.Kinds) > 0 {
		picker["kinds"] = view.Kinds
	}
	// A check-box tree speaks its unit (« avec les sous-pages », « cette
	// page seule », « incluse »), agreeing with its declared gender.
	if view.Layout == "tree" {
		labels := picker["labels"].(map[string]any)
		for k, v := range treeLabels(view.Unit, c.Picker.Feminine()) {
			labels[k] = v
		}
	}
	// Its scopes as chips, bound to the carrier.
	if scopes := pickerScopes(c); scopes != nil {
		picker["scopes"] = scopes
	}
	return picker
}

// locationData is a source's location as its form binds it: an empty one
// while it has none.
func locationData(ctx context.Context, cwm cwmapi.CWMService, source string) (any, error) {
	f, _, err := sourceFields(ctx, cwm, source)
	if err != nil {
		return nil, err
	}
	if f.Location == nil {
		return map[string]any{}, nil
	}
	return f.Location, nil
}
