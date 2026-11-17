// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"net/url"
	"strings"
)

// URI is the relative addressing form for every CRUD operation
// (cwm-v3 §2.4 / §5). Connection-relative — the hosting
// CRUDService instance owns the provider scheme, so URIs travelling
// through method inputs carry NO `<provider>:/` prefix. When a URI
// flows OUT of the connection (catalog Contribution body,
// cross-process schema reference), the connection prefixes its
// provider; see ResourceURI grammar in the URI grammar.
//
// Grammar (relative form):
//
//	URI ::= <path>[?<query>]
//	<path> ::= <segment>(/<segment>)*
//	<segment> ::= [a-zA-Z0-9_-]+
//	<query> ::= <pair>(&<pair>)*
//	<pair> ::= <key>=<value>
//
// Method-based disambiguation: List/Watch take collection URIs;
// Load/Save/Delete take record URIs. The CRUDService backend trusts
// the method semantics — no trailing-slash convention, no
// discriminator field.
//
// Examples by shape:
//
//	book                          — top-level collection
//	book/123                      — top-level record
//	node/X/item                   — scoped collection (items on node X)
//	node/X/item/Y                 — scoped record
//	node/X/item?type=todo         — discriminated collection (faceted)
//	tenant/T/team/U/project/P     — n-level path nesting
type URI string

// ParsedURI is the structured view of a relative URI after the
// connection backend has split it into path segments + query
// params. Backends typically classify URIs by Segments[0] (the
// top-level type) and route accordingly; cross-cutting helpers
// like the in-memory store's bucket lookup index by Segments[0].
type ParsedURI struct {
	// Path is the raw path portion (everything before `?`). Useful
	// for echoing back in error messages without re-encoding.
	Path string
	// Segments is the slash-split path. Always at least 1 element
	// for a well-formed URI; empty path yields an error from
	// ParseURI.
	Segments []string
	// Query is the parsed query params. Empty when the URI has no
	// `?<query>` suffix. Per the URI grammar the
	// query syntax is the standard URL form; multi-valued keys
	// degrade to the first value.
	Query url.Values
}

// ParseURI splits a relative URI into its structural parts.
// Returns ErrInvalidInput-wrapped errors for empty / malformed
// inputs. Does NOT accept absolute URIs with a scheme prefix
// (`<provider>:/...`) — backends operate on the relative form;
// callers strip the scheme prefix at the connection boundary.
func ParseURI(uri URI) (ParsedURI, error) {
	s := strings.TrimSpace(string(uri))
	if s == "" {
		return ParsedURI{}, fmt.Errorf("URI empty: %w", ErrInvalidInput)
	}
	if strings.Contains(s, ":/") {
		// Rejecting scheme-bearing URIs at the backend boundary keeps
		// the grammar honest. Producers that hold a `<provider>:/...`
		// URI must strip the scheme before calling.
		return ParsedURI{}, fmt.Errorf("URI %q carries a scheme prefix; CRUDService methods take relative form: %w", s, ErrInvalidInput)
	}
	if strings.HasPrefix(s, "/") {
		return ParsedURI{}, fmt.Errorf("URI %q starts with `/`; relative form has no leading slash: %w", s, ErrInvalidInput)
	}
	pathPart := s
	queryPart := ""
	if idx := strings.IndexByte(s, '?'); idx >= 0 {
		pathPart = s[:idx]
		queryPart = s[idx+1:]
	}
	pathPart = strings.TrimSuffix(pathPart, "/")
	if pathPart == "" {
		return ParsedURI{}, fmt.Errorf("URI %q has empty path: %w", s, ErrInvalidInput)
	}
	segs := strings.Split(pathPart, "/")
	for _, seg := range segs {
		if seg == "" {
			return ParsedURI{}, fmt.Errorf("URI %q has empty segment: %w", s, ErrInvalidInput)
		}
	}
	values, err := url.ParseQuery(queryPart)
	if err != nil {
		return ParsedURI{}, fmt.Errorf("URI %q query parse: %w: %w", s, err, ErrInvalidInput)
	}
	return ParsedURI{Path: pathPart, Segments: segs, Query: values}, nil
}

// Type returns the first segment of the URI — the convention is
// that this names the record type the URI addresses. Returns ""
// for an empty ParsedURI (defensive — ParseURI rejects empties
// before they reach this method).
func (p ParsedURI) Type() string {
	if len(p.Segments) == 0 {
		return ""
	}
	return p.Segments[0]
}

// LastSegment returns the trailing path segment — typically the
// record ID for record URIs (`book/123` → "123") or the leaf
// collection name for nested collections (`node/X/item` → "item").
// Use IsRecord / IsCollection contexts (or the method semantics)
// to disambiguate.
func (p ParsedURI) LastSegment() string {
	if n := len(p.Segments); n > 0 {
		return p.Segments[n-1]
	}
	return ""
}

// ScopedID interprets a URI as a flat-form record reference:
//
//	<type>          → type=<type>, id=""  (collection / new-record-with-server-id)
//	<type>/<id>     → type=<type>, id=<id>  (specific record)
//
// Backends serving simple top-level types use this helper; n-level-
// scoping backends parse Segments directly. Returns ErrInvalidInput
// wrap when Segments are nested deeper than 2 — those URIs are
// outside the flat scope.
func (p ParsedURI) ScopedID() (typeName, id string, err error) {
	switch len(p.Segments) {
	case 1:
		return p.Segments[0], "", nil
	case 2:
		return p.Segments[0], p.Segments[1], nil
	default:
		return "", "", fmt.Errorf("URI %q has %d segments; ScopedID supports only flat <type>[/<id>]: %w",
			p.Path, len(p.Segments), ErrInvalidInput)
	}
}
