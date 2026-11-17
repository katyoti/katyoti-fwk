// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// EDITING A PLAN — a request's to-dos are changed before they run: one's
// packet edited, a step added, one taken out (archived, saying why), what one
// comes after set; a failed or blocked one sent back. A to-do a worker has is
// never changed; after each change every to-do is settled again.

// openTodo reads a to-do that may be changed: one not started.
func openTodo(ctx context.Context, id, verb string) (api.WorkTodo, error) {
	t, err := getTodo(ctx, id)
	if err != nil {
		return api.WorkTodo{}, err
	}
	if t.Owner != api.Owner {
		return api.WorkTodo{}, invalid("%s: %s is not planification's", verb, id)
	}
	if !notStarted(t) {
		return api.WorkTodo{}, invalid("%s: %s is %s — a to-do a worker has is not changed", verb, id, t.State)
	}
	return t, nil
}

// Edit changes a to-do not started: only what is given changes.
func (w *Work) Edit(ctx context.Context, in api.EditWorkInput) (api.WorkTodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	t, err := openTodo(ctx, in.ID, "Edit")
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	if in.Task != "" {
		t.Packet.Task = in.Task
	}
	if in.Skill != "" {
		if err := checkSkill(in.Skill); err != nil {
			return api.WorkTodoOutput{}, err
		}
		t.Skill = in.Skill
	}
	if in.Instructions != "" {
		t.Packet.Instructions = in.Instructions
	}
	if in.Params != nil {
		t.Packet.Params = in.Params
	}
	if in.Priority != "" {
		if !slices.Contains(api.Priorities, in.Priority) {
			return api.WorkTodoOutput{}, invalid("Edit: priority %q is not one of %v", in.Priority, api.Priorities)
		}
		t.Priority = in.Priority
	}
	if in.Start != "" {
		if in.Start != schedapi.StartAuto && in.Start != schedapi.StartManual {
			return api.WorkTodoOutput{}, invalid("Edit: start is auto or manual, got %q", in.Start)
		}
		t.Start = in.Start
	}
	saved, err := put(ctx, t)
	return api.WorkTodoOutput{Todo: saved}, err
}

// Add adds a step to a planned request, after some of its to-dos and before
// others not started.
func (w *Work) Add(ctx context.Context, in api.AddWorkInput) (api.WorkTodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	if in.Step == "" || in.Instructions == "" {
		return api.WorkTodoOutput{}, invalid("Add: a step needs its name and what to do (instructions)")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	rid := requestRef(ctx, in.Request) // in its project (requestRef)
	o, err := load(ctx, cwm, rid)
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	plan, err := todosOf(ctx, rid)
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	if len(plan) == 0 {
		return api.WorkTodoOutput{}, invalid("Add: %s has nothing scheduled — plan it first (work plan)", rid)
	}
	byID := map[string]api.WorkTodo{}
	last := 0
	for _, t := range plan {
		byID[t.ID] = t
		last = max(last, t.Position)
	}
	if err := checkAfter(byID, "", in.After); err != nil {
		return api.WorkTodoOutput{}, err
	}
	for _, b := range in.Before {
		t, ok := byID[b]
		switch {
		case !ok:
			return api.WorkTodoOutput{}, invalid("Add: %s is not a to-do of %s", b, rid)
		case !notStarted(t):
			return api.WorkTodoOutput{}, invalid("Add: %s has started (%s): nothing goes before it now", b, t.State)
		case slices.Contains(in.After, b):
			return api.WorkTodoOutput{}, invalid("Add: %s cannot be both before and after the new step", b)
		}
	}
	p := api.WorkPacket{Step: in.Step, Doer: in.Doer, Task: in.Task, Interaction: in.Interaction, Instructions: in.Instructions,
		Phase: in.Phase, Params: in.Params}
	if p.Doer == "" {
		p.Doer = api.DoerTask
	}
	if p.Phase == "" {
		p.Phase = addedPhase(byID, in.After, in.Before, p)
	}
	// Added by hand: by its caller, one generation after the furthest step
	// it comes after - the first when it comes after none.
	var from api.WorkPacket
	for _, a := range in.After {
		if g := byID[a].Packet; g.Generation > from.Generation {
			from = g
		}
	}
	why := strings.TrimSpace(in.Why)
	if why == "" {
		why = "added by hand (work add)"
	}
	p.Added(from, in.By, firstLine(why))
	priority := in.Priority
	if priority == "" {
		if priority = fieldsOf(o).Priority; priority == "" {
			priority = api.DefaultPriority
		}
	}
	if err := checkPacket(p, priority, in.Skill); err != nil {
		return api.WorkTodoOutput{}, err
	}
	module, _ := cwmapi.ModuleOfObjectID(bareID(rid)) // put names it in the request's project
	reads, writes := []string{module}, []string(nil)
	if p.Phase == api.PhaseBuild && p.Doer != api.DoerPerson {
		reads, writes = nil, []string{module}
	}
	saved, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
		Module: module, Target: rid, Request: rid, Title: in.Step + " — " + titleOf(o), For: forOf(p.Doer), Skill: in.Skill,
		Group: api.PlanGroup, Start: startOf(p.Doer), Priority: priority, State: schedapi.StateWaiting,
		Since: fieldsOf(o).FiledAt, Position: last + 1, After: in.After, Reads: reads, Writes: writes,
	}, Packet: p})
	if err != nil {
		return api.WorkTodoOutput{}, fmt.Errorf("Add: %w", err)
	}
	for _, b := range in.Before {
		bt := byID[b]
		bt.After = append(bt.After, saved.ID)
		if _, err := put(ctx, bt); err != nil {
			return api.WorkTodoOutput{}, err
		}
	}
	if err := settleRequest(ctx, rid, in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	got, err := getTodo(ctx, saved.ID)
	return api.WorkTodoOutput{Todo: got}, err
}

