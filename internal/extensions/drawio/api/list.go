// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"sort"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ListViewpoints returns every viewpoint the catalog holds — the types of
// drawing a diagram can name — sorted by URI.
func ListViewpoints(ctx context.Context) ([]mbapi.Attributed[ViewpointBody], error) {
	return list[ViewpointBody](ctx, ContributionTypeViewpoint)
}

// ListNotations returns every notation the catalog holds, sorted by URI.
func ListNotations(ctx context.Context) ([]mbapi.Attributed[NotationBody], error) {
	return list[NotationBody](ctx, ContributionTypeNotation)
}

func list[T any](ctx context.Context, kind string) ([]mbapi.Attributed[T], error) {
	out, err := mbapi.QueryTyped[T](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: kind})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out, nil
}
