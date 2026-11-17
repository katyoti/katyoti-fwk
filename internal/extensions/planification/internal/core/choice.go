// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A RESULT THAT ENDS ON A CHOICE — an agent's step whose work comes to options
// for a person (an analysis with options, a review that needs an arbitration)
// reports them under more.choice. Planification opens a decision with them in
// the request's module, through ktw-methods' own decision methods (open,
// prepare, challenge: nothing is written in their place), links it to the
// request, and plans a person's decide step on it: the step ends when the
// decision is resolved (ends_on applies, on the decision), by that person,
// with the option chosen — which passes to the steps after it. The person's
// validation that came right after the step becomes that decide step (deciding
// is accepting the findings, with the option): an accept stays only for
// findings that need no choice.

// The ktw-methods decision methods a choice goes through.
const (
	decisionsOpenURI      = "ktw-methods:decisions/open"
	decisionsPrepareURI   = "ktw-methods:decisions/prepare"
	decisionsChallengeURI = "ktw-methods:decisions/challenge"
)

// stepDecide is the name of the person's step a choice plans.
const stepDecide = "decide"

// choiceOf reads the choice a result reports (its more.choice), or nil.
func choiceOf(r *schedapi.Result) *api.Choice {
	if r == nil || r.More == nil || r.More[api.ChoiceKey] == nil {
		return nil
	}
	var c api.Choice
	fromMore(r, api.ChoiceKey, &c)
	return &c
}

// checkChoice refuses a choice that is no choice: a question, at least two
// options, the proposal one of them — before anything is opened.
func checkChoice(p api.WorkPacket, c api.Choice) error {
	if p.Doer == api.DoerPerson {
		return invalid("a person's step reports no choice: their answer is the choice")
	}
	if strings.TrimSpace(c.Question) == "" {
		return invalid("a choice names its question (more.choice.question)")
	}
	if len(c.Options) < 2 {
		return invalid("a choice has at least two options (more.choice.options): one option is not a choice")
	}
	var labels []string
	for _, o := range c.Options {
		labels = append(labels, strings.TrimSpace(o.Label))
	}
	if !slices.Contains(labels, strings.TrimSpace(c.Proposal.Option)) {
		return invalid("the proposal %q is not one of the choice's options (%s)", c.Proposal.Option, strings.Join(labels, ", "))
	}
	return nil
}

// openChoice opens the decision a choice asks, in the request's module, linked
// to the request: open, prepare (its options and the proposal), challenge (the
// points already raised). It answers the decision's id.
func openChoice(ctx context.Context, wt api.WorkTodo, c api.Choice) (string, error) {
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return "", fmt.Errorf("no action service to open the decision: %w", err)
	}
	r := wt.Result
	why := strings.TrimSpace(c.Context)
	if why == "" {
		why = strings.TrimSpace(r.Summary + "\n\n" + r.Details)
	}
	// The request it came from is the decision's Source and its about edge,
	// never repeated in its text (the decision refuses that).
	var opened decisionsapi.OpenOutput
	if err := runAction(ctx, actions, decisionsOpenURI, decisionsapi.OpenInput{
		Module: wt.Module, Question: strings.TrimSpace(c.Question), Context: why, RaisedBy: r.By.ID, Source: wt.Request,
	}, &opened); err != nil {
		return "", err
	}
	id := opened.Decision.ID
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	// Opened but not prepared (an option its methods refuse): dropped, never
	// left open for nobody to decide; the step is reported again.
	drop := func(why error) (string, error) {
		_, _ = cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: decisionsapi.StatusDropped, By: r.By,
			Note: "the choice of " + wt.Request + " was refused: " + why.Error()})
		return "", why
	}
	if err := runAction(ctx, actions, decisionsPrepareURI, decisionsapi.PrepareInput{
		ID: id, Options: c.Options, Proposal: c.Proposal, Actor: r.By,
	}, nil); err != nil {
		return drop(err)
	}
	if len(c.Challenges) > 0 {
		if err := runAction(ctx, actions, decisionsChallengeURI, decisionsapi.ChallengeInput{
			ID: id, Points: c.Challenges, Actor: r.By,
		}, nil); err != nil {
			return drop(err)
		}
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: id, To: wt.Request, Kind: cwmapi.EdgeType(edgeAbout)}); err != nil {
		return drop(fmt.Errorf("link the decision %s to %s: %w", id, wt.Request, err))
	}
	return id, nil
}

