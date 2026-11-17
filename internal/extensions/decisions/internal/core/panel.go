// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
)

// resolveURI is the method each option's form runs: the decision resolves
// through its own verb, whoever asks it (the panel, the CLI, the Inbox).
const resolveURI = "ktw-methods:decisions/resolve"

// capPlan is the field-group a planned object carries; the panel reads only
// its steps' details, as data (it does not import the planner).
const capPlan = "plan"

// chooseForm is what a person says choosing an option.
type chooseForm struct {
	Reason string `json:"reason" jsonschema:"description=Why this option,required"`
}

// Panel is a decision as the page its Decision tab shows: the question and
// its reasoning; the proposal; the challenge against it; one card per option
// (what it means, for, against, its cost, what follows if chosen), the
// proposed one highlighted, each ending — while the decision is open — with a
// form choosing it, which runs decisions/resolve with its exact label. A
// decision whose options have no prose section shows its labels as plain
// cards with the same forms, under its prepare step's details when its plan
// holds them. Decided, it is read-only: what was chosen, by whom, when and
// why.
func (s *Service) Panel(ctx context.Context, in api.PanelInput) (render.Def, error) {
	if in.ID == "" {
		return render.Def{}, fmt.Errorf("%w: decisions.Panel: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	o, _, st, err := load(ctx, cwm, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	var move *cwmapi.LifecycleEntry
	if !isOpen(o, st) {
		if m, ok := lastMove(ctx, o.ID); ok {
			move = &m
		}
	}
	return render.Def{Kind: "page", DSLTree: panelTree(in.Language, o, st, move)}, nil
}

func panelTree(lang string, o cwmapi.Object, st state, move *cwmapi.LifecycleEntry) render.Node {
	status := stateOf(o)
	open := status == api.StatusOpen
	prep := api.ReadPrepared(o.Body, st.Options)
	prepared := prep.Any()

	head := []render.Node{render.HBox(
		render.Badge(o.ID, "gray").With("keepCase", true),
		render.Badge(tr(lang, status), stateTone(status)),
	)}
	head = append(head, render.Heading(o.DisplayTitle(), 3))
	if ownerOf(st) == api.OwnerAgent {
		head = append(head, render.HBox(render.Badge(tr(lang, "agentOwned"), "blue")),
			render.Dimmed(fmt.Sprintf(tr(lang, "agentOwnedWhy"), st.OwnerReason)))
	}
	if body := prep.Reasoning; body != "" {
		head = append(head, render.Markdown(body).Collapsible(260))
	}
	blocks := []render.Node{render.VBox(head...).With("gap", "xs")}

	if !open {
		blocks = append(blocks, decidedSection(lang, o, st, status, move))
	}
	blocks = append(blocks, proposalSection(lang, st), challengeSection(lang, st))
	if !prepared {
		if details := prepareDetails(o); details != "" {
			blocks = append(blocks, render.Section(tr(lang, "preparation"), "notes").With("size", "md").
				With("children", []render.Node{render.Markdown(details).Collapsible(420)}))
		}
	}
	blocks = append(blocks, optionsSection(lang, o, st, prep.Options, open))
	return render.VBox(blocks...).With("gap", "md")
}

func stateTone(status string) string {
	switch status {
	case api.StatusOpen:
		return "orange"
	case api.StatusApplies:
		return "green"
	}
	return "gray"
}

func proposalSection(lang string, st state) render.Node {
	sec := render.Section(tr(lang, "proposal"), "bulb").With("size", "md").With("color", "teal")
	p := st.Proposal
	if p == nil || p.Option == "" {
		return sec.Empty(tr(lang, "noProposal"), "")
	}
	kids := []render.Node{render.Text(p.Option).With("fw", "bold")}
	if p.Why != "" {
		kids = append(kids, render.Markdown(p.Why))
	}
	sec["children"] = kids
	return sec
}

func challengeSection(lang string, st state) render.Node {
	sec := render.Section(tr(lang, "challenge"), "swords").With("size", "md").With("color", "orange")
	c := st.Challenge
	if c == nil || len(c.Points) == 0 {
		return sec.Empty(tr(lang, "notChallenged"), "")
	}
	sec = sec.With("badge", len(c.Points)).With("badgeColor", "orange")
	sec["children"] = []render.Node{
		render.Markdown(bullets(c.Points)),
		render.Dimmed(fmt.Sprintf(tr(lang, "challengedBy"), c.By, c.At)),
	}
	return sec
}

// decidedSection is a decided decision's answer: what was chosen, by whom
// and when (its last move), and why (its `## Decision` prose).
func decidedSection(lang string, o cwmapi.Object, st state, status string, move *cwmapi.LifecycleEntry) render.Node {
	sec := render.Section(tr(lang, "decided"), "check").With("size", "md").With("color", "green")
	var kids []render.Node
	if len(st.Selected) > 0 {
		chosen := make([]render.Node, 0, len(st.Selected))
		for _, sel := range st.Selected {
			chosen = append(chosen, render.Badge(sel, "green").With("keepCase", true).With("variant", "filled"))
		}
		kids = append(kids, render.HBox(chosen...))
	} else {
		kids = append(kids, render.Text(tr(lang, "closedNoChoice")))
	}
	switch {
	case move != nil && move.By.ID != "":
		kids = append(kids, render.Dimmed(fmt.Sprintf(tr(lang, "decidedBy"), move.By.ID, dateOf(move.At))))
	case st.Resolved != "":
		kids = append(kids, render.Dimmed(fmt.Sprintf(tr(lang, "decidedOn"), st.Resolved)))
	}
	if why := section(o.Body, api.DecisionHeading); why != "" {
		kids = append(kids, render.Text(tr(lang, "why")).With("fw", "bold").With("size", "sm"), render.Markdown(why))
	}
	if status != api.StatusApplies {
		kids = append(kids, render.Dimmed(fmt.Sprintf(tr(lang, "notOpenAnymore"), tr(lang, status))))
	}
	sec["children"] = kids
	return sec
}

func optionsSection(lang string, o cwmapi.Object, st state, cards []api.PreparedOption, open bool) render.Node {
	sec := render.Section(tr(lang, "options"), "list-check").With("badge", len(cards))
	if len(cards) == 0 {
		return sec.Empty(tr(lang, "noOptions"), "")
	}
	proposed := ""
	if st.Proposal != nil {
		proposed = st.Proposal.Option
	}
	out := make([]render.Node, 0, len(cards))
	for _, c := range cards {
		out = append(out, cardNode(lang, o.ID, c, c.Label == proposed, slices.Contains(st.Selected, c.Label), open))
	}
	sec["children"] = out
	return sec
}

func cardNode(lang, id string, c api.PreparedOption, proposed, chosen, open bool) render.Node {
	title := render.Section(c.Label, "point").With("size", "md")
	var aside []render.Node
	if proposed {
		title = title.With("icon", "star").With("color", "teal")
		aside = append(aside, render.Badge(tr(lang, "proposed"), "teal").With("size", "xs"))
	}
	if chosen {
		title = title.With("icon", "check").With("color", "green")
		aside = append(aside, render.Badge(tr(lang, "chosen"), "green").With("size", "xs").With("variant", "filled"))
	}
	if len(aside) > 0 {
		title = title.Aside(aside...)
	}
	var kids []render.Node
	field := func(key, v string) {
		if v != "" {
			kids = append(kids, render.Markdown("**"+tr(lang, key)+"** - "+v))
		}
	}
	list := func(key string, vs []string) {
		if len(vs) > 0 {
			kids = append(kids, render.Markdown("**"+tr(lang, key)+"**\n\n"+bullets(vs)))
		}
	}
	field("means", c.Means)
	list("pros", c.Pros)
	list("cons", c.Cons)
	field("cost", c.Cost)
	field("ifChosen", c.Consequence)
	if c.Text != "" {
		kids = append(kids, render.Markdown(c.Text))
	}
	if open {
		if strings.Contains(c.Label, "${") {
			kids = append(kids, render.Dimmed(tr(lang, "cannotChoose")))
		} else {
			submit := render.RunURI(tr(lang, "choose"), resolveURI, map[string]any{
				"id": id, "selected": []string{c.Label}, "rationale": "${value.reason}", "actor": "${person}",
			})
			kids = append(kids, render.AnswerForm(render.InlineSchema[chooseForm]("ChooseOption", map[string]string{"reason": tr(lang, "reason")}),
				submit, "check", chooseForm{}))
		}
	}
	title["children"] = kids
	card := render.Card(title)
	if proposed || chosen {
		card = card.With("shadow", "md")
	}
	return card
}

func bullets(vs []string) string {
	lines := make([]string, 0, len(vs))
	for _, v := range vs {
		lines = append(lines, "- "+v)
	}
	return strings.Join(lines, "\n")
}

// prepareDetails is what the decision's plan says its prepare step found —
// the latest step named `…/prepare` with details — read from the object as
// data. A decision prepared before its options had prose sections keeps its
// analysis there.
func prepareDetails(o cwmapi.Object) string {
	plan, _ := o.Capabilities[capPlan]
	steps, _ := plan["steps"].([]any)
	for i := len(steps) - 1; i >= 0; i-- {
		step, _ := steps[i].(map[string]any)
		name, _ := step["step"].(string)
		details, _ := step["details"].(string)
		if (name == "prepare" || strings.HasSuffix(name, "/prepare")) && strings.TrimSpace(details) != "" {
			return strings.TrimSpace(details)
		}
	}
	return ""
}
