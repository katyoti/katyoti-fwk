// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN AGENT TESTS FIRST — a deliverable no check verifies goes to an agent
// before a person: the auto-test step writes the check that proves it (shown
// to fail once), records it on the request (Revise) and runs it (Check); one
// only a person can judge it marks by_hand — the exact steps and why no agent
// can — and only that one reaches the person. Asked for (AutoTest), or taken
// by the request's end when what refuses it is only untested deliverables;
// once per deliverable: a second refusal blocks as before.

// autoTestInstructions is what the auto-test step does.
const autoTestInstructions = "For each deliverable named in params.deliverables: write the check that proves it - a Go or Bun test, a CLI probe, a scenario case run - and show it can fail (break what it tests, see it fail, restore it from a kept copy). " +
	"Record it on the request (requests revise, the full list, the falsification in the note) and run it (requests check). " +
	"A deliverable already marked by_hand is tried too: a scenario case (ktw-compose scenario) on its own universe, or a task run on a scripted case for a model's judgement, often judges it - give it that check and drop its by_hand. " +
	"When only a person can judge a deliverable (what a person sees or decides), give it no check: revise it with by_hand saying the exact steps and why no test can judge it. " +
	"A check that fails because the work is wrong is a failure of the work: report it with more.add an implement step and a new auto-test, saying what failed."

// AutoTest asks an agent to test a request's deliverables no check verifies.
func (s *Service) AutoTest(ctx context.Context, in api.AutoTestInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	o, f, err := openRequest(ctx, "AutoTest", in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	titles := untested(f.Deliverables)
	if len(in.Deliverables) > 0 {
		for _, t := range in.Deliverables {
			if !slices.Contains(titles, t) {
				return api.RequestOutput{}, invalid("AutoTest: %q is not a deliverable of %s that no check verifies", t, in.ID)
			}
		}
		titles = in.Deliverables
	}
	if len(titles) == 0 {
		return api.RequestOutput{}, invalid("AutoTest: %s has no deliverable that no check verifies", in.ID)
	}
	if planState(o) == api.PlanBlocked {
		if _, err := NewPlans().move(ctx, o.ID, api.PlanReady, "plan", in.By, "an agent tests its deliverables: "+strings.Join(titles, ", "), nil); err != nil {
			return api.RequestOutput{}, err
		}
	}
	if err := scheduleAutoTest(ctx, o, titles, in.By, "an agent tests its deliverables: "+strings.Join(titles, ", ")); err != nil {
		return api.RequestOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if o, err = load(ctx, cwm, in.ID); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(o)}, nil
}

// untested is the deliverables no check verifies and no person was named to
// test: not verified, no check, no by_hand.
func untested(ds []api.Deliverable) []string {
	var titles []string
	for _, d := range ds {
		if !d.Verified && strings.TrimSpace(d.Check) == "" && strings.TrimSpace(d.ByHand) == "" {
			titles = append(titles, d.Title)
		}
	}
	return titles
}

// handOnly is the deliverables marked for a person's hand test, not verified:
// an auto-test tries each once before the review.
func handOnly(ds []api.Deliverable) []string {
	var titles []string
	for _, d := range ds {
		if !d.Verified && strings.TrimSpace(d.Check) == "" && strings.TrimSpace(d.ByHand) != "" {
			titles = append(titles, d.Title)
		}
	}
	return titles
}

// autoTestAtEnd is the deliverables a request's end gives to an agent rather
// than block on: when every unverified deliverable is untested and none was
// given to an auto-test before (once per deliverable). Nil otherwise: a failing
// check, or a deliverable a person tests, blocks as before.
func autoTestAtEnd(f api.RequestFields, done []api.WorkTodo) []string {
	titles := untested(f.Deliverables)
	if len(titles) == 0 {
		return nil
	}
	for _, d := range f.Deliverables {
		if !d.Verified && !slices.Contains(titles, d.Title) {
			return nil
		}
	}
	for _, t := range done {
		if t.Packet.Step != api.StepAutoTest {
			continue
		}
		for _, title := range autoTestTitles(t.Packet) {
			if slices.Contains(titles, title) {
				return nil
			}
		}
	}
	return titles
}

// autoTestTitles is the deliverables an auto-test step was given.
func autoTestTitles(p api.WorkPacket) []string {
	var out []string
	switch v := p.Params["deliverables"].(type) {
	case []string:
		out = v
	case []any:
		for _, x := range v {
			out = append(out, fmt.Sprint(x))
		}
	}
	return out
}

// testBeforeReview puts an agent's auto-test between a step and the review -
// a person's or an agent's - of what was built after it (a validation in
// Verify), when that review's approval verifies the deliverables no
// check verifies (verifies: true) and some were never given to an agent: an
// agent tests first (rule), the approval verifies only what
// is left — what a person alone can judge. Only the review waits on it.
func testBeforeReview(ctx context.Context, req cwmapi.Object, wt api.WorkTodo, plan []api.WorkTodo, last int) error {
	if wt.Packet.Step == api.StepAutoTest {
		return nil
	}
	var reviews []api.WorkTodo
	for _, x := range plan {
		if x.Packet.Step == api.StepAutoTest && notStarted(x) {
			return nil // one waits already
		}
		if notStarted(x) && slices.Contains(x.After, wt.ID) &&
			x.Packet.Interaction == api.InteractionValidate &&
			x.Packet.Phase == api.PhaseVerify && (x.Packet.Verifies == api.VerifiesUnchecked || len(handOnly(fieldsOf(req).Deliverables)) > 0) {
			reviews = append(reviews, x)
		}
	}
	if len(reviews) == 0 {
		return nil
	}
	done, err := doneOf(ctx, req.ID)
	if err != nil {
		return err
	}
	var titles []string
	for _, t := range append(untested(fieldsOf(req).Deliverables), handOnly(fieldsOf(req).Deliverables)...) {
		if !slices.ContainsFunc(done, func(d api.WorkTodo) bool {
			return d.Packet.Step == api.StepAutoTest && slices.Contains(autoTestTitles(d.Packet), t)
		}) {
			titles = append(titles, t)
		}
	}
	if len(titles) == 0 {
		return nil
	}
	p := api.WorkPacket{Step: api.StepAutoTest, Doer: api.DoerTask, Task: api.AutoTestTask, Phase: api.PhaseVerify,
		Instructions: autoTestInstructions, Params: map[string]any{"deliverables": titles}}
	p.Added(wt.Packet, PlanningActor, "an agent tests what no check verifies before the review: "+strings.Join(titles, ", "))
	if err := checkPacket(p, wt.Priority, ""); err != nil {
		return err
	}
	module, _ := cwmapi.ModuleOfObjectID(req.ID)
	created, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
		Module: module, Target: wt.Target, Request: todoRequestRef(ctx, wt.Todo), Title: api.StepAutoTest + " — " + titleOf(req),
		For: forOf(p.Doer), Group: api.PlanGroup, Start: startOf(p.Doer), Priority: wt.Priority,
		State: schedapi.StateWaiting, Since: wt.Since, Position: last + 1, After: []string{wt.ID},
		Reads: []string{module}, Writes: []string{module},
	}, Packet: p})
	if err != nil {
		return err
	}
	for _, x := range reviews {
		x.After = append(x.After, created.ID)
		if _, err := put(ctx, x); err != nil {
			return err
		}
	}
	return nil
}