// edgeAbout is CWM's own edge: what an object is about.
const edgeAbout = "about"

// runAction runs one method of another sidecar and decodes its answer.
func runAction(ctx context.Context, actions actionsapi.ActionService, uri string, in, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	res, err := actions.Run(ctx, actionsapi.RunInput{URI: uri, Input: raw})
	if err != nil {
		return fmt.Errorf("%s: %w", uri, err)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(res.Value, out); err != nil {
		return fmt.Errorf("%s answered %s: %w", uri, string(res.Value), err)
	}
	return nil
}

// decidePacket is the person's decide step on decision, its question saying
// what is expected and where: the decision, its options with the proposal
// marked, its challenge.
func decidePacket(phase, decision string, c api.Choice) api.WorkPacket {
	var b strings.Builder
	b.WriteString("Decide " + decision + ": " + strings.TrimSpace(c.Question) + "\n\nOptions:\n")
	for _, o := range c.Options {
		line := "- " + strings.TrimSpace(o.Label)
		if strings.TrimSpace(o.Label) == strings.TrimSpace(c.Proposal.Option) {
			line += " (proposed)"
		}
		if m := strings.TrimSpace(o.Means); m != "" {
			line += ": " + m
		}
		b.WriteString(line + "\n")
	}
	if why := strings.TrimSpace(c.Proposal.Why); why != "" {
		b.WriteString("\nWhy the proposal: " + why + "\n")
	}
	if len(c.Challenges) > 0 {
		b.WriteString("\nChallenged:\n")
		for _, p := range c.Challenges {
			b.WriteString("- " + strings.TrimSpace(p) + "\n")
		}
	}
	b.WriteString("\nChoose an option on the decision's panel, with your reason (ktw-methods:decisions/resolve); the step ends when it is decided, and the option chosen passes to what comes next.")
	return api.WorkPacket{Step: stepDecide, Doer: api.DoerPerson, Interaction: api.InteractionDecide, Instructions: b.String(),
		Phase: phase, EndsOn: decisionsapi.StatusApplies, On: decision}
}

// planChoice plans the decide step on decision after wt: the person's
// validation that waited on wt alone becomes it (what it verified and the move
// it made carried over, plan updated in place); with none, the step is added
// after wt — making wt's move when takesMove — and what came after waits on
// it. It answers the step added ("" when one became it).
func planChoice(ctx context.Context, req cwmapi.Object, wt api.WorkTodo, plan []api.WorkTodo, last int, decision string, c api.Choice, takesMove bool, by cwmapi.Actor) (string, error) {
	p := decidePacket(wt.Packet.Phase, decision, c)
	for i, x := range plan {
		if !notStarted(x) || x.Packet.Doer != api.DoerPerson || x.Packet.Interaction != api.InteractionValidate ||
			len(x.After) != 1 || x.After[0] != wt.ID {
			continue
		}
		p.Template, p.Verifies, p.Moves, p.Phase = x.Packet.Template, x.Packet.Verifies, x.Packet.Moves, x.Packet.Phase
		x.Packet, x.Title = p, stepDecide+" — "+titleOf(req)
		got, err := put(ctx, x)
		if err != nil {
			return "", err
		}
		plan[i] = got
		return "", nil
	}
	if takesMove {
		p.Moves = wt.Packet.Moves
	}
	// Added beyond the recipe: by whoever reported the choice, for its question.
	p.Added(wt.Packet, by, firstLine(c.Question))
	created, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
		Module: wt.Module, Target: wt.Target, Request: todoRequestRef(ctx, wt.Todo), Title: stepDecide + " — " + titleOf(req),
		For: forOf(p.Doer), Group: api.PlanGroup, Start: startOf(p.Doer), Priority: wt.Priority,
		State: schedapi.StateWaiting, Since: wt.Since, Position: last + 1, After: []string{wt.ID},
		Reads: wt.Reads, Writes: wt.Writes,
	}, Packet: p})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// endsOnObject is the object whose move ends a step that ends on a state: the
// one it names, else its target.
func endsOnObject(t api.WorkTodo) string {
	if t.Packet.On != "" {
		return t.Packet.On
	}
	return t.Target
}