// addedPhase is the phase of a step added with none given: preparation when it
// goes before a preparation step, or when it reviews (a person's validation or
// challenge, a reviewer's challenge) placed after preparation steps only — it
// then reads its module, writes nothing, and waits only until a dependency's
// preparation is done; build otherwise.
func addedPhase(byID map[string]api.WorkTodo, after, before []string, p api.WorkPacket) string {
	prep := func(id string) bool {
		t, ok := byID[id]
		return ok && api.PhaseIndex(t.Packet.Phase) >= 0 && api.PhaseIndex(t.Packet.Phase) <= api.PhaseIndex(api.PhasePreparation)
	}
	if slices.ContainsFunc(before, prep) {
		return api.PhasePreparation
	}
	if len(after) > 0 && !slices.ContainsFunc(after, func(id string) bool { return !prep(id) }) && reviews(p) {
		return api.PhasePreparation
	}
	return api.PhaseBuild
}

// reviewerPersona is the catalog persona that challenges a piece of work.
const reviewerPersona = "reviewer"

// reviews is a step that reviews the work before it: a person's validation or
// challenge, or an agent's challenge (the reviewer persona's).
func reviews(p api.WorkPacket) bool {
	if p.Doer == api.DoerPerson {
		return p.Interaction == api.InteractionValidate || p.Interaction == api.InteractionChallenge
	}
	return p.Interaction == api.InteractionChallenge || p.Task == reviewerPersona
}

