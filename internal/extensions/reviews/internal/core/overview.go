// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// ratingRank orders ratings, worst first; none last.
func ratingRank(r string) int {
	switch r {
	case api.RatingCritical:
		return 0
	case api.RatingNeedsAttention:
		return 1
	case api.RatingGood:
		return 2
	}
	return 3
}

// Cell is one module's reviews of one dimension: the worst current rating,
// how it moved since the previous reviews of the same types, the age of the
// oldest current review, and the reviews.
type Cell struct {
	Rating  string
	Trend   int // -1 worse, 0 same or unknown, +1 better
	AgeDays int
	Reviews []string
}

// ModuleRow is one module of the overview.
type ModuleRow struct {
	Module cwmapi.Object
	Cells  map[string]*Cell
	Open   map[string]int // open findings by severity
}

// OverviewOf reads the overview: for each reviewable module (every one, or
// in.ID and the modules under it), its current review of each type grouped by
// the type's dimension, and its open findings by severity.
func (s *Service) OverviewOf(ctx context.Context, id string, now time.Time) ([]ModuleRow, []string, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return nil, nil, err
	}
	// The reviewable capability is attached to every module: an attachment the
	// listing's capability filter does not read, so modules are listed by
	// what makes them one - the module level capability their types carry,
	// never a type's name.
	all, err := c.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapModule})
	if err != nil {
		return nil, nil, err
	}
	keep := func(o cwmapi.Object) bool { return true }
	if id != "" {
		parent := map[string]string{}
		for _, o := range all.Objects {
			parent[o.ID] = o.Parent
		}
		keep = func(o cwmapi.Object) bool {
			for x := o.ID; x != ""; x = parent[x] {
				if x == id {
					return true
				}
			}
			return false
		}
	}
	dimOf := map[string]string{}
	if types, err := ReviewTypes(ctx); err == nil {
		for _, t := range types {
			dimOf[t.Name] = t.Dimension
		}
	}
	dims := map[string]bool{}
	var rows []ModuleRow
	for _, m := range all.Objects {
		if !keep(m) {
			continue
		}
		reviews, err := children(ctx, c, m.ID, api.ReviewType)
		if err != nil {
			return nil, nil, err
		}
		row := ModuleRow{Module: m, Cells: map[string]*Cell{}, Open: map[string]int{}}
		byType := map[string][]ReviewOf{}
		for _, o := range reviews {
			r, err := ReadReview(ctx, c, o)
			if err != nil {
				return nil, nil, err
			}
			byType[r.Fields.Type] = append(byType[r.Fields.Type], r)
		}
		types := make([]string, 0, len(byType))
		for typ := range byType {
			types = append(types, typ)
		}
		sort.Strings(types)
		for _, typ := range types {
			rs := byType[typ]
			sort.SliceStable(rs, func(i, j int) bool { return rs[i].Fields.ReviewedAt > rs[j].Fields.ReviewedAt })
			dim := dimOf[typ]
			if dim == "" {
				dim = "other"
			}
			dims[dim] = true
			cell := row.Cells[dim]
			if cell == nil {
				cell = &Cell{Rating: ""}
				row.Cells[dim] = cell
			}
			cur := rs[0]
			// The cell shows its worst current rating and opens the review
			// that gave it: that review leads its list.
			if cell.Rating == "" || ratingRank(cur.Fields.Rating) < ratingRank(cell.Rating) {
				cell.Rating = cur.Fields.Rating
				cell.Reviews = append([]string{cur.Object.ID}, cell.Reviews...)
			} else {
				cell.Reviews = append(cell.Reviews, cur.Object.ID)
			}
			if len(rs) > 1 {
				a, b := ratingRank(cur.Fields.Rating), ratingRank(rs[1].Fields.Rating)
				switch {
				case a < b:
					cell.Trend = -1
				case a > b && cell.Trend == 0:
					cell.Trend = 1
				}
			}
			if at, err := time.Parse(time.RFC3339, cur.Fields.ReviewedAt); err == nil {
				if d := int(now.Sub(at).Hours() / 24); d > cell.AgeDays {
					cell.AgeDays = d
				}
			}
			for _, f := range cur.Findings {
				if f.Open() {
					row.Open[f.Fields.Severity]++
				}
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Module.ID < rows[j].Module.ID })
	var dimList []string
	for d := range dims {
		dimList = append(dimList, d)
	}
	sort.Strings(dimList)
	return rows, dimList, nil
}

