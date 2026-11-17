// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/demosignoff/api"
)

// Board is the Sign-off panel: the sign-off drawn as a page of the page DSL with
// every kind of act a page runs — a chat, a row action, a link that opens an
// object, a create dialog, a form submitted to a method with an object-ref
// picker.
func (s *Service) Board(ctx context.Context, in api.ObjectInput) (render.Def, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	got, set, err := load(ctx, c, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	ch, err := contesters(ctx, c, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	log, _ := readLog(ctx, c, in.ID) // no log yet is an empty one
	return BoardPage(in.Language, got.Object, set, ch, log.Lines), nil
}

// BoardPage draws the board of obj — what Board answers, built from what it
// read — its texts in lang (en or fr; empty = English).
func BoardPage(lang string, obj cwmapi.Object, set api.SignoffSettings, contesters []string, log []string) render.Def {
	g := GroupOf(obj)
	approved := Approvals(g)
	tone := "orange"
	if SignedOff(g, set) {
		tone = "green"
	}
	verdict := verdictOrPending(g.Verdict)
	notes := strings.TrimSpace(g.Notes)
	if notes == "" {
		notes = tr(lang, "noNotes")
	}

	signoff := render.Section(tr(lang, "signoff"), "eye-check",
		render.HBox(
			render.KPI(tr(lang, "approvals"), fmt.Sprintf("%d / %d", len(approved), set.Required()), tone, "checks"),
			render.Badge(verdict, VerdictColor(g.Verdict)),
		),
		render.Markdown(notes).Collapsible(160),
		render.Form(api.VerdictFormType, render.RunURI(tr(lang, "saveVerdict"), api.WriteURI,
			map[string]any{"id": obj.ID, "verdict": "${value.verdict}", "notes": "${value.notes}"})).
			With("mode", "edit").With("data", api.VerdictForm{Verdict: g.Verdict, Notes: g.Notes}),
	).Aside(render.Button(tr(lang, "askSignoff"), render.ChatOn(tr(lang, "askSignoff"), api.SignoffRequestTask, obj.ID)))

	given := map[string]bool{}
	for _, a := range approved {
		given[a] = true
	}
	rows := make([]api.ReviewerRow, 0, len(g.Reviewers))
	for _, r := range g.Reviewers {
		rows = append(rows, api.ReviewerRow{Name: r, Approved: given[r]})
	}
	approve := render.RunURI(tr(lang, "approve"), api.ApproveURI, map[string]any{"id": obj.ID, "reviewer": "${item.name}"})
	approve.Icon, approve.Confirm = "check", fmt.Sprintf(tr(lang, "approveAs"), obj.ID)
	var reviewers render.Node
	if len(rows) > 0 {
		reviewers = render.Section(tr(lang, "reviewers"), "users", render.Table(api.ReviewerRowType, rows, approve))
	} else {
		reviewers = render.Section(tr(lang, "reviewers"), "users").Empty(tr(lang, "noReviewers"), "")
	}

	var links []render.Node
	for _, id := range contesters {
		links = append(links, render.Link(id, render.NavigateTo(fmt.Sprintf(tr(lang, "open"), id), id, "")))
	}
	contests := render.Section(tr(lang, "contestedBy"), "swords", links...).
		Empty(tr(lang, "noContest"), "green").
		Aside(render.Button(tr(lang, "newItem"), render.CreateUnder(tr(lang, "newItem"), api.ItemType, "")))
	contests["badge"] = len(contesters)

	contest := render.Section(tr(lang, "contestAnother"), "flag",
		render.Form(api.ContestFormType, render.RunURI(tr(lang, "contest"), api.ContestURI,
			map[string]any{"id": "${value.target}", "contester": obj.ID})),
	)

	var lines []render.Node
	for i := len(log) - 1; i >= 0 && len(lines) < 5; i-- {
		lines = append(lines, render.Dimmed(log[i]))
	}
	activity := render.Section(tr(lang, "activity"), "history", lines...).Empty(tr(lang, "noActivity"), "")

	return render.Def{Kind: "page", Title: tr(lang, "signoff"), DSLTree: render.VBox(signoff, reviewers, contests, contest, activity)}
}

// ContestsOf is the Contests panel's rows: every object that contests
// in.ID, with its own sign-off's verdict.
func (s *Service) ContestsOf(ctx context.Context, in api.ObjectInput) ([]api.ContestRow, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := contesters(ctx, c, in.ID)
	if err != nil {
		return nil, err
	}
	rows := make([]api.ContestRow, 0, len(ch))
	for _, id := range ch {
		row := api.ContestRow{ID: id}
		if module, err := cwmapi.ModuleOfObjectID(id); err == nil {
			if got, err := c.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id}); err == nil {
				row.Name, row.Type, row.Verdict = got.Object.Name, got.Object.Type, GroupOf(got.Object).Verdict
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
