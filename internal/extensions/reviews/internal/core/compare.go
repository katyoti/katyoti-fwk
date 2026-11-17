// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// Comparison is a review against the previous review of its type under the
// same object: the findings it raised (found in it), those it saw again
// (seen in it, found in another or in a review folded away) and those it
// resolved (closed, last seen in the previous review).
type Comparison struct {
	Previous   *ReviewOf
	New        []FindingOf
	Persisting []FindingOf
	Resolved   []FindingOf
}

// CompareOf compares r with the previous review of its type.
func CompareOf(ctx context.Context, c cwmapi.CWMService, r ReviewOf) (Comparison, error) {
	var out Comparison
	if r.Object.Parent == "" || r.Fields.Type == "" {
		return out, nil
	}
	siblings, err := children(ctx, c, r.Object.Parent, api.ReviewType)
	if err != nil {
		return out, err
	}
	for _, o := range siblings {
		if o.ID == r.Object.ID {
			continue
		}
		var f api.ReviewFields
		decodeFields(o, &f)
		if f.Type != r.Fields.Type || f.ReviewedAt > r.Fields.ReviewedAt || (f.ReviewedAt == r.Fields.ReviewedAt && o.ID > r.Object.ID) {
			continue
		}
		if out.Previous == nil || f.ReviewedAt > out.Previous.Fields.ReviewedAt {
			p, err := ReadReview(ctx, c, o)
			if err != nil {
				return out, err
			}
			out.Previous = &p
		}
	}
	for _, f := range r.Findings {
		switch {
		case f.FoundIn == r.Object.ID:
			out.New = append(out.New, f)
		case f.SeenIn == r.Object.ID:
			out.Persisting = append(out.Persisting, f)
		}
	}
	if out.Previous != nil {
		for _, f := range out.Previous.Findings {
			if f.SeenIn == out.Previous.Object.ID && !f.Open() {
				out.Resolved = append(out.Resolved, f)
			}
		}
	}
	return out, nil
}

// compareSection draws the comparison: the rating and score change, then the
// new, persisting and resolved findings, each opening the finding.
func compareSection(lang string, r ReviewOf, cmp Comparison) render.Node {
	if cmp.Previous == nil {
		return render.Section(tr(lang, "sincePrevious"), "git-compare").Empty(tr(lang, "noPrevious"), "")
	}
	p := cmp.Previous
	change := render.HBox(
		render.Badge(ratingText(lang, p.Fields.Rating), RatingColor(p.Fields.Rating)),
		render.Text("→"),
		render.Badge(ratingText(lang, r.Fields.Rating), RatingColor(r.Fields.Rating)),
	)
	nodes := []render.Node{change, render.Link(fmt.Sprintf(tr(lang, "previousOn"), day(p.Fields.ReviewedAt)),
		render.NavigateTo(tr(lang, "openReview"), p.Object.ID, ""))}
	if r.Fields.Score != nil && p.Fields.Score != nil {
		nodes = append(nodes, render.Text(fmt.Sprintf(tr(lang, "scoreDelta"), *p.Fields.Score, *r.Fields.Score, *r.Fields.Score-*p.Fields.Score)))
	}
	group := func(key, color string, fs []FindingOf) render.Node {
		line := []render.Node{render.Badge(fmt.Sprintf(tr(lang, key), len(fs)), color)}
		for _, f := range fs {
			line = append(line, render.Link(title(f.Object), render.NavigateTo(tr(lang, "openFinding"), f.Object.ID, "")))
		}
		return render.HBox(line...)
	}
	nodes = append(nodes, group("newFindings", "red", cmp.New), group("persistingFindings", "orange", cmp.Persisting),
		group("resolvedFindings", "green", cmp.Resolved))
	return render.Section(tr(lang, "sincePrevious"), "git-compare", nodes...)
}

// Point is one review of a type on the timeline: kept or folded away.
type Point struct {
	ReviewedAt string
	Rating     string
	Score      *int
	Open       int
	Review     string // the review's id; folded ones open nothing
	Kept       bool
}

// Timelines are the points of each review type of an object, oldest first:
// its reviewable history and the reviews it still holds.
func Timelines(obj cwmapi.Object, reviews []ReviewOf) map[string][]Point {
	out := map[string][]Point{}
	var hist api.ReviewableFields
	if g := obj.Capabilities[api.Capability]; len(g) > 0 {
		b, _ := json.Marshal(g)
		_ = json.Unmarshal(b, &hist)
	}
	for _, h := range hist.History {
		n := 0
		for _, v := range h.Open {
			n += v
		}
		out[h.Type] = append(out[h.Type], Point{ReviewedAt: h.ReviewedAt, Rating: h.Rating, Score: h.Score, Open: n, Review: h.Review})
	}
	for _, r := range reviews {
		if r.Fields.Type == "" {
			continue
		}
		out[r.Fields.Type] = append(out[r.Fields.Type], Point{ReviewedAt: r.Fields.ReviewedAt, Rating: r.Fields.Rating,
			Score: r.Fields.Score, Open: openCount(r.Findings), Review: r.Object.ID, Kept: true})
	}
	for t := range out {
		sort.SliceStable(out[t], func(i, j int) bool { return out[t][i].ReviewedAt < out[t][j].ReviewedAt })
	}
	return out
}

// scorecard draws an object's current review of each type, grouped by the
// type's dimension, then each type's timeline: a chip per review, oldest
// first, coloured by its rating with its score and open findings.
func scorecard(lang string, dimOf map[string]string, obj cwmapi.Object, reviews []ReviewOf) render.Node {
	lines := Timelines(obj, reviews)
	if len(lines) == 0 {
		return nil
	}
	byDim := map[string][]string{}
	for t := range lines {
		d := dimOf[t]
		if d == "" {
			d = "other"
		}
		byDim[d] = append(byDim[d], t)
	}
	dims := make([]string, 0, len(byDim))
	for d := range byDim {
		dims = append(dims, d)
	}
	sort.Strings(dims)
	var rows []render.Node
	for _, d := range dims {
		sort.Strings(byDim[d])
		rows = append(rows, render.Heading(d, 5))
		for _, t := range byDim[d] {
			pts := lines[t]
			cur := pts[len(pts)-1]
			line := []render.Node{render.Text(t).With("w", 140),
				render.Link(ratingText(lang, cur.Rating), render.NavigateTo(tr(lang, "openReview"), cur.Review, "")).With("color", RatingColor(cur.Rating))}
			for _, p := range pts {
				label := day(p.ReviewedAt)
				if p.Score != nil {
					label += fmt.Sprintf(" · %d", *p.Score)
				}
				label += fmt.Sprintf(" · %d", p.Open)
				line = append(line, render.Badge(label, RatingColor(p.Rating)))
			}
			rows = append(rows, render.HBox(line...))
		}
	}
	return render.Section(tr(lang, "scorecard"), "chart-dots", rows...)
}