// Overview is the Reviews overview panel: the KPIs (modules reviewed, cells
// critical, open critical and high findings, the stalest review), then one
// row per module — its name, a cell per dimension (the worst current rating
// as a colour, its trend, its age; a click opens the review) and its open
// findings by severity.
func (s *Service) Overview(ctx context.Context, in api.ObjectInput) (render.Def, error) {
	rows, dims, err := s.OverviewOf(ctx, in.ID, time.Now().UTC())
	if err != nil {
		return render.Def{}, err
	}
	return OverviewPage(in.Language, rows, dims), nil
}

// OverviewPage draws the overview.
func OverviewPage(lang string, rows []ModuleRow, dims []string) render.Def {
	reviewed, critical, stalest := 0, 0, 0
	open := map[string]int{}
	for _, r := range rows {
		if len(r.Cells) > 0 {
			reviewed++
		}
		for _, c := range r.Cells {
			if c.Rating == api.RatingCritical {
				critical++
			}
			if c.AgeDays > stalest {
				stalest = c.AgeDays
			}
		}
		for k, v := range r.Open {
			open[k] += v
		}
	}
	kpis := render.HBox(
		render.KPI(tr(lang, "modulesReviewed"), fmt.Sprintf("%d / %d", reviewed, len(rows)), "blue", "clipboard-check"),
		render.KPI(tr(lang, "criticalCells"), critical, toneIf(critical > 0, "red"), "alert-triangle"),
		render.KPI(tr(lang, "openCriticalHigh"), open["critical"]+open["high"], toneIf(open["critical"]+open["high"] > 0, "orange"), "alert-circle"),
		render.KPI(tr(lang, "stalest"), fmt.Sprintf(tr(lang, "days"), stalest), "gray", "clock"),
	).With("gap", "md")

	head := []render.Node{render.Text(tr(lang, "module")).With("w", 180)}
	for _, d := range dims {
		head = append(head, render.Text(d).With("w", 150))
	}
	head = append(head, render.Text(tr(lang, "openFindings")))
	lines := []render.Node{render.HBox(head...)}
	for _, r := range rows {
		line := []render.Node{render.Link(title(r.Module), render.NavigateTo(fmt.Sprintf(tr(lang, "open"), r.Module.ID), r.Module.ID, "reviews")).With("w", 180)}
		for _, d := range dims {
			c := r.Cells[d]
			if c == nil {
				line = append(line, render.Badge(tr(lang, "notReviewed"), "gray").With("w", 150))
				continue
			}
			text := ratingText(lang, c.Rating) + " " + trendMark(c.Trend) + " · " + fmt.Sprintf(tr(lang, "days"), c.AgeDays)
			line = append(line, render.Link(text, render.NavigateTo(tr(lang, "openReview"), c.Reviews[0], "")).
				With("color", RatingColor(c.Rating)).With("w", 150))
		}
		line = append(line, render.HBox(openBadges(lang, r.Open)...))
		lines = append(lines, render.HBox(line...))
	}
	grid := render.Section(tr(lang, "overview"), "layout-grid", lines[1:]...).Empty(tr(lang, "noModule"), "")
	if len(rows) > 0 {
		grid = render.Section(tr(lang, "overview"), "layout-grid", lines...)
	}
	return render.Def{Kind: "page", Title: tr(lang, "overview"), DSLTree: render.VBox(kpis, grid)}
}

func toneIf(b bool, tone string) string {
	if b {
		return tone
	}
	return "green"
}

func trendMark(t int) string {
	switch {
	case t < 0:
		return "▼"
	case t > 0:
		return "▲"
	}
	return "="
}

func openBadges(lang string, open map[string]int) []render.Node {
	var out []render.Node
	for _, sev := range api.Severities {
		if n := open[sev]; n > 0 {
			out = append(out, render.Badge(fmt.Sprintf("%d %s", n, sev), SeverityColor(sev)))
		}
	}
	if len(out) == 0 {
		out = append(out, render.Dimmed(strings.TrimSpace(tr(lang, "noOpen"))))
	}
	return out
}
