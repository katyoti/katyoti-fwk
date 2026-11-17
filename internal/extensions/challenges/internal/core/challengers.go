// SPDX-License-Identifier: Apache-2.0

// Package core implements the challenges service over the CWM Object API: it
// reads the challenges edges through CWMService and stores nothing.
package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/challenges/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// Service implements api.ChallengesService.
type Service struct{}

// New returns the challenges service.
func New() *Service { return &Service{} }

var _ api.ChallengesService = (*Service)(nil)

// Challengers is what challenges an object — every object with a
// `challenges` edge to it — as the page its Challenged by tab shows: each
// challenger with its type, its state (an open doubt and an answered one
// tell apart) and where it sits, a click opening it. Read from CWM's index.
func (s *Service) Challengers(ctx context.Context, in api.ObjectInput) (render.Def, error) {
	if in.ID == "" {
		return render.Def{}, fmt.Errorf("%w: challenges.Challengers: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return render.Def{}, err
	}
	return challengers(ctx, cwm, in)
}

// challengers reads the page from cwm. The challengers are the edges' ends in
// the challenged object's project (an edge never crosses one): with several
// projects served, a bare end would answer every project's homonym.
func challengers(ctx context.Context, cwm cwmapi.CWMService, in api.ObjectInput) (render.Def, error) {
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: in.ID, Kind: cwmapi.EdgeType(api.EdgeChallenges), Direction: cwmapi.DirectionIn})
	if err != nil {
		return render.Def{}, err
	}
	var ids []string
	seen := map[string]bool{}
	for _, e := range edges.Edges {
		if e.To == in.ID && !seen[e.From] {
			seen[e.From] = true
			ids = append(ids, e.From)
		}
	}
	if ids, err = paneapi.EdgeEnds(ctx, cwm, in.ID, ids); err != nil {
		return render.Def{}, err
	}
	rows, err := paneapi.ReadRowsOf(ctx, cwm, ids)
	if err != nil {
		return render.Def{}, err
	}
	return render.Def{Kind: "page", DSLTree: challengersTree(in.Language, rows, paneapi.Unresolved(ids, rows))}, nil
}

func challengersTree(lang string, rows []paneapi.ObjectRow, unresolved int) render.Node {
	lines := make([]render.Node, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, paneapi.RowNode(r, render.Badge(r.TypeLabel, "gray").With("size", "xs").With("variant", "outline").With("keepCase", true)))
	}
	sec := render.Section(tr(lang, "challengedBy"), "swords").With("size", "md").With("color", "red").With("badge", len(rows)).
		With("description", tr(lang, "why"))
	if unresolved > 0 {
		sec = sec.Aside(render.Badge(fmt.Sprintf(tr(lang, "unresolved"), unresolved), "orange").With("size", "xs").With("keepCase", true))
	}
	if len(lines) == 0 {
		return render.VBox(sec.Empty(tr(lang, "none"), "green"))
	}
	sec["children"] = []render.Node{render.VBox(lines...).With("gap", "xs")}
	return render.VBox(sec)
}
