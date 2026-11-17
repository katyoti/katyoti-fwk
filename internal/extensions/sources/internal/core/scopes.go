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

// scopesTimeout bounds a connector's scopes list: a picker waits on it.
const scopesTimeout = 45 * time.Second

// Scopes answers a carrier's scopes for a connector: the
// connector's scopes list called with the carrier's own source location for
// it - the configuration's defaults and the scopes the carrier added, kept in
// the location's scopes field. A connector declaring no scopes has none.
func (s *Service) Scopes(ctx context.Context, in api.ScopesInput) (api.ConnectorScopesOutput, error) {
	if strings.TrimSpace(in.Connector) == "" || strings.TrimSpace(in.Object) == "" {
		return api.ConnectorScopesOutput{}, fmt.Errorf("%w: sources.Scopes: connector and object required", cwmapi.ErrInvalidInput)
	}
	c, err := s.connector(ctx, in.Connector)
	if err != nil {
		return api.ConnectorScopesOutput{}, err
	}
	out := api.ConnectorScopesOutput{Scopes: []api.Scope{}}
	if c.Scopes == nil || c.Scopes.List == "" {
		return out, nil
	}
	location, root, err := s.carrierLocation(ctx, in.Object, c.Name)
	if err != nil {
		return api.ConnectorScopesOutput{}, err
	}
	got, err := call[api.ConnectorScopesOutput](ctx, c.Scopes.List, scopesTimeout, api.ConnectorScopesInput{Location: location, Root: root})
	if err != nil {
		return api.ConnectorScopesOutput{}, err
	}
	if got.Scopes != nil {
		out.Scopes = got.Scopes
	}
	return out, nil
}

// ScopeOptions answers an opened source's scopes as the choices of its
// « Réglages » scopes row: the connector's scopes list
// called with the source's own location, each scope's key the value, its
// key and title the label (« ARCHI · Architecture SI »). A connector
// declaring no scopes list answers none: the row keeps its keys.
func (s *Service) ScopeOptions(ctx context.Context, in api.ScopeOptionsInput) (api.ScopeOptionsOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.ScopeOptionsOutput{}, fmt.Errorf("%w: sources.ScopeOptions: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ScopeOptionsOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	f, module, err := sourceFields(ctx, cwm, in.ID)
	if err != nil {
		return api.ScopeOptionsOutput{}, err
	}
	out := api.ScopeOptionsOutput{Options: []api.ScopeOption{}}
	c, err := s.connector(ctx, f.Connector)
	if err != nil {
		return api.ScopeOptionsOutput{}, err
	}
	if c.Scopes == nil || c.Scopes.List == "" {
		return out, nil
	}
	got, err := call[api.ConnectorScopesOutput](ctx, c.Scopes.List, scopesTimeout, api.ConnectorScopesInput{Location: f.Location, Root: projectRoot(ctx, cwm, module)})
	if err != nil {
		return api.ScopeOptionsOutput{}, err
	}
	seen := map[string]bool{}
	for _, sc := range got.Scopes {
		key := strings.TrimSpace(sc.Key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		label := key
		if t := strings.TrimSpace(sc.Title); t != "" && t != key {
			label = key + " · " + t
		}
		out.Options = append(out.Options, api.ScopeOption{Value: key, Label: label})
	}
	return out, nil
}

// ScopeAdd keeps a scope a carrier adds for a connector:
// its key appended, once, to the scopes field of the carrier's own source
// location, through the source's save gate - the next Scopes answers it.
func (s *Service) ScopeAdd(ctx context.Context, in api.ScopeAddInput) (api.ScopeAddOutput, error) {
	key := strings.TrimSpace(in.Scope)
	if strings.TrimSpace(in.Connector) == "" || strings.TrimSpace(in.Object) == "" || key == "" {
		return api.ScopeAddOutput{}, fmt.Errorf("%w: sources.ScopeAdd: connector, object and scope required", cwmapi.ErrInvalidInput)
	}
	c, err := s.connector(ctx, in.Connector)
	if err != nil {
		return api.ScopeAddOutput{}, err
	}
	if c.Scopes == nil || !c.Scopes.Add {
		return api.ScopeAddOutput{}, fmt.Errorf("%w: the connector %s lets no carrier add a scope", cwmapi.ErrInvalidInput, c.Name)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ScopeAddOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, in.Object); err != nil {
		return api.ScopeAddOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, in.Object)
	if err != nil {
		return api.ScopeAddOutput{}, err
	}
	source := ownSource(srcs, c.Name)
	if source == "" {
		return api.ScopeAddOutput{}, fmt.Errorf("%w: %s has no source of its own reading through %s: use it first", cwmapi.ErrInvalidInput, in.Object, c.Name)
	}
	f, module, err := sourceFields(ctx, cwm, source)
	if err != nil {
		return api.ScopeAddOutput{}, err
	}
	kept := scopeKeys(f.Location[api.LocationScopes])
	for _, k := range kept {
		if k == key {
			return api.ScopeAddOutput{OK: true, Message: fmt.Sprintf("%s est déjà parmi les endroits où chercher.", key)}, nil
		}
	}
	location := map[string]any{}
	for k, v := range f.Location {
		location[k] = v
	}
	keys := make([]any, 0, len(kept)+1)
	for _, k := range kept {
		keys = append(keys, k)
	}
	location[api.LocationScopes] = append(keys, key)
	f.Location = location
	if err := s.recordSource(ctx, cwm, module, source, f); err != nil {
		return api.ScopeAddOutput{}, err
	}
	return api.ScopeAddOutput{OK: true, Message: fmt.Sprintf("%s ajouté aux endroits où chercher.", key)}, nil
}

// ownSource is the carrier's own source reading through a connector: the
// first of its sources held as a child - the one Scopes reads and ScopeAdd
// writes, so an added scope shows in the next list; none
// when it has none.
func ownSource(srcs []api.SourceSummary, connector string) string {
	for _, src := range srcs {
		if src.Via == "child" && src.Connector == connector {
			return src.ID
		}
	}
	return ""
}

// scopeKeys reads the scope keys a location keeps: a list of keys, or a
// comma-separated string.
func scopeKeys(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, s := range strings.Split(t, ",") {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

// pickerScopes is a picker node's scopes for a connector
// declaring them: listed by Scopes, a scope added kept by ScopeAdd when the
// connector lets a carrier add one; none without a scopes list.
func pickerScopes(c api.ConnectorDecl) map[string]any {
	if c.Scopes == nil || c.Scopes.List == "" {
		return nil
	}
	out := map[string]any{"list": api.ScopesURI}
	if c.Scopes.Unit != "" {
		out["unit"] = c.Scopes.Unit
	}
	if c.Scopes.Add {
		out["add"], out["save"] = true, api.ScopeAddURI
	}
	return out
}
