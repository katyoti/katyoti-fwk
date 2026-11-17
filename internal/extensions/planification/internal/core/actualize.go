// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// ACTUALIZING AN OPEN REQUEST — what was asked changes, and the open request
// follows: rescoped (reworded, the old wording kept in its details), superseded
// by another (it ends dropped; what depended on it depends on its successor;
// its requester is told) or merged into another (the same, its content carried
// over). A step not started follows; a running one is never rewritten nor
// taken from its worker — it is marked stale, and its report is recorded
// saying so.

// Rescope rewords an open request.
func (s *Service) Rescope(ctx context.Context, in api.RescopeInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	note, title, details := strings.TrimSpace(cleanText(in.Note)), strings.TrimSpace(cleanText(in.Title)), strings.TrimSpace(cleanText(in.Details))
	if note == "" {
		return api.RequestOutput{}, invalid("Rescope: say why it changes (note)")
	}
	if title == "" && details == "" {
		return api.RequestOutput{}, invalid("Rescope: give its new title, its new details, or both")
	}
	o, f, err := openRequest(ctx, "Rescope", in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if title != "" && o.Type != "request" {
		return api.RequestOutput{}, invalid("Rescope: %s is a request on a %s - its title is the %s's name; rescope its details", in.ID, o.Type, o.Type)
	}
	was := "«" + titleOf(o) + "»"
	if details != "" && strings.TrimSpace(f.Details) != "" {
		was += "\n\n" + quote(f.Details)
	}
	if title != "" {
		o.Name = title
	}
	if details != "" {
		f.Details = details
	}
	f.Details = strings.TrimRight(f.Details, "\n") + fmt.Sprintf("\n\n_Rescoped by %s on %s: %s. It read:_ %s", in.By.ID, today(), note, was)
	out, err := saveRequest(ctx, o, f)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if out, err = followTodos(ctx, out, "rescoped: "+note); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(out)}, nil
}

// Supersede ends an open request for the one that carries its work.
func (s *Service) Supersede(ctx context.Context, in api.SupersedeInput) (api.RequestOutput, error) {
	return supersede(ctx, "Supersede", in.ID, in.With, in.Note, in.By, false)
}

// Merge merges an open request into another, then ends it as superseded.
func (s *Service) Merge(ctx context.Context, in api.MergeInput) (api.RequestOutput, error) {
	return supersede(ctx, "Merge", in.ID, in.Into, in.Note, in.By, true)
}

// supersede ends id for with: merged (its content carried over) or not.
func supersede(ctx context.Context, verb, id, with, note string, by cwmapi.Actor, merge bool) (api.RequestOutput, error) {
	if err := checkActor(by); err != nil {
		return api.RequestOutput{}, err
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return api.RequestOutput{}, invalid("%s: say why (note)", verb)
	}
	if id == with {
		return api.RequestOutput{}, invalid("%s: %s cannot replace itself", verb, id)
	}
	x, xf, err := openRequest(ctx, verb, id)
	if err != nil {
		return api.RequestOutput{}, err
	}
	y, yf, err := openRequest(ctx, verb, with)
	if err != nil {
		return api.RequestOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if edgeTo(y, api.EdgeSplitFrom) == id {
		return api.RequestOutput{}, invalid("%s: %s is a part of %s - a part never replaces its whole", verb, with, id)
	}
	// What depended on it waits on its successor: refused when that closes a cycle.
	dependents, err := incoming(ctx, cwm, id, api.EdgeDependsOn)
	if err != nil {
		return api.RequestOutput{}, err
	}
	for _, d := range dependents {
		if d == with {
			continue
		}
		if path, cycle, err := reaches(ctx, cwm, with, d); err != nil {
			return api.RequestOutput{}, err
		} else if cycle {
			return api.RequestOutput{}, invalid("%s: %s depends on %s; waiting on %s instead closes a cycle (%s)", verb, d, id, with, strings.Join(append([]string{with}, path...), " → "))
		}
	}
	if merge {
		todos, err := todosOf(ctx, with)
		if err != nil {
			return api.RequestOutput{}, err
		}
		if slices.ContainsFunc(todos, func(t api.WorkTodo) bool { return t.State == schedapi.StateClaimed }) {
			return api.RequestOutput{}, invalid("Merge: %s has work running - its content cannot change under it; supersede %s with it instead", with, id)
		}
		if y, err = carryOver(ctx, cwm, x, xf, y, yf, note, by); err != nil {
			return api.RequestOutput{}, err
		}
	}
	for _, d := range dependents {
		if d == with {
			continue
		}
		if err := repoint(ctx, cwm, d, id, with); err != nil {
			return api.RequestOutput{}, err
		}
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: id, To: with, Kind: cwmapi.EdgeType(api.EdgeSupersededBy)}); err != nil {
		return api.RequestOutput{}, fmt.Errorf("requests.%s: %w", verb, err)
	}
	how := "superseded by " + with
	if merge {
		how = "merged into " + with
	}
	if x, err = followTodos(ctx, x, how+": "+note); err != nil {
		return api.RequestOutput{}, err
	}
	if _, err := NewPlans().Finish(ctx, api.FinishInput{ID: id, Outcome: api.PlanDropped, By: by, Note: how + ": " + note}); err != nil {
		return api.RequestOutput{}, err
	}
	tellRequester(ctx, x, xf, how, note)
	if err := settleRequest(ctx, with, PlanningActor); err != nil {
		return api.RequestOutput{}, err
	}
	if x, err = load(ctx, cwm, id); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(x)}, nil
}

