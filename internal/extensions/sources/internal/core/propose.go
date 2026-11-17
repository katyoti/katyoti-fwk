// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// proposeTimeout bounds a connector's proposal: a wizard waits on it.
const proposeTimeout = 90 * time.Second

// Propose runs one connector's propose method for a carrier, in its own
// source location for the connector.
func (s *Service) Propose(ctx context.Context, in api.ProposeInput) (api.ConnectorProposeOutput, error) {
	c, err := s.connector(ctx, strings.TrimSpace(in.Connector))
	if err != nil {
		return api.ConnectorProposeOutput{}, err
	}
	if c.Propose == "" {
		return api.ConnectorProposeOutput{}, fmt.Errorf("%w: the connector %s proposes nothing", cwmapi.ErrInvalidInput, c.Name)
	}
	req := api.ConnectorProposeInput{Terms: in.Terms, Text: strings.TrimSpace(in.Text)}
	if in.Object != "" {
		if req.Location, req.Root, err = s.carrierLocation(ctx, in.Object, c.Name); err != nil {
			return api.ConnectorProposeOutput{}, err
		}
		// The carrier's source's own keywords and bound.
		var kws []string
		kws, req.Max = s.searchSettings(ctx, in.Object, c.Name)
		req.Terms = withKeywords(req.Terms, kws)
		// A connector declaring propose_from also proposes from what the
		// carrier's source of the other connector kept.
		if c.ProposeFrom != nil {
			cwm, err := application.UseService[cwmapi.CWMService](ctx)
			if err != nil {
				return api.ConnectorProposeOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
			}
			if req.ProposeFrom, err = keptItems(ctx, cwm, in.Object, c.ProposeFrom); err != nil {
				return api.ConnectorProposeOutput{}, err
			}
		}
	}
	if len(req.Terms) == 0 && req.Text == "" && (req.ProposeFrom == nil || len(req.ProposeFrom.Values) == 0) {
		return api.ConnectorProposeOutput{}, fmt.Errorf("%w: sources.Propose: %w", cwmapi.ErrInvalidInput, api.ErrNoContext)
	}
	out, err := call[api.ConnectorProposeOutput](ctx, c.Propose, proposeTimeout, req)
	if err != nil {
		return api.ConnectorProposeOutput{}, err
	}
	out = bounded(out, req.Max)
	if out.Items == nil {
		out.Items = []api.ProposedItem{}
	}
	return out, nil
}

