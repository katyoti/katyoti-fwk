// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"sort"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// WorkspaceLinkKind is one edge kind the "Add link" picker offers.
type WorkspaceLinkKind struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// WorkspaceObjectLinkKinds lists the edge kinds declared in the universe — the
// `edge-type` contributions, read from the catalog like any other kind. The
// substrate still decides whether a link of that kind may leave this object
// (EdgeCreate refuses with its reason); the picker only names what exists.
//
// Wails-bound: window.go.main.App.WorkspaceObjectLinkKinds(universeName).
func (a *App) WorkspaceObjectLinkKinds(universeName string) []WorkspaceLinkKind {
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectLinkKinds %q: %v", universeName, err))
		return nil
	}
	out, err := linkKindsVia(a.ctx, cat)
	if err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectLinkKinds %q: %v", universeName, err))
		return nil
	}
	return out
}

// linkKindsVia lists the `edge-type` contributions a catalog holds, one row
// per name, sorted.
func linkKindsVia(ctx context.Context, cat mbapi.CatalogReader) ([]WorkspaceLinkKind, error) {
	entries, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat, Kind: "edge-type"})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]WorkspaceLinkKind, 0, len(entries))
	for _, e := range entries {
		meta := e.ResolvedMetadata()
		name := meta.Name
		if name == "" {
			name = e.ID
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, WorkspaceLinkKind{Name: name, Description: meta.Description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
