// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
)

// kinds_listing.go — the list of lists: every declared kind with its
// description and how many entries it has, and one kind's entries.
//
// Read from the two things a view already holds — its kind declarations
// (KindsAt, the one registry) and its index (Query) — and from nothing else:
// a second list of kinds would drift from the declarations the first time a
// pack added one. A kind with no entry is listed at 0, never left out: that a
// kind exists and is empty is something a person walking the list must see.

// KindListing is one declared kind: its name, its description and the number
// of entries a Query of that kind returns in the same view.
type KindListing struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Count       int    `json:"count"`
}

// KindEntry is one entry of a kind as the index lists it: its URI, and the
// name and description derived into its meta — no layer opened.
type KindEntry struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// ListKinds returns every kind declared in the view `q` names, sorted by
// name, each with its description and its entry count. q.Kind is ignored:
// every kind is listed, each counted by its own Query under q's reach and
// filter, so a count is exactly what listing that kind returns.
func ListKinds(ctx context.Context, q Q) ([]KindListing, error) {
	kinds, err := KindsAt(ctx, q)
	if err != nil {
		return nil, err
	}
	names := kinds.Names()
	out := make([]KindListing, 0, len(names))
	for _, name := range names {
		qk := q
		qk.Kind = name
		entries, err := Query(ctx, qk)
		if err != nil {
			return nil, fmt.Errorf("contributions: count kind %s: %w", name, err)
		}
		decl, _ := kinds.Get(name)
		out = append(out, KindListing{Name: name, Description: decl.Description, Count: len(entries)})
	}
	return out, nil
}

// ErrKindNotDeclared is returned by ListKindEntries for a kind the view does
// not declare.
var ErrKindNotDeclared = errors.New("kind not declared in this view")

// ListKindEntries returns the entries of one declared kind, from the index
// alone: each URI with the name and description in its meta.
func ListKindEntries(ctx context.Context, q Q, kind string) ([]KindEntry, error) {
	kinds, err := KindsAt(ctx, q)
	if err != nil {
		return nil, err
	}
	if _, ok := kinds.Get(kind); !ok {
		return nil, fmt.Errorf("contributions: %s: %w", kind, ErrKindNotDeclared)
	}
	q.Kind = kind
	entries, err := Query(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]KindEntry, 0, len(entries))
	for _, c := range entries {
		m := c.ResolvedMetadata()
		out = append(out, KindEntry{URI: c.ResolvedURI(), Name: m.Name, Description: m.Description})
	}
	return out, nil
}
