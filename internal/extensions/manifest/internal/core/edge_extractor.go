// SPDX-License-Identifier: Apache-2.0

// Generic, data-type-driven edge extractor.
//
// At catalog ingest (Phase 2 of (modules/contributions/decisions/OD-006-in-what-order-does-the-hub-build-its-catalog-and.cwm.md)), the catalog
// walks each Contribution's data layer against its registered data-type. For
// every field marked as an edge in the data-type (via the DSL's
// `->Contribution[]` link sugar), the extractor lifts URI strings out of
// the Body and emits Contribution.Edges entries with the declared
// relation.
//
// Producers ship body URIs as plain strings — they don't populate
// Edges directly. Polyglot producers don't need strategy code; the
// extractor is consumer-side and generic.
//
// Failure mode: edge URI fails to parse → a warning at reindex, not an ingest
// failure.

package core

import (
	"encoding/json"
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// ExtractEdges walks `body` against `dataType` and emits ContributionEdge
// values for every field whose data-type marks it as a link.
//
// The data-type's Fields map is the source of truth: any field whose
// type has `LinkMode != ""` is an edge. The field name becomes the
// relation; the field's value (a URI string for one2*, a []URI for
// many2*) is the edge target.
//
// Returns the empty slice (not nil) when no edges are found — the
// catalog stores it as-is on the Contribution.
func ExtractEdges(body json.RawMessage, dataType *schemaapi.Type) ([]contribapi.ContributionEdge, error) {
	if dataType == nil || len(body) == 0 {
		return nil, nil
	}
	if dataType.Kind != schemaapi.KindObject {
		return nil, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("edge extractor: invalid JSON body: %w", err)
	}
	out := make([]contribapi.ContributionEdge, 0)
	for fname, ftype := range dataType.Fields {
		if ftype == nil {
			continue
		}
		edges, err := edgesForField(fname, ftype, decoded[fname])
		if err != nil {
			return nil, fmt.Errorf("edge extractor: field %q: %w", fname, err)
		}
		out = append(out, edges...)
	}
	return out, nil
}

// edgesForField inspects one field's data-type entry + the corresponding
// body value, emitting edges when the field is link-shaped.
//
// Cases covered:
//   - Object field with LinkMode != "" (e.g. `installSkill: ->Contribution`)
//     → one edge (or zero, if value is missing)
//   - Array field whose Items inline a link (e.g. `requires: ->Contribution[]`,
//     which the DSL expands into `array{items: {link, target: Contribution}}`)
//     → one edge per array element
func edgesForField(name string, ftype *schemaapi.Type, raw any) ([]contribapi.ContributionEdge, error) {
	if ftype.LinkMode != "" {
		// Direct link (one2one / many2one).
		uri, ok := raw.(string)
		if !ok || uri == "" {
			return nil, nil
		}
		return []contribapi.ContributionEdge{{
			Relation: name,
			Target:   uri,
			Source:   name,
		}}, nil
	}

	// Array of inline-link items.
	if ftype.Kind == schemaapi.KindArray && ftype.Items != nil {
		var elemType *schemaapi.Type
		if ftype.Items.Inline != nil {
			elemType = ftype.Items.Inline
		}
		if elemType == nil || elemType.LinkMode == "" {
			return nil, nil
		}
		arr, ok := raw.([]any)
		if !ok || len(arr) == 0 {
			return nil, nil
		}
		out := make([]contribapi.ContributionEdge, 0, len(arr))
		for i, v := range arr {
			uri, ok := v.(string)
			if !ok || uri == "" {
				continue
			}
			out = append(out, contribapi.ContributionEdge{
				Relation: name,
				Target:   uri,
				Source:   fmt.Sprintf("%s[%d]", name, i),
			})
		}
		return out, nil
	}

	// Nested object — recurse so edges in sub-objects are still surfaced.
	if ftype.Kind == schemaapi.KindObject && len(ftype.Fields) > 0 {
		sub, ok := raw.(map[string]any)
		if !ok {
			return nil, nil
		}
		out := make([]contribapi.ContributionEdge, 0)
		for fname, ftype2 := range ftype.Fields {
			child, err := edgesForField(fname, ftype2, sub[fname])
			if err != nil {
				return nil, err
			}
			// Prefix child's Source with the parent field path.
			for i := range child {
				child[i].Source = name + "." + child[i].Source
			}
			out = append(out, child...)
		}
		return out, nil
	}

	return nil, nil
}

// extractEdgesAcrossAll is a convenience for Catalog.Reindex —
// extracts edges for every Contribution given a data-type-resolver
// callback.
//
// The callback receives a Contribution's `Schema` URI and returns the
// parsed data-type Type (or nil + ErrDataTypeNotFound when absent).