// Remove takes a to-do not started out of the plan: archived, saying why; what
// came after it waits on what it came after instead.
func (w *Work) Remove(ctx context.Context, in api.RemoveWorkInput) (api.WorkTodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	if in.Note == "" {
		return api.WorkTodoOutput{}, invalid("Remove: say why it goes (note)")
	}
	t, err := openTodo(ctx, in.ID, "Remove")
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	if t.Packet.Starts != "" || t.Packet.Moves != "" {
		return api.WorkTodoOutput{}, invalid("Remove: %s moves its node (%s%s) — plan the request again instead", t.ID, t.Packet.Starts, t.Packet.Moves)
	}
	plan, err := todosOf(ctx, t.Request)
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	for _, x := range plan {
		if x.ID == t.ID || !notStarted(x) || !slices.Contains(x.After, t.ID) {
			continue
		}
		var after []string
		for _, a := range x.After {
			if a != t.ID {
				after = append(after, a)
				continue
			}
			for _, inherited := range t.After {
				if !slices.Contains(after, inherited) && !slices.Contains(x.After, inherited) {
					after = append(after, inherited)
				}
			}
		}
		x.After = after
		if _, err := put(ctx, x); err != nil {
			return api.WorkTodoOutput{}, err
		}
	}
	if err := archive(ctx, t.ID, "removed by "+in.By.ID+": "+in.Note); err != nil {
		return api.WorkTodoOutput{}, err
	}
	// A person's step asked in the Inbox: its question goes with it, never
	// left pending for an answer nothing would collect.
	closeAsked(ctx, t, "removed: "+in.Note)
	if err := settleRequest(ctx, t.Request, in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	got, err := getTodo(ctx, t.ID)
	return api.WorkTodoOutput{Todo: got}, err
}

// Move sets what a to-do not started comes after; a cycle is refused.
func (w *Work) Move(ctx context.Context, in api.MoveWorkInput) (api.WorkTodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	t, err := openTodo(ctx, in.ID, "Move")
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	plan, err := todosOf(ctx, t.Request)
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	byID := map[string]api.WorkTodo{}
	for _, x := range plan {
		byID[x.ID] = x
	}
	if err := checkAfter(byID, t.ID, in.After); err != nil {
		return api.WorkTodoOutput{}, err
	}
	seen := map[string]bool{}
	var reaches func(id string) bool
	reaches = func(id string) bool {
		if id == t.ID {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		return slices.ContainsFunc(byID[id].After, reaches)
	}
	if slices.ContainsFunc(in.After, reaches) {
		return api.WorkTodoOutput{}, invalid("Move: %s after %v makes a cycle", t.ID, in.After)
	}
	t.After = in.After
	if _, err := put(ctx, t); err != nil {
		return api.WorkTodoOutput{}, err
	}
	if err := settleRequest(ctx, t.Request, in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	got, err := getTodo(ctx, t.ID)
	return api.WorkTodoOutput{Todo: got}, err
}

// Retry sends a failed or blocked to-do back to the queue.
func (w *Work) Retry(ctx context.Context, in api.RetryWorkInput) (api.WorkTodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	t, err := openTodo(ctx, in.ID, "Retry")
	if err != nil {
		return api.WorkTodoOutput{}, err
	}
	if !stopped(t) {
		return api.WorkTodoOutput{}, invalid("Retry: %s neither failed nor is blocked", t.ID)
	}
	clearStop(&t)
	if _, err := put(ctx, t); err != nil {
		return api.WorkTodoOutput{}, err
	}
	if err := settleRequest(ctx, t.Request, in.By); err != nil {
		return api.WorkTodoOutput{}, err
	}
	got, err := getTodo(ctx, t.ID)
	return api.WorkTodoOutput{Todo: got}, err
}

// checkAfter refuses an after naming anything but the request's other
// scheduled to-dos.
func checkAfter(byID map[string]api.WorkTodo, self string, after []string) error {
	for _, a := range after {
		if a == self {
			return invalid("a to-do cannot come after itself (%s)", a)
		}
		if _, ok := byID[a]; !ok {
			return invalid("%s is not a scheduled to-do of the same request", a)
		}
	}
	return nil
}

// checkPacket refuses a step a worker could not act on.
func checkPacket(p api.WorkPacket, priority, skill string) error {
	switch p.Doer {
	case api.DoerTask, api.DoerPersona:
		if p.Task == "" {
			return invalid("a %s step names its task", p.Doer)
		}
	case api.DoerPerson:
		if !slices.Contains([]string{api.InteractionValidate, api.InteractionChallenge, api.InteractionDecide, api.InteractionProvide}, p.Interaction) {
			return invalid("a person's step says what it asks (validate, challenge, decide or provide), got %q", p.Interaction)
		}
	default:
		return invalid("doer is task, persona or person, got %q", p.Doer)
	}
	if api.PhaseIndex(p.Phase) < 0 {
		return invalid("phase is one of %v, got %q", api.Phases, p.Phase)
	}
	if !slices.Contains(api.Priorities, priority) {
		return invalid("priority is one of %v, got %q", api.Priorities, priority)
	}
	if skill != "" {
		return checkSkill(skill)
	}
	return nil
}

// checkSkill refuses a skill that is not a skill URI.
func checkSkill(s string) error {
	if u, err := contribapi.ParseResourceURI(s); err != nil || u.Type != "skill" {
		return invalid("skill %q is not a skill URI (<provider>:/skill/<name>)", s)
	}
	return nil
}

// confirmRequirement records a person's approval of a gate requirement on a
// node (gate/requirement).
func confirmRequirement(ctx context.Context, node, confirms string, by cwmapi.Actor) error {
	gate, requirement, ok := strings.Cut(confirms, "/")
	if !ok {
		return fmt.Errorf("confirms %q is not <gate>/<requirement>", confirms)
	}
	svc, err := application.UseService[gatesapi.GatesService](ctx)
	if err != nil {
		return fmt.Errorf("the gates service: %w", err)
	}
	_, err = svc.Confirm(ctx, gatesapi.ConfirmInput{ID: node, Gate: gate, Requirement: requirement, By: by})
	return err
}
