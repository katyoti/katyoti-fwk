// SPDX-License-Identifier: Apache-2.0

// Package core implements the features service over the CWM Object API: it
// reads features through CWMService and stores nothing.
package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// Service implements api.FeaturesService.
type Service struct{}

// New returns the features service.
func New() *Service { return &Service{} }

var _ api.FeaturesService = (*Service)(nil)

// Board is every feature under an object — a module's whole subtree, not
// only its direct children; a project's every module — as the page the
// Features tab shows: the features by state as a bar and as chips filtering
// the list, then each feature, highest cost of delay first, with its state,
// the object it sits under, its cost of delay and the goals it serves (each
// opening it), a click opening it; Add creates one under the object. The
// index answers the rows; each feature is read whole for its cost of delay
// and its edges.
func (s *Service) Board(ctx context.Context, in api.BoardInput) (render.Def, error) {
	if in.ID == "" {
		return render.Def{}, fmt.Errorf("%w: features.Board: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return render.Def{}, err
	}
	return board(ctx, cwm, in)
}

func board(ctx context.Context, cwm cwmapi.CWMService, in api.BoardInput) (render.Def, error) {
	// Holding keeps to the project holding the object (a qualified id, or an
	// id one project alone holds, in a universe serving several); the index
	// keeps the features only - the whole subtree read over the bus is
	// megabytes.
	rows, err := paneapi.HoldingSearch(ctx, cwm, in.ID, cwmapi.ObjectSearchInput{Types: []string{api.FeatureType}})
	if err != nil {
		return render.Def{}, err
	}
	whole := cwmapi.NewWholeReader(ctx, cwm, api.FeatureType)
	cod := map[string]int{}
	serves := map[string][]string{}
	var goals []string
	seen := map[string]bool{}
	for _, r := range rows {
		// A bare homonym is ambiguous with several projects: the feature is
		// read by its id qualified with the project holding its row.
		ref := r.Object
		if r.Project != "" {
			ref.ID = cwmapi.QualifiedID(r.Project, r.ID)
		}
		o, err := whole.One(ref)
		if err != nil {
			o = r.Object // a feature whose file cannot be read still shows, from its index row
		}
		cod[r.ID] = api.CoD(o).Sum()
		for _, e := range o.Edges {
			if string(e.Kind) == api.EdgeServes {
				// A bare goal id is of the feature's own project: qualified
				// with it when several are served, so the goal named and
				// opened is never another project's homonym.
				g := e.To
				if r.Project != "" && !strings.Contains(g, ":") {
					g = cwmapi.QualifiedID(r.Project, g)
				}
				serves[r.ID] = append(serves[r.ID], g)
				if !seen[g] {
					seen[g] = true
					goals = append(goals, g)
				}
			}
		}
	}
	names, err := goalNames(ctx, cwm, goals)
	if err != nil {
		return render.Def{}, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if cod[rows[i].ID] != cod[rows[j].ID] {
			return cod[rows[i].ID] > cod[rows[j].ID]
		}
		return rows[i].Name < rows[j].Name
	})
	return render.Def{Kind: "page", DSLTree: boardTree(in, rows, cod, serves, names)}, nil
}

// goalType is the object type a served goal is stored as.
const goalType = "goal"

// goalNames answers the name of each goal id - bare, or qualified when
// several projects are served - read from its module's index, keyed by the id
// given: a qualified id takes its own project's row only.
func goalNames(ctx context.Context, cwm cwmapi.CWMService, ids []string) (map[string]string, error) {
	names := map[string]string{}
	if len(ids) == 0 {
		return names, nil
	}
	var mods []string
	seen := map[string]bool{}
	for _, id := range ids {
		_, bare, err := cwmapi.SplitReference(id)
		if err != nil {
			continue
		}
		if m, err := cwmapi.ModuleOfObjectID(bare); err == nil && !seen[m] {
			seen[m] = true
			mods = append(mods, m)
		}
	}
	if len(mods) == 0 {
		return names, nil
	}
	// The goals only: their modules' every object would cross the bus.
	rows, err := paneapi.ReadRows(ctx, cwm, cwmapi.ObjectSearchInput{Modules: mods, Types: []string{goalType}})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		key := r.ID
		if r.Project != "" {
			key = cwmapi.QualifiedID(r.Project, r.ID)
		}
		if _, ok := names[key]; !ok {
			names[key] = r.Name
		}
	}
	return names, nil
}

func boardTree(in api.BoardInput, rows []paneapi.ObjectRow, cod map[string]int, serves map[string][]string, names map[string]string) render.Node {
	lang := in.Language
	q := map[string]any{"state": in.State}
	shown := rows
	if in.State != "" {
		shown = nil
		for _, r := range rows {
			if r.State == in.State {
				shown = append(shown, r)
			}
		}
	}
	orphans := 0
	for _, r := range rows {
		if len(serves[r.ID]) == 0 {
			orphans++
		}
	}
	var blocks []render.Node
	if bar := paneapi.StateBar(tr(lang, "byState"), rows); bar != nil {
		blocks = append(blocks, bar)
	}
	if chips := paneapi.StateChips(tr(lang, "states"), tr(lang, "all"), rows, in.State, q); chips != nil {
		blocks = append(blocks, chips)
	}
	lines := make([]render.Node, 0, len(shown))
	for _, r := range shown {
		codBadge := render.Badge(fmt.Sprintf("CoD %d", cod[r.ID]), "violet").With("size", "xs").With("keepCase", true).With("tooltip", tr(lang, "cod"))
		line := []render.Node{paneapi.RowNode(r, codBadge)}
		if gs := serves[r.ID]; len(gs) > 0 {
			chips := []render.Node{render.Text(tr(lang, "serves")).With("size", "xs").With("color", "dimmed")}
			for _, g := range gs {
				chips = append(chips, render.Badge(firstOf(names[g], g), "teal").With("size", "xs").With("icon", "target").
					With("keepCase", true).With("action", render.NavigateTo(tr(lang, "openGoal"), g, "")))
			}
			line = append(line, render.HBox(chips...).With("align", "center"))
		} else {
			line = append(line, render.Badge(tr(lang, "servesNothing"), "orange").With("size", "xs").With("icon", "alert-triangle").With("keepCase", true))
		}
		lines = append(lines, render.Card(line...).With("gap", "xs").With("p", "xs"))
	}
	aside := []render.Node{}
	if orphans > 0 {
		aside = append(aside, render.Badge(fmt.Sprintf(tr(lang, "orphans"), orphans), "orange").With("size", "xs").With("keepCase", true))
	}
	aside = append(aside, render.Button(tr(lang, "add"), render.CreateUnder(tr(lang, "addFeature"), api.FeatureType, "")).With("icon", "plus"))
	sec := render.Section(tr(lang, "features"), "package").With("size", "md").With("color", "violet").With("badge", len(shown)).Aside(aside...)
	if len(shown) == 0 {
		empty := tr(lang, "noFeatures")
		if len(rows) > 0 {
			empty = tr(lang, "noneInState")
		}
		sec = sec.Empty(empty, "")
	} else {
		sec["children"] = []render.Node{render.VBox(lines...).With("gap", "xs")}
	}
	return render.VBox(append(blocks, sec)...).With("gap", "md")
}

func firstOf(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
