// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// DataTypeSummary is the GUI-facing record for one data-type
// Contribution discovered via the hub catalog. Carries enough to
// render a tree node + the side-pane detail: provider + id (the
// path-shaped suffix inside the URI, e.g. `task/hello-world/input`)
// + the raw DSL source bytes the sidecar embedded.
type DataTypeSummary struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	URI      string `json:"uri"`
	Format   string `json:"format,omitempty"` // "yaml" or "json"; sidecar-supplied
	Source   string `json:"source,omitempty"` // raw DSL bytes as a string (UTF-8)
}

// ListDataTypesResult is the Wails return for the Type Browser's
// initial fetch. Error is inline so the UI can show a banner.
type ListDataTypesResult struct {
	DataTypes []DataTypeSummary `json:"dataTypes"`
	Error     string            `json:"error,omitempty"`
}

// ListDataTypes returns every `data-type` Contribution registered in
// the universe's catalog — across every sidecar — with the raw DSL
// source attached so the Type Browser can render a YAML preview
// without a second round-trip per type.
//
// One query at universe reach. The aggregator answers from its index — no
// contributor is contacted — so this is cheap even with many providers.
//
// Wails-bound: window.go.main.App.ListDataTypes(universeName).
func (a *App) ListDataTypes(universeName string) ListDataTypesResult {
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return ListDataTypesResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}

	// A data-type's content IS its DSL, carried in the index because every
	// consumer of a type needs it — so this listing renders the schema
	// without a second call, and decodes nothing.
	entries, err := mbapi.Query(a.ctx,
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: contribapi.ResourceTypeDataType, Catalog: cat})
	if err != nil {
		return ListDataTypesResult{Error: fmt.Sprintf("list data-types: %v", err)}
	}

	out := make([]DataTypeSummary, 0, len(entries))
	for _, e := range entries {
		ref, _ := e.SourceRef()
		out = append(out, DataTypeSummary{
			Provider: e.Provider,
			ID:       e.ID,
			URI:      e.ResolvedURI(),
			// Format is READ from the media type now, not restated in the
			// content. One statement of what the bytes are.
			Format: formatOf(ref.MediaType),
			Source: string(e.Source()),
		})
	}

	// Stable order — by provider, then id — so the tree doesn't
	// shuffle between fetches (catalog enumeration order is map-based
	// and non-deterministic).
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ID < out[j].ID
	})
	return ListDataTypesResult{DataTypes: out}
}

// formatOf names a DSL's serialisation from its media type.
//
// The media type is the one statement of what the bytes are; this is the
// frontend's shorter spelling of it, not a second source of truth.
func formatOf(mediaType string) string {
	if strings.Contains(mediaType, "json") {
		return "json"
	}
	return "yaml"
}
