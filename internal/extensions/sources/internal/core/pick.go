// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// searchTimeout bounds a connector's search: a picker waits on it.
const searchTimeout = 45 * time.Second

// Pickers lists the contributed connectors that pick - by a search, by
// browsing, or from what another source kept - as the
// generic picker draws them: no screen holds code per
// connector - a connector contributed tomorrow is a picker everywhere.
func (s *Service) Pickers(ctx context.Context, in api.PickersInput) (api.PickersOutput, error) {
	list, err := s.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		return api.PickersOutput{}, err
	}
	out := api.PickersOutput{Connectors: []api.ConnectorPickerView{}}
	for _, c := range list.Connectors {
		if !picks(c) {
			continue
		}
		out.Connectors = append(out.Connectors, pickerView(c))
	}
	return out, nil
}

// pickerView is a connector as a picker: its search is sources' Search and
// its browse sources' SourceBrowse, with the connector laid over the
// picker's input - each only when the connector declares it.
func pickerView(c api.ConnectorDecl) api.ConnectorPickerView {
	v := api.ConnectorPickerView{Name: c.Name, Label: c.Label, Icon: c.Icon,
		Args: map[string]any{"connector": c.Name}, Layout: "list", Propose: c.Propose != "", Configure: c.Configure,
		Color: c.Color, Scopes: c.Scopes, From: c.From, Fetch: c.Fetch != "", Kinds: c.Kinds}
	if c.Search != "" {
		v.Search = api.SearchURI
	}
	if c.Browse != "" {
		v.Browse = api.SourceBrowseURI
	}
	if c.Test != "" {
		v.Status = api.StatusURI
	}
	if v.Label == "" {
		v.Label = c.Name
	}
	if p := c.Picker; p != nil {
		if p.Layout == "tree" {
			v.Layout = "tree"
		}
		v.Unit, v.Gender, v.Badges, v.Derived = p.Unit, p.Gender, p.Badges, p.Derived
	}
	return v
}

// Search runs one connector's search for a picker, in the carrier's own
// source location for it when it has one.
func (s *Service) Search(ctx context.Context, in api.SearchInput) (api.ConnectorSearchOutput, error) {
	var location map[string]any
	root := ""
	if in.Source != "" {
		// One source: its connector, in its location.
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return api.ConnectorSearchOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		f, module, err := sourceFields(ctx, cwm, in.Source)
		if err != nil {
			return api.ConnectorSearchOutput{}, err
		}
		in.Connector, location, root, in.Object = f.Connector, f.Location, projectRoot(ctx, cwm, module), ""
	}
	if strings.TrimSpace(in.Connector) == "" || strings.TrimSpace(in.Query) == "" {
		return api.ConnectorSearchOutput{}, fmt.Errorf("%w: sources.Search: a source, or a connector, and a query required", cwmapi.ErrInvalidInput)
	}
	c, err := s.connector(ctx, in.Connector)
	if err != nil {
		return api.ConnectorSearchOutput{}, err
	}
	if c.Search == "" && c.From != nil && c.Browse != "" {
		return s.searchTyped(ctx, c, in)
	}
	if c.Search == "" {
		return api.ConnectorSearchOutput{}, fmt.Errorf("%w: the connector %s declares no search", cwmapi.ErrInvalidInput, c.Name)
	}
	req := api.ConnectorSearchInput{Query: strings.TrimSpace(in.Query), Location: location, Root: root, Scopes: in.Scopes}
	carrier := in.Object
	if in.Object != "" {
		if req.Location, req.Root, err = s.carrierLocation(ctx, in.Object, c.Name); err != nil {
			return api.ConnectorSearchOutput{}, err
		}
	}
	if c.From != nil {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return api.ConnectorSearchOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		if in.Source != "" {
			carrier = carrierOfSource(ctx, cwm, in.Source)
		}
		if req.From, err = fromItems(ctx, cwm, carrier, c); err != nil {
			return api.ConnectorSearchOutput{}, err
		}
	}
	out, err := call[api.ConnectorSearchOutput](ctx, c.Search, searchTimeout, req)
	if err != nil {
		return api.ConnectorSearchOutput{}, err
	}
	if out.Items == nil {
		out.Items = []api.PickItem{}
	}
	return out, nil
}

// carrierLocation is the location of the carrier's own source reading
// through a connector, and the project's root; none when it has none.
func (s *Service) carrierLocation(ctx context.Context, carrierID, connector string) (map[string]any, string, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return nil, "", fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, carrierID); err != nil {
		return nil, "", err
	}
	srcs, err := sourcesOf(ctx, cwm, carrierID)
	if err != nil {
		return nil, "", err
	}
	if source := ownSource(srcs, connector); source != "" {
		f, module, err := sourceFields(ctx, cwm, source)
		if err != nil {
			return nil, "", err
		}
		return f.Location, projectRoot(ctx, cwm, module), nil
	}
	return nil, projectRoot(ctx, cwm, carrierID), nil
}

// statusTimeout bounds a connector's test: a picker waits on it.
const statusTimeout = 60 * time.Second

// Status says whether a connector is configured and reachable with no
// location, and where it is configured.
func (s *Service) Status(ctx context.Context, in api.StatusInput) (api.StatusOutput, error) {
	if in.Source != "" {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return api.StatusOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		f, _, err := sourceFields(ctx, cwm, in.Source)
		if err != nil {
			return api.StatusOutput{}, err
		}
		in.Connector = f.Connector
	}
	c, err := s.connector(ctx, strings.TrimSpace(in.Connector))
	if err != nil {
		return api.StatusOutput{}, err
	}
	out := api.StatusOutput{Connector: c.Name, Configure: c.Configure}
	if c.Test == "" {
		out.Configured, out.Message = true, "ce connecteur ne se vérifie pas avant une synchronisation"
		return out, nil
	}
	got, err := call[api.ConnectorTestOutput](ctx, c.Test, statusTimeout, api.ConnectorSyncInput{})
	if err != nil {
		out.Configured, out.Message = true, err.Error()
		return out, nil
	}
	out.Configured, out.Reachable, out.Message = !got.NotConfigured, got.OK, got.Message
	return out, nil
}

// SourcePicked is a source's choice as its picker holds it: the items picked
// in its selection, each with its state once applied, then those held
func (s *Service) SourcePicked(ctx context.Context, in api.SourcePickedInput) (api.SourcePickedOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.SourcePickedOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	f, _, err := sourceFields(ctx, cwm, in.ID)
	if err != nil {
		return api.SourcePickedOutput{}, err
	}
	out := api.SourcePickedOutput{ID: in.ID, Connector: f.Connector, Selected: []api.PickState{}}
	seen := map[string]bool{}
	for _, it := range api.SelectedItems(f.Selection) {
		p := api.PickState{Key: it.Key, Title: it.Title, Parent: it.Parent, WithChildren: it.WithChildren}
		if st, ok := f.States[it.Key]; ok {
			p.State, p.Why = st.State, st.Why
		}
		if p.Title == "" {
			p.Title = it.Key
		}
		seen[it.Key] = true
		out.Selected = append(out.Selected, p)
	}
	for _, it := range f.Held {
		if seen[it.Key] {
			continue
		}
		title := it.Title
		if title == "" {
			title = it.Key
		}
		out.Selected = append(out.Selected, api.PickState{Key: it.Key, Title: title, State: api.ItemWaiting, Why: "retenu, pas encore lu", Held: true,
			Parent: it.Parent, WithChildren: it.WithChildren})
	}
	return out, nil
}
