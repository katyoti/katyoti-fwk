// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// SeverityRank orders severities, most severe first; an unknown one last.
func SeverityRank(s string) int {
	for i, v := range api.Severities {
		if v == s {
			return i
		}
	}
	return len(api.Severities)
}

// SeverityColor is the tone of a severity.
func SeverityColor(s string) string {
	switch s {
	case "critical":
		return "red"
	case "high":
		return "orange"
	case "medium":
		return "yellow"
	case "info":
		return "blue"
	default:
		return "gray"
	}
}

// RatingColor is the tone of a rating; gray when none is written yet.
func RatingColor(v string) string {
	switch v {
	case api.RatingGood:
		return "green"
	case api.RatingNeedsAttention:
		return "orange"
	case api.RatingCritical:
		return "red"
	default:
		return "gray"
	}
}

func ratingText(lang, v string) string {
	if v == "" {
		return tr(lang, "noRating")
	}
	return v
}

// day is the day of a reviewed_at timestamp, "" when none.
func day(at string) string {
	if len(at) >= 10 {
		return at[:10]
	}
	return at
}

func short(commit string) string {
	if len(commit) > 9 {
		return commit[:9]
	}
	return commit
}

// runMeta is a review's line of facts: its type, day, commit, scope and
// reviewers, those it has.
func runMeta(f api.ReviewFields) string {
	var parts []string
	for _, p := range []string{f.Type, day(f.ReviewedAt), short(f.Commit), f.Scope, strings.Join(f.Reviewers, ", ")} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// ratingBadges are a review's rating, its score when it has one, and its
// state.
func ratingBadges(lang string, r ReviewOf) render.Node {
	badges := []render.Node{render.Badge(ratingText(lang, r.Fields.Rating), RatingColor(r.Fields.Rating))}
	if r.Fields.Score != nil {
		badges = append(badges, render.Badge(fmt.Sprintf(tr(lang, "score"), *r.Fields.Score), "blue"))
	}
	return render.HBox(append(badges, render.Badge(r.State, "gray"))...)
}

// OpenBySeverity counts a review's open findings by severity, most severe
// first; a severity with none is left out. An unrated one is named in lang.
func OpenBySeverity(lang string, fs []FindingOf) []render.Node {
	count := map[string]int{}
	for _, f := range fs {
		if f.Open() {
			count[f.Fields.Severity]++
		}
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return SeverityRank(keys[i]) < SeverityRank(keys[j]) })
	var out []render.Node
	for _, k := range keys {
		label := k
		if label == "" {
			label = tr(lang, "unrated")
		}
		out = append(out, render.Badge(fmt.Sprintf("%d %s", count[k], label), SeverityColor(k)))
	}
	return out
}

func openCount(fs []FindingOf) int {
	n := 0
	for _, f := range fs {
		if f.Open() {
			n++
		}
	}
	return n
}

func title(o cwmapi.Object) string {
	if t := o.DisplayTitle(); t != "" {
		return t
	}
	return o.ID
}

// ReviewsPage draws the reviews of obj, newest first: each a card with its
// type, day and commit, its rating, its state and its open findings by
// severity, opening the review on a click; a button creates a review under
// obj; picker (the Start a review section) and card (the scorecard and the
// timelines) come first when there are. Its
// texts are in lang (en or fr; empty = English).
func ReviewsPage(lang string, obj cwmapi.Object, reviews []ReviewOf, picker, card render.Node) render.Def {
	sort.SliceStable(reviews, func(i, j int) bool {
		if reviews[i].Fields.ReviewedAt != reviews[j].Fields.ReviewedAt {
			return reviews[i].Fields.ReviewedAt > reviews[j].Fields.ReviewedAt
		}
		return reviews[i].Object.ID > reviews[j].Object.ID
	})
	var cards []render.Node
	for _, r := range reviews {
		head := render.HBox(
			render.Link(title(r.Object), render.NavigateTo(tr(lang, "openReview"), r.Object.ID, "")),
			ratingBadges(lang, r),
		)
		meta := runMeta(r.Fields)
		if r.Fields.ReviewedAt == "" {
			meta = strings.TrimSuffix(tr(lang, "undated")+" · "+meta, " · ")
		}
		meta += fmt.Sprintf(tr(lang, "findingsOpen"), len(r.Findings), openCount(r.Findings))
		body := []render.Node{head, render.Dimmed(meta)}
		if counts := OpenBySeverity(lang, r.Findings); len(counts) > 0 {
			body = append(body, render.HBox(counts...))
		}
		if s := strings.TrimSpace(r.Fields.Summary); s != "" {
			body = append(body, render.Markdown(s).Collapsible(120))
		}
		cards = append(cards, render.Card(body...))
	}
	section := render.Section(tr(lang, "reviews"), "clipboard-check", cards...).
		Empty(fmt.Sprintf(tr(lang, "noReview"), title(obj)), "").
		Aside(render.Button(tr(lang, "newReview"), render.CreateUnder(tr(lang, "newReview"), api.ReviewType, obj.ID)))
	section["badge"] = len(reviews)
	var nodes []render.Node
	for _, n := range []render.Node{picker, card, section} {
		if n != nil {
			nodes = append(nodes, n)
		}
	}
	return render.Def{Kind: "page", Title: tr(lang, "reviews"), DSLTree: render.VBox(nodes...)}
}

