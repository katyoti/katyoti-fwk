// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"sort"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Layouts reads the deck-layout contributions q reaches — the host's own
// (ReachSelf) or every sidecar's (ReachUniverse), through q's Catalog when the
// caller holds one — sorted by name.
func Layouts(ctx context.Context, q mbapi.Q) ([]LayoutBody, error) {
	q.Kind = ContributionTypeDeckLayout
	entries, err := mbapi.QueryTyped[LayoutBody](ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]LayoutBody, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Body)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// LayoutsInReach reads the layouts universe reaches — the universe's catalog,
// with the override entries it applies — and the host's own (ReachSelf) only
// when the universe cannot be read or serves no layout: the lint's pattern
// (LintObject), in one place for every reader that needs the layouts outside
// a lint (the theme import's sample deck).
func LayoutsInReach(ctx context.Context, universe mbapi.Q) []LayoutBody {
	if layouts, err := Layouts(ctx, universe); err == nil && len(layouts) > 0 {
		return layouts
	}
	layouts, _ := Layouts(ctx, mbapi.Q{Reach: mbapi.ReachSelf})
	return layouts
}
