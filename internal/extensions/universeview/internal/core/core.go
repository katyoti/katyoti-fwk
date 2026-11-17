// SPDX-License-Identifier: Apache-2.0

// Package core resolves a universe's view: the union of the allow-lists its
// tags declare, each item at the lowest level a tag or the universe gives it,
// less what the universe hides, at the person's display level.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/universeview/api"
)

// Service implements api.UniverseViewService.
type Service struct {
	// home is the universe's config root its Univers.yaml is read from; the
	// test seam (production leaves it empty: the universe this process serves).
	home string
}

// New returns the service for the universe this process serves.
func New() *Service { return &Service{} }

// NewAt returns the service reading the Univers.yaml under home.
func NewAt(home string) *Service { return &Service{home: home} }

// ResolveView reads the universe's Univers.yaml and the tags its members
// declare, and resolves the view at the level asked.
func (s *Service) ResolveView(ctx context.Context, in api.ResolveViewInput) (api.View, error) {
	level, err := levelOf(in.Level)
	if err != nil {
		return api.View{}, err
	}
	home := s.home
	if home == "" {
		home = application.Env().UniverseDir()
	}
	m, err := cenv.LoadUniverseManifest(home)
	if err != nil {
		return api.View{}, fmt.Errorf("universe-view: read Univers.yaml: %w", err)
	}
	var declared map[string]api.UniverseTag
	if len(m.Tags) > 0 {
		declared = tagsOf(ctx)
	}
	return Resolve(declared, m, level), nil
}

// tagsOf reads every universe-tag the universe's members declare, by id.
func tagsOf(ctx context.Context) map[string]api.UniverseTag {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return nil
	}
	got, err := mbapi.QueryTyped[json.RawMessage](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: api.TagKind, Catalog: cat})
	if err != nil {
		slog.Warn("universe-view: universe-tag declarations not read", "error", err)
		return nil
	}
	out := make(map[string]api.UniverseTag, len(got))
	for _, g := range got {
		u, err := contribapi.ParseResourceURI(g.URI)
		if err != nil || u.ID == "" {
			continue
		}
		var t api.UniverseTag
		if len(g.Body) > 0 {
			if err := json.Unmarshal(g.Body, &t); err != nil {
				slog.Warn("universe-view: universe-tag skipped", "uri", g.URI, "error", err)
				continue
			}
		}
		out[u.ID] = t // two members declaring one tag: the catalog's order decides, as for any entry
	}
	return out
}

// Resolve is the view at level: with no tag, everything (All); otherwise
// the union of the declared tags' elements, in tag
// order, each at the lowest level any of them gives it, adjusted by gui.levels (by element
// id), kept when its level is at most the one asked. First is gui.first, else
// the tags' own. A tag no member declares is named in Unknown and adds nothing.
// gui.hide is retired: it is not applied and the ids it names are listed in
// RetiredHide.
func Resolve(declared map[string]api.UniverseTag, m *cenv.UniverseManifest, level string) api.View {
	v := api.View{Level: level, Allowed: []string{}, First: []string{}}
	var gui cenv.UniverseGUI
	if m != nil && m.GUI != nil {
		gui = *m.GUI
	}
	v.RetiredHide = append(v.RetiredHide, gui.Hide...)
	if m == nil || len(m.Tags) == 0 {
		v.All = true
		v.First = append(v.First, gui.First...)
		return v
	}
	v.Tags = append([]string(nil), m.Tags...)
	rank := rankOf(level)
	var order, first []string
	lowest := map[string]int{}
	for _, name := range m.Tags {
		t, ok := declared[name]
		if !ok {
			v.Unknown = append(v.Unknown, name)
			continue
		}
		for _, it := range t.Elements {
			r := rankOf(it.Level)
			if prev, seen := lowest[it.ID]; !seen {
				order = append(order, it.ID)
				lowest[it.ID] = r
			} else if r < prev {
				lowest[it.ID] = r
			}
		}
		first = append(first, t.First...)
	}
	for _, id := range order {
		r := lowest[id]
		if adj, ok := gui.Levels[id]; ok {
			r = rankOf(adj)
		}
		if r > rank {
			continue
		}
		v.Allowed = append(v.Allowed, id)
	}
	if len(gui.First) > 0 {
		v.First = append(v.First, gui.First...)
	} else {
		v.First = append(v.First, dedupe(first)...)
	}
	return v
}

// rankOf is a level's place, lowest first; an empty or unknown level is
// essentiel, the safe default for an item (seen by everyone).
func rankOf(level string) int {
	for i, l := range api.Levels {
		if l == level {
			return i
		}
	}
	return 0
}

// levelOf checks an asked level; empty is essentiel, a new person's level.
func levelOf(level string) (string, error) {
	if level == "" {
		return api.LevelEssentiel, nil
	}
	for _, l := range api.Levels {
		if l == level {
			return l, nil
		}
	}
	return "", fmt.Errorf("universe-view: level %q is none of %v", level, api.Levels)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
