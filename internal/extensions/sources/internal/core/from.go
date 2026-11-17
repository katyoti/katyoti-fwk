// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// picks says whether a connector is a picker: it searches
// by free text, browses, or picks from what another source kept.
func picks(c api.ConnectorDecl) bool {
	return c.Search != "" || c.Browse != "" || c.From != nil
}

// fromItems is what a connector declaring from receives for a carrier
// : the attribute's values on the items the carrier's own
// source of the other connector brought - read from their data, each value
// once, in the order its anchors are listed, with the title of the item that
// carries it (its anchor's name). None for a connector without from
// or with no carrier.
func fromItems(ctx context.Context, cwm cwmapi.CWMService, carrierID string, c api.ConnectorDecl) (*api.FromItems, error) {
	return keptItems(ctx, cwm, carrierID, c.From)
}

// keptItems is what the carrier's own source of another connector kept, as
// a from declaration reads it - from, or propose_from.
// None for no declaration or no carrier.
func keptItems(ctx context.Context, cwm cwmapi.CWMService, carrierID string, from *api.ConnectorFrom) (*api.FromItems, error) {
	if from == nil || carrierID == "" {
		return nil, nil
	}
	out := &api.FromItems{Connector: from.First(), Attr: from.Attr, Values: []string{}}
	srcs, err := sourcesOf(ctx, cwm, carrierID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	matched := false
	for _, src := range srcs {
		// Every source of a connector it follows: a LeanIX
		// and a PeaqView application source of one carrier both feed it.
		if src.Via != "child" || !from.Follows(src.Connector) {
			continue
		}
		if !matched {
			matched, out.Connector = true, src.Connector
		}
		module, err := cwmapi.ModuleOfObjectID(src.ID)
		if err != nil {
			return nil, err
		}
		root := projectRoot(ctx, cwm, module)
		docs, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Parent: src.ID, Type: api.SourceDocumentType})
		if err != nil {
			return nil, fmt.Errorf("sources: what %s brought: %w", src.ID, err)
		}
		// A list answers resident metadata: each anchor is read whole for
		// its data file.
		for _, d := range docs.Objects {
			got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: d.ID})
			if err != nil {
				continue
			}
			var f struct {
				DataFile string `json:"data_file"`
			}
			_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
			if f.DataFile == "" {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.DataFile)))
			if err != nil {
				continue // a file gone: the next sync says so
			}
			var data map[string]any
			if json.Unmarshal(raw, &data) != nil {
				continue
			}
			v := attrValue(data[from.Attr])
			if v != "" && !seen[v] {
				seen[v] = true
				out.Values = append(out.Values, v)
				out.SetTitle(v, got.Object.Name)
				out.SetConnector(v, src.Connector)
			}
		}
	}
	return out, nil
}

// followedConnector is the connector of the carrier's first own source a
// from declaration follows - the other source a sentence or a badge names -
// else the first connector it follows.
func followedConnector(ctx context.Context, cwm cwmapi.CWMService, carrierID string, from *api.ConnectorFrom) string {
	if carrierID != "" {
		if srcs, err := sourcesOf(ctx, cwm, carrierID); err == nil {
			for _, src := range srcs {
				if src.Via == "child" && from.Follows(src.Connector) {
					return src.Connector
				}
			}
		}
	}
	return from.First()
}

// andList joins names in French: « A », « A et B », « A, B et C ».
func andList(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " et " + names[len(names)-1]
}

// followedConnectors is the connector of each of the carrier's own sources
// a declaration follows, each once, in their order - else its first
// connector.
func followedConnectors(ctx context.Context, cwm cwmapi.CWMService, carrierID string, from *api.ConnectorFrom) []string {
	var out []string
	if carrierID != "" {
		if srcs, err := sourcesOf(ctx, cwm, carrierID); err == nil {
			seen := map[string]bool{}
			for _, src := range srcs {
				if src.Via == "child" && from.Follows(src.Connector) && !seen[src.Connector] {
					seen[src.Connector] = true
					out = append(out, src.Connector)
				}
			}
		}
	}
	if len(out) == 0 {
		if n := from.First(); n != "" {
			out = []string{n}
		}
	}
	return out
}

// attrValue is an attribute's value as text: a string, a number or a
// boolean; nothing for anything else.
func attrValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64, bool:
		return fmt.Sprint(x)
	}
	return ""
}

// carrierOfSource is the carrier an information source is under.
func carrierOfSource(ctx context.Context, cwm cwmapi.CWMService, sourceID string) string {
	module, err := cwmapi.ModuleOfObjectID(sourceID)
	if err != nil {
		return ""
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: sourceID})
	if err != nil {
		return ""
	}
	return got.Object.ParentID()
}