// carryOver merges x's content into y: its details under a heading, its
// deliverables not there yet (by title, unverified), its want and release
// when y has none, its dependencies.
func carryOver(ctx context.Context, cwm cwmapi.CWMService, x cwmapi.Object, xf api.RequestFields, y cwmapi.Object, yf api.RequestFields, note string, by cwmapi.Actor) (cwmapi.Object, error) {
	merged := fmt.Sprintf("_Merged from %s by %s on %s: %s. Asked by %s:_ «%s»", x.ID, by.ID, today(), note, xf.Requester.ID, titleOf(x))
	if strings.TrimSpace(xf.Details) != "" {
		merged += "\n\n" + quote(xf.Details)
	}
	yf.Details = strings.TrimRight(yf.Details, "\n") + "\n\n" + merged
	for _, d := range xf.Deliverables {
		if !slices.ContainsFunc(yf.Deliverables, func(e api.Deliverable) bool { return e.Title == d.Title }) {
			yf.Deliverables = append(yf.Deliverables, api.Deliverable{Title: d.Title, Check: d.Check, ByHand: d.ByHand})
		}
	}
	for _, kind := range []string{api.EdgeServes, api.EdgeTargets} {
		if to := edgeTo(x, kind); to != "" && edgeTo(y, kind) == "" {
			y.Edges = append(y.Edges, cwmapi.ObjectEdge{Kind: kind, To: to})
		}
	}
	for _, dep := range dependsOn(x) {
		if dep == y.ID || slices.Contains(dependsOn(y), dep) {
			continue
		}
		if _, cycle, err := reaches(ctx, cwm, dep, y.ID); err != nil {
			return cwmapi.Object{}, err
		} else if cycle {
			continue // its successor is already before it
		}
		y.Edges = append(y.Edges, cwmapi.ObjectEdge{Kind: api.EdgeDependsOn, To: dep})
	}
	return saveRequest(ctx, y, yf)
}

// repoint moves d's dependency on from to its successor.
func repoint(ctx context.Context, cwm cwmapi.CWMService, d, from, to string) error {
	o, err := loadObject(ctx, cwm, d)
	if err != nil {
		return err
	}
	has := slices.Contains(dependsOn(o), to)
	o.Edges = slices.DeleteFunc(o.Edges, func(e cwmapi.ObjectEdge) bool { return e.Kind == api.EdgeDependsOn && e.To == from })
	if !has {
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeDependsOn, To: to})
	}
	if _, err := update(ctx, cwm, o); err != nil {
		return err
	}
	return settleRequest(ctx, d, PlanningActor)
}

// followTodos makes a request's to-dos follow what changed: those not started
// take its title; one a worker holds is never rewritten — its plan records it
// stale, and its step's record will say so.
func followTodos(ctx context.Context, o cwmapi.Object, why string) (cwmapi.Object, error) {
	todos, err := todosOf(ctx, o.ID)
	if err != nil {
		return o, err
	}
	var stale []api.StaleStep
	for _, t := range todos {
		switch {
		case notStarted(t):
			t.Title = t.Packet.Step + " — " + titleOf(o)
			if _, err := put(ctx, t); err != nil {
				return o, err
			}
		case t.State == schedapi.StateClaimed:
			stale = append(stale, api.StaleStep{Todo: t.ID, Why: why})
		}
	}
	f, has := planOf(o)
	if len(stale) == 0 || !has {
		return o, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return o, err
	}
	f.Stale = append(f.Stale, stale...)
	return writePlan(ctx, cwm, o, f, true)
}

// tellRequester tells a person whose request was superseded what became of it.
func tellRequester(ctx context.Context, x cwmapi.Object, xf api.RequestFields, how, note string) {
	if xf.Requester.Kind != "person" {
		return
	}
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil || inbox == nil {
		return
	}
	if _, err := inbox.File(ctx, inboxapi.FileInput{Title: fmt.Sprintf("Your request %s was %s", x.ID, how),
		Description: fmt.Sprintf("«%s» is %s: %s. Its work goes on there.", titleOf(x), how, note),
		RequestedBy: "planification:" + x.ID, About: x.ID}); err != nil {
		slog.Warn("planification: the requester of a superseded request was not told", "request", x.ID, "error", err)
	}
}

// quote is text as a markdown quote.
func quote(s string) string {
	return "> " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n> ")
}

func today() string { return time.Now().UTC().Format("2006-01-02") }