// scheduleAutoTest puts the auto-test step after a request's last step: an
// agent's step in Verify that writes the module (its tests), then settles it —
// inside a collection, the collection's own settle does. It says it was added
// by `by` for `why`, a first generation: it follows no to-do.
func scheduleAutoTest(ctx context.Context, o cwmapi.Object, titles []string, by cwmapi.Actor, why string) error {
	open, err := todosOf(ctx, o.ID)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(open, func(t api.WorkTodo) bool { return t.Packet.Step == api.StepAutoTest && notStarted(t) }) {
		return invalid("AutoTest: %s already has an auto-test waiting", o.ID)
	}
	done, err := doneOf(ctx, o.ID)
	if err != nil {
		return err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	last := 0
	for _, t := range append(slices.Clone(done), open...) {
		last = max(last, t.Position)
	}
	priority := api.DefaultPriority
	if pf, ok := planOf(o); ok && pf.Priority != "" {
		priority = pf.Priority
	}
	p := api.WorkPacket{Step: api.StepAutoTest, Doer: api.DoerTask, Task: api.AutoTestTask, Phase: api.PhaseVerify,
		Instructions: autoTestInstructions, Params: map[string]any{"deliverables": titles}}
	p.Added(api.WorkPacket{}, by, why)
	if err := checkPacket(p, priority, ""); err != nil {
		return err
	}
	if _, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
		Module: module, Target: o.ID, Request: requestRef(ctx, o.ID), Title: api.StepAutoTest + " — " + titleOf(o),
		For: forOf(p.Doer), Group: api.PlanGroup, Start: startOf(p.Doer), Priority: priority,
		State: schedapi.StateWaiting, Since: fieldsOf(o).FiledAt, Position: last + 1,
		Reads: []string{module}, Writes: []string{module},
	}, Packet: p}); err != nil {
		return err
	}
	if !collecting(ctx) {
		return settleRequest(ctx, o.ID, PlanningActor)
	}
	return nil
}