// ProposalWizard answers the page of a wizard proposing items from every
// chosen source: one step per source, its picker pre-ticked with that
// connector's proposal. The wizard is the page DSL's - its
// steps page nodes -, so no screen holds code per connector.
func (s *Service) ProposalWizard(ctx context.Context, in api.ProposalWizardInput) (api.ProposalWizardOutput, error) {
	list, err := s.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		return api.ProposalWizardOutput{}, err
	}
	byName := map[string]api.ConnectorDecl{}
	for _, c := range list.Connectors {
		byName[c.Name] = c
	}
	// No connector named: every one a picker can search - pre-ticked where
	// it proposes, empty (searched by hand) where it cannot.
	names := in.Connectors
	if len(names) == 0 {
		for _, c := range list.Connectors {
			if picks(c) {
				names = append(names, c.Name)
			}
		}
	}
	for _, name := range names {
		if _, ok := byName[name]; !ok {
			return api.ProposalWizardOutput{}, fmt.Errorf("%w: no source connector %q", cwmapi.ErrInvalidInput, name)
		}
	}
	// Every source proposes at once: the wizard waits on
	// the slowest, never on their sum.
	type proposal struct {
		got api.ConnectorProposeOutput
		err error
	}
	proposals := make([]proposal, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		if c := byName[name]; picks(c) && c.Propose != "" {
			wg.Add(1)
			go func(i int, name string) {
				defer wg.Done()
				proposals[i].got, proposals[i].err = s.Propose(ctx, api.ProposeInput{Connector: name, Object: in.Object, Terms: in.Terms, Text: in.Text})
			}(i, name)
		}
	}
	wg.Wait()
	var out api.ProposalWizardOutput
	steps := []any{} // never null: the wizard reads its length
	for i, name := range names {
		c := byName[name]
		if !picks(c) {
			continue // not a picker: nothing to pre-tick
		}
		view := pickerView(c)
		selected := []any{}
		if c.Propose != "" {
			got, err := proposals[i].got, proposals[i].err
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", view.Label, err))
			} else {
				// The searches that failed, the others' items kept
				//
				for _, e := range got.Errors {
					out.Errors = append(out.Errors, fmt.Sprintf("%s: %s", view.Label, e))
				}
			}
			for _, it := range got.Items {
				item := map[string]any{"key": it.Key, "title": it.Title}
				if it.Parent != "" {
					item["parent"] = it.Parent
				}
				if len(it.Badges) > 0 {
					item["badges"] = it.Badges
				}
				if it.Why != "" {
					item["why"] = it.Why
				}
				selected = append(selected, item)
			}
		}
		// Each step holds what is ticked: ticked or unticked
		// in place, the choice kept by Hold as it changes -
		// « Suivant » alone carries it -, never read until Apply.
		picker := map[string]any{"kind": "picker", "id": name, "args": view.Args, "layout": view.Layout, "selected": selected,
			"tickable": true, "apply": api.HoldURI, "applyOn": "change"}
		// A search and a browse only where the connector declares them
		// : a from-only connector shows what it proposes.
		if view.Search != "" {
			picker["search"] = view.Search
		}
		if view.Browse != "" {
			picker["browse"] = view.Browse
		}
		if in.Object != "" {
			picker["object"] = in.Object
		}
		if len(view.Badges) > 0 {
			picker["badges"] = view.Badges
		}
		// Its item kinds - their labels and slots.
		if len(view.Kinds) > 0 {
			picker["kinds"] = view.Kinds
		}
		// Whether its source is configured and reachable: a banner when it is
		// not, « Réessayer » when it cannot be reached.
		if view.Status != "" {
			picker["status"] = view.Status
		}
		step := map[string]any{"id": name, "title": view.Label, "node": picker}
		if view.Derived != "" {
			step["description"] = view.Derived
		}
		steps = append(steps, step)
	}
	// The wizard's schema holds no field: its steps are the pickers, each
	// keeping its own selection.
	schema := map[string]any{
		"name": "source-proposal", "version": 1, "root": "Proposal",
		"types": map[string]any{"Proposal": map[string]any{"type": "object", "label": "Sources", "fields": map[string]any{},
			"extensions": []any{map[string]any{"type": "ui-wizard", "navigation": "linear", "steps": steps}}}},
	}
	out.Page = map[string]any{"kind": "wizard", "id": "source-proposal", "schema": schema}
	return out, nil
}

// ProposalPage answers ProposalWizard as a page region: the
// failed proposals said above the wizard, whose finish applies what the
// carrier holds - its knowledge base built from what is kept.
func (s *Service) ProposalPage(ctx context.Context, in api.ProposalPageInput) (api.ConnectorPanelOutput, error) {
	if strings.TrimSpace(in.Object) == "" {
		return api.ConnectorPanelOutput{}, fmt.Errorf("%w: sources.ProposalPage: object required", cwmapi.ErrInvalidInput)
	}
	got, err := s.ProposalWizard(ctx, api.ProposalWizardInput{Object: in.Object, Connectors: in.Connectors, Terms: in.Terms, Text: in.Text})
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	label := in.CompleteLabel
	if label == "" {
		label = "Appliquer"
	}
	got.Page["complete"] = map[string]any{"label": label, "uri": api.ApplyURI, "args": map[string]any{"id": in.Object}}
	if len(in.Complete) > 0 {
		got.Page["complete"] = in.Complete
	}
	if len(in.Cancel) > 0 {
		got.Page["cancel"] = in.Cancel
	}
	children := []any{}
	for _, e := range got.Errors {
		children = append(children, map[string]any{"kind": "text", "size": "sm", "color": "red", "content": e})
	}
	children = append(children, got.Page)
	return api.ConnectorPanelOutput{DSLTree: map[string]any{"kind": "vbox", "gap": "sm", "children": children}}, nil
}