// ReviewPage draws a review: its rating and summary, its findings by
// severity (each opening the finding, the object it is about a link), the
// documents and slide decks written for it, and buttons filing each under it.
// Its texts are in lang (en or fr; empty = English).
func ReviewPage(lang string, r ReviewOf, docs, decks []cwmapi.Object, since render.Node) render.Def {
	id := r.Object.ID
	head := []render.Node{ratingBadges(lang, r)}
	if meta := runMeta(r.Fields); meta != "" {
		head = append(head, render.Dimmed(meta))
	}
	if s := strings.TrimSpace(r.Fields.Summary); s != "" {
		head = append(head, render.Markdown(s).Collapsible(200))
	}
	rating := render.Section(tr(lang, "rating"), "clipboard-check", head...)

	var rows []render.Node
	for _, f := range r.Findings {
		line := []render.Node{
			render.Badge(orDash(lang, f.Fields.Severity), SeverityColor(f.Fields.Severity)),
			render.Link(title(f.Object), render.NavigateTo(tr(lang, "openFinding"), f.Object.ID, "")),
			render.Badge(f.State, "gray"),
		}
		if f.About != "" {
			line = append(line, render.Link(fmt.Sprintf(tr(lang, "about"), f.About), render.NavigateTo(fmt.Sprintf(tr(lang, "open"), f.About), f.About, "")))
		}
		item := []render.Node{render.HBox(line...)}
		if s := strings.TrimSpace(f.Fields.Statement); s != "" {
			item = append(item, render.Text(s))
		}
		if f.Fields.Location != "" {
			item = append(item, render.Dimmed(f.Fields.Location))
		}
		rows = append(rows, render.VBox(item...).With("gap", "xs"))
	}
	findings := render.Section(tr(lang, "findings"), "alert-circle", rows...).
		Empty(tr(lang, "noFinding"), "").
		Aside(append(OpenBySeverity(lang, r.Findings),
			render.Button(tr(lang, "newFinding"), render.CreateUnder(tr(lang, "newFinding"), api.FindingType, id)))...)
	findings["badge"] = len(r.Findings)

	documents := render.Section(tr(lang, "documents"), "file-text", links(lang, docs)...).
		Empty(tr(lang, "noDocument"), "").
		Aside(render.Button(tr(lang, "newDocument"), render.CreateUnder(tr(lang, "newDocument"), api.DocumentType, id)))
	slides := render.Section(tr(lang, "decks"), "presentation", links(lang, decks)...).
		Empty(tr(lang, "noDeck"), "").
		Aside(render.Button(tr(lang, "newDeck"), render.CreateUnder(tr(lang, "newDeck"), api.DeckType, id)))

	return render.Def{Kind: "page", Title: tr(lang, "review"), DSLTree: render.VBox(rating, since, findings, documents, slides)}
}

func links(lang string, objs []cwmapi.Object) []render.Node {
	sort.SliceStable(objs, func(i, j int) bool { return objs[i].ID < objs[j].ID })
	out := make([]render.Node, 0, len(objs))
	for _, o := range objs {
		out = append(out, render.Link(title(o), render.NavigateTo(fmt.Sprintf(tr(lang, "open"), o.ID), o.ID, "")))
	}
	return out
}

func orDash(lang, s string) string {
	if s == "" {
		return tr(lang, "unrated")
	}
	return s
}
