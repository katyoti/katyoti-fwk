// SPDX-License-Identifier: Apache-2.0

// Package core answers the Contributions panel: the project's provider token
// and folder from its settings (projectfile), its entries from the universe's
// catalog, grouped by kind.
package core

import (
	"context"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// Service implements api.ProjectContributionsService.
type Service struct{}

// New returns the service.
func New() *Service { return &Service{} }

// Panel lists what the universe publishes under the token of the project
// holding the object: the hub serves the project's contributions folder as a
// member of that name, so the catalog is the one
// answer — what is served, not what the folder holds.
func (s *Service) Panel(ctx context.Context, in api.ObjectInput) (api.PanelOutput, error) {
	if in.ID == "" {
		return api.PanelOutput{}, fmt.Errorf("project-contributions: id is required")
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.PanelOutput{}, err
	}
	r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: in.ID})
	if err != nil {
		return api.PanelOutput{}, err
	}
	f, err := projectfile.Read(r.ProjectRoot)
	if err != nil {
		return api.PanelOutput{}, err
	}
	out := api.PanelOutput{Provider: f.Contributions.Provider, Folder: f.Contributions.Root, Groups: []api.Group{}}
	cat, _ := application.UseService[mbapi.Catalog](ctx)
	entries, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat, Filter: mbapi.Filter{Providers: []string{out.Provider}}})
	if err != nil {
		return api.PanelOutput{}, err
	}
	out.Groups = Group(entries)
	out.Overrides = []api.Overridden{}
	if r, ok := any(cat).(mbapi.OverrideReader); ok {
		if list, err := r.Overrides(ctx, contribapi.ProjectOrigin(out.Provider)); err == nil {
			out.Overrides = OverriddenEntries(list)
		}
	}
	return out, nil
}

// OverriddenEntries groups overrides by entry, the entries and each one's
// layers sorted.
func OverriddenEntries(list []contribapi.Override) []api.Overridden {
	byURI := map[string][]string{}
	for _, o := range list {
		byURI[o.URI] = append(byURI[o.URI], o.Layer)
	}
	out := make([]api.Overridden, 0, len(byURI))
	for uri, layers := range byURI {
		sort.Strings(layers)
		out = append(out, api.Overridden{URI: uri, Layers: layers})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out
}

// Group sorts entries into one group per kind, the kinds and each kind's
// entries by name.
func Group(entries []contribapi.Contribution) []api.Group {
	byKind := map[string][]api.Entry{}
	for _, c := range entries {
		m := c.ResolvedMetadata()
		byKind[c.Type] = append(byKind[c.Type], api.Entry{URI: c.ResolvedURI(), ID: c.ID, Name: m.Name, Description: m.Description})
	}
	out := make([]api.Group, 0, len(byKind))
	for kind, es := range byKind {
		sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
		out = append(out, api.Group{Kind: kind, Entries: es})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}
