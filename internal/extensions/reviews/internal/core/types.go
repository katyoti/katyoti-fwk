// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// ReviewTypes reads the review types of the universe: every review-type
// contribution, each decoded on its own — one that does not decode is skipped
// with the reason, never all of them.
func ReviewTypes(ctx context.Context) ([]api.ReviewTypeDef, error) {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return nil, fmt.Errorf("reviews: no catalog: %v", err)
	}
	entries, err := mbapi.QueryTyped[json.RawMessage](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: api.ReviewTypeKind, Catalog: cat})
	if err != nil {
		return nil, fmt.Errorf("reviews: review types not read: %w", err)
	}
	var out []api.ReviewTypeDef
	for _, e := range entries {
		var t api.ReviewTypeDef
		if len(e.Body) > 0 {
			if err := json.Unmarshal(e.Body, &t); err != nil {
				slog.Warn("reviews: review type skipped", "uri", e.URI, "error", err)
				continue
			}
		}
		if t.Name == "" {
			continue
		}
		t.Provider = e.Provider
		out = append(out, t)
	}
	return out, nil
}

// applies says whether the review type selects obj: its types and
// capabilities against the object's type and effective capabilities, then,
// when it has one, its when predicate (and edge kinds) through CWM's own
// search — the one evaluator every filter uses.
func applies(ctx context.Context, c cwmapi.CWMService, t api.ReviewTypeDef, obj cwmapi.Object, caps []string) (bool, error) {
	f := t.Applies
	shape := cwmapi.ObjectFilter{Types: f.Types, Capabilities: f.Capabilities}
	if ok, _ := shape.Matches(cwmapi.FilterSubject{Type: obj.Type, Capabilities: caps}); !ok {
		return false, nil
	}
	if f.When == "" && !f.ReadsEdges() {
		return true, nil
	}
	got, err := c.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Within: obj.ID, Types: []string{obj.Type}, Filter: &f})
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(got.Objects, func(o cwmapi.Object) bool { return o.ID == obj.ID }), nil
}

// TypesFor lists the review types that apply to in.ID (of in.Mode when given),
// grouped by dimension, and the presets they form.
func (s *Service) TypesFor(ctx context.Context, in api.TypesInput) (api.TypesOutput, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return api.TypesOutput{}, err
	}
	obj, err := get(ctx, c, in.ID)
	if err != nil {
		return api.TypesOutput{}, err
	}
	eff, err := c.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: obj.ID})
	if err != nil {
		return api.TypesOutput{}, err
	}
	caps := make([]string, 0, len(eff.Capabilities))
	for _, cp := range eff.Capabilities {
		caps = append(caps, cp.ID)
	}
	all, err := ReviewTypes(ctx)
	if err != nil {
		return api.TypesOutput{}, err
	}
	byDim := map[string][]api.ReviewTypeDef{}
	presets := map[string][]string{}
	for _, t := range all {
		mode := t.Trigger.Mode
		if mode == "" {
			mode = api.ModeManual
		}
		if in.Mode != "" && in.Mode != mode {
			continue
		}
		ok, err := applies(ctx, c, t, obj, caps)
		if err != nil {
			return api.TypesOutput{}, fmt.Errorf("review type %s on %s: %w", t.Name, obj.ID, err)
		}
		if !ok {
			continue
		}
		byDim[t.Dimension] = append(byDim[t.Dimension], t)
		for _, p := range t.Presets {
			presets[p] = append(presets[p], t.Name)
		}
	}
	var out api.TypesOutput
	for dim, ts := range byDim {
		sort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })
		out.Dimensions = append(out.Dimensions, api.DimensionTypes{Dimension: dim, Types: ts})
	}
	sort.Slice(out.Dimensions, func(i, j int) bool { return out.Dimensions[i].Dimension < out.Dimensions[j].Dimension })
	for name, ts := range presets {
		sort.Strings(ts)
		out.Presets = append(out.Presets, api.Preset{Name: name, Types: ts})
	}
	sort.Slice(out.Presets, func(i, j int) bool { return out.Presets[i].Name < out.Presets[j].Name })
	return out, nil
}
