// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT IS DEFINED BY ITS DELIVERY GOALS — what it promises, in the
// user's words, kept on it. Define now, plan when run (rule):
// defined, nothing is planned for it; when a person runs it, a proposal is
// filed on the increment: a demand whose template (increment-delivery) first
// analyses the goals against the day's state, then reconciles them across the
// modules into a delta - the module goals and features and direct work that
// deliver them, each op naming the delivery goal it answers (`for`) - the
// person approves once, an agent applies it. Applied, the increment aims at
// what the delta created and its own work is planned (plan-work).

// cleanGoals trims the delivery goals and drops the empty ones.
func cleanGoals(goals []string) []string {
	var out []string
	for _, g := range goals {
		if g = strings.TrimSpace(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}

// setDelivery writes an increment's delivery goals; one not run yet keeps
// them and nothing more (Run files its proposal). Goals changed once a proposal answered them are proposed
// again, alone: a reworded goal - the one goal gone for the one come, not
// dropped - keeps its operations (their `for` follows its new words, so the
// pane keeps what it became); no other operation changes its goal; the
// operations not yet applied for a goal reworded, added or removed give way;
// then the proposal still open is planned again for the changed goals, or -
// applied - a follow-up proposal is filed for them, what was applied kept.
func setDelivery(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, goals, drop []string, by cwmapi.Actor) (cwmapi.Object, error) {
	f := releaseFields(o)
	before := f.Delivery
	f.Delivery = cleanGoals(goals)
	if err := o.SetCapability(api.CapRelease, f); err != nil {
		return o, err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return o, err
	}
	o = out.Object
	if len(f.Delivery) == 0 {
		return o, nil
	}
	if by.ID == "" {
		by = PlanningActor
	}
	if !hasRequest(o) {
		return o, nil // defined: nothing is planned until a person runs it
	}
	rf := fieldsOf(o)
	if len(rf.Delta) == 0 {
		// No delta yet: its reconcile reads the goals as they now are; an
		// ended proposal that made none is filed again.
		if planEnded(stateOf(o)) {
			if _, err := New().File(ctx, api.FileInput{On: o.ID, Kind: api.KindDemand, By: by}); err != nil {
				return o, err
			}
			return loadIncrement(ctx, cwm, o.ID)
		}
		return o, nil
	}
	renamed, changed, removed := goalChanges(before, f.Delivery, drop)
	if len(changed) == 0 && len(removed) == 0 {
		return o, nil
	}
	delta, applied := regoal(rf.Delta, rf.Applied, renamed, append(append([]string{}, changed...), removed...))
	details := roundDetails(changed)
	if len(changed) == 0 {
		// Only removed: what waited for them is gone, nothing to propose; a
		// round still open stops listing them.
		rf.Delta, rf.Applied = delta, applied
		if pending := roundGoals(rf.Details, renamed, f.Delivery, nil); len(pending) > 0 && !planEnded(stateOf(o)) {
			rf.Details = roundDetails(pending)
		}
		if _, err := saveRequest(ctx, o, rf); err != nil {
			return o, err
		}
		return loadIncrement(ctx, cwm, o.ID)
	}
	if planEnded(stateOf(o)) {
		if _, err := New().File(ctx, api.FileInput{On: o.ID, Kind: api.KindDemand, Details: details, By: by}); err != nil {
			return o, err
		}
		if o, err = loadIncrement(ctx, cwm, o.ID); err != nil {
			return o, err
		}
		nf := fieldsOf(o)
		nf.Delta, nf.Applied = delta, applied
		if _, err := saveRequest(ctx, o, nf); err != nil {
			return o, err
		}
		return loadIncrement(ctx, cwm, o.ID)
	}
	// The proposal still open is planned again: its round keeps the goals an
	// earlier round still waits to reconcile, beside this call's.
	rf.Delta, rf.Applied, rf.Details = delta, applied, roundDetails(roundGoals(rf.Details, renamed, f.Delivery, changed))
	if _, err := saveRequest(ctx, o, rf); err != nil {
		return o, err
	}
	if _, err := NewWork().Plan(ctx, api.PlanWorkInput{Request: o.ID, Again: true, NoIntake: true, By: by}); err != nil {
		return o, fmt.Errorf("plan the proposal of %s again for its changed goals: %w", o.ID, err)
	}
	return loadIncrement(ctx, cwm, o.ID)
}

// goalDiff is what a change of delivery goals moves, by their words: the
// goals gone (in before, not in after) and the goals come (in after, not in
// before).
func goalDiff(before, after []string) (gone, come []string) {
	for _, g := range before {
		if !slices.Contains(after, g) {
			gone = append(gone, g)
		}
	}
	for _, g := range after {
		if !slices.Contains(before, g) {
			come = append(come, g)
		}
	}
	return gone, come
}

// goalChanges aligns an increment's delivery goals before and after a change,
// by their words, never by position: a goal kept word for word is unchanged;
// a goal is reworded (renamed: old words to new) only when that is
// unambiguous - exactly one goal gone, exactly one come, and the gone one not
// dropped; otherwise every goal come is added and every goal gone removed.
// changed is what a proposal answers again: the reworded and the added, in
// their new words.
func goalChanges(before, after, drop []string) (renamed map[string]string, changed, removed []string) {
	renamed = map[string]string{}
	gone, come := goalDiff(before, after)
	if len(gone) == 1 && len(come) == 1 && !slices.Contains(drop, gone[0]) {
		renamed[gone[0]] = come[0]
		return renamed, come, nil
	}
	return renamed, come, gone
}

// dropGuard refuses a change of a run increment's goals that would remove a
// goal the caller did not name in drop (a list collapsed by mistake loses
// nothing silently): an unambiguous reword is no drop; each goal of drop must
// be a goal the change removes. It answers the goals removed.
func dropGuard(id string, before, after, drop []string) ([]string, error) {
	gone, come := goalDiff(before, after)
	for _, d := range drop {
		if !slices.Contains(gone, d) {
			return nil, invalid("SetGoals: %s: drop names %q, which is not a goal this change removes (its goals: %s)", id, d, quoteAll(before))
		}
	}
	_, _, removed := goalChanges(before, after, drop)
	var silent []string
	for _, g := range removed {
		if !slices.Contains(drop, g) {
			silent = append(silent, g)
		}
	}
	if len(silent) > 0 {
		return nil, invalid("SetGoals: %s: the new list would drop %d goal(s) not named in drop: %s - keep them in goals, or name each in drop to remove it (a goal counts as reworded only when exactly one goes and one comes; here %d go and %d come)",
			id, len(silent), quoteAll(silent), len(gone), len(come))
	}
	return removed, nil
}

// quoteAll quotes each goal, joined for a message.
func quoteAll(goals []string) string {
	q := make([]string, len(goals))
	for i, g := range goals {
		q[i] = fmt.Sprintf("%q", g)
	}
	return strings.Join(q, ", ")
}

// regoal follows the goals' change on a delta: each operation's `for` takes
// its goal's new words; an operation not yet applied for a goal proposed
// again (or removed) gives way, the applied ones staying as what was made.
func regoal(d []api.DeltaOp, applied []api.Applied, renamed map[string]string, again []string) ([]api.DeltaOp, []api.Applied) {
	done := map[int]bool{}
	for _, x := range applied {
		done[x.Op] = true
	}
	var out []api.DeltaOp
	moved := map[int]int{}
	for i, op := range d {
		if to, ok := renamed[op.For]; ok {
			op.For = to
		}
		if !done[i] && slices.Contains(again, op.For) {
			continue
		}
		moved[i] = len(out)
		out = append(out, op)
	}
	var keptApplied []api.Applied
	for _, x := range applied {
		if n, ok := moved[x.Op]; ok {
			keptApplied = append(keptApplied, api.Applied{Op: n, ID: x.ID})
		}
	}
	return out, keptApplied
}

// roundHead opens the details of a proposal planned again for changed goals.
const roundHead = "The delivery goals changed. Reconcile only these, each operation naming its goal (for):\n"

// roundDetails tells a proposal planned again what it answers: the goals
// added or reworded, alone - nothing else is listed; the operations of the
// others are kept, approved.
func roundDetails(changed []string) string {
	var b strings.Builder
	b.WriteString(roundHead)
	for _, g := range changed {
		b.WriteString("- " + g + "\n")
	}
	b.WriteString("\nThe operations already in the delta for the other goals are kept as approved: do not repeat them. The person approves only what this round adds.")
	return b.String()
}

// roundGoals is what a proposal still open is planned again for
// : the goals its current round still waits to
// reconcile - those its details list, in their new words when reworded, while
// still a delivery goal - then the goals this change adds or rewords. Planning
// again archives the earlier round's to-dos, so a goal it listed and never
// reconciled is listed again, never lost.
func roundGoals(details string, renamed map[string]string, delivery, changed []string) []string {
	var out []string
	if rest, ok := strings.CutPrefix(details, roundHead); ok {
		for _, line := range strings.Split(rest, "\n") {
			g, ok := strings.CutPrefix(line, "- ")
			if !ok {
				break
			}
			if to, ok := renamed[g]; ok {
				g = to
			}
			if slices.Contains(delivery, g) && !slices.Contains(out, g) {
				out = append(out, g)
			}
		}
	}
	for _, g := range changed {
		if !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	return out
}

// SetGoals gives an increment its delivery goals: kept while it has not run;
// once run, its proposal follows them, and a goal missing from the new list is
// removed only when the call names it in Drop (refused otherwise, naming what
// would drop); the answer names the goals removed.
func (i *Increments) SetGoals(ctx context.Context, in api.SetGoalsInput) (api.IncrementOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.IncrementOutput{}, err
	}
	if len(cleanGoals(in.Goals)) == 0 {
		return api.IncrementOutput{}, invalid("SetGoals: at least one delivery goal")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if st := releaseState(o); !open(st) {
		return api.IncrementOutput{}, invalid("SetGoals: %s is %s", in.ID, st)
	}
	before, after, drop := releaseFields(o).Delivery, cleanGoals(in.Goals), cleanGoals(in.Drop)
	var dropped []string
	if hasRequest(o) {
		if dropped, err = dropGuard(in.ID, before, after, drop); err != nil {
			return api.IncrementOutput{}, err
		}
	} else {
		_, _, dropped = goalChanges(before, after, drop)
	}
	if o, err = setDelivery(ctx, cwm, o, in.Goals, drop, in.By); err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, o), Dropped: dropped}, nil
}

// Run is the person's go for an increment (define now, plan when run): its
// proposal is filed - its analysis first, then the reconcile, the person's
// approval and the apply. Refused for an agent, with no delivery goals, for an
// ended increment, one on hold, and one already run.
func (i *Increments) Run(ctx context.Context, in api.IncrementRunInput) (api.IncrementOutput, error) {
	if in.By.Kind != "person" || in.By.ID == "" {
		return api.IncrementOutput{}, invalid("Run: a person runs an increment, got %+v", in.By)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if why := notRunnable(ctx, o); why != "" {
		return api.IncrementOutput{}, invalid("Run: %s %s", in.ID, why)
	}
	if _, err := New().File(ctx, api.FileInput{On: o.ID, Kind: api.KindDemand, By: in.By}); err != nil {
		return api.IncrementOutput{}, err
	}
	if o, err = loadIncrement(ctx, cwm, o.ID); err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, o)}, nil
}

// notRunnable says why an increment cannot be run; "" when it can.
func notRunnable(ctx context.Context, o cwmapi.Object) string {
	return whyNotRunnable(api.Increment{ID: o.ID, State: releaseState(o), Running: ranIncrement(o), Delivery: releaseFields(o).Delivery, Held: heldIncrement(ctx, o.ID)})
}

// whyNotRunnable is the one predicate of a run: the
// verb refuses on it and the pages offer « Run » exactly when it is "" - an
// open increment, not yet run, with delivery goals, not on hold.
func whyNotRunnable(inc api.Increment) string {
	switch {
	case !open(inc.State):
		return "is " + inc.State
	case inc.Running:
		return "is already running: its proposal is filed"
	case len(inc.Delivery) == 0:
		return "has no delivery goals: give it its goals first (increments set-goals)"
	case inc.Held != nil:
		return "is on hold: resume it first"
	}
	return ""
}

// Run is the person's go for a release: each increment shipping in it that can
// run is run; the others are said, with why.
func (r *Releases) Run(ctx context.Context, in api.ReleaseRunInput) (api.ReleaseRunOutput, error) {
	if in.By.Kind != "person" || in.By.ID == "" {
		return api.ReleaseRunOutput{}, invalid("Run: a person runs a release, got %+v", in.By)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseRunOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseRunOutput{}, err
	}
	if rel.Type != api.ReleaseType {
		return api.ReleaseRunOutput{}, invalid("Run: %s is an increment - run it with increments run", in.ID)
	}
	incs, err := incrementsIn(ctx, cwm, rel)
	if err != nil {
		return api.ReleaseRunOutput{}, err
	}
	out := api.ReleaseRunOutput{Ran: []string{}, Skipped: map[string]string{}}
	runner := NewIncrements()
	for _, o := range incs {
		if why := notRunnable(ctx, o); why != "" {
			out.Skipped[o.ID] = why
			continue
		}
		if _, err := runner.Run(ctx, api.IncrementRunInput{ID: o.ID, By: in.By}); err != nil {
			out.Skipped[o.ID] = err.Error()
			continue
		}
		out.Ran = append(out.Ran, o.ID)
	}
	return out, nil
}

// mergeApplied is what each operation became: the earlier entries, then the
// report's, which win for an operation both name.
func mergeApplied(prior, report []api.Applied) []api.Applied {
	named := map[int]bool{}
	for _, x := range report {
		named[x.Op] = true
	}
	var out []api.Applied
	for _, x := range prior {
		if !named[x.Op] {
			out = append(out, x)
		}
	}
	return append(out, report...)
}

// keepApplied records what each operation of a demand's delta became, beside
// what an earlier round applied.
func keepApplied(ctx context.Context, cwm cwmapi.CWMService, request string, a []api.Applied) error {
	o, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	f := fieldsOf(o)
	f.Applied = mergeApplied(f.Applied, a)
	_, err = saveRequest(ctx, o, f)
	return err
}

// deliverIncrement follows a demand's applied delta: each goal or feature an
// operation made or changed is aimed at the operation's increment - for an
// increment's own proposal, the goals and features it added at the increment
// itself - then each increment aimed at has its work planned by its release,
// or by itself when it ships in none.
func deliverIncrement(ctx context.Context, cwm cwmapi.CWMService, request string) error {
	o, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	f := fieldsOf(o)
	incs := NewIncrements()
	var aimed []string
	if o.Type == api.IncrementType {
		aimed = append(aimed, o.ID)
	}
	for _, x := range f.Applied {
		if x.Op < 0 || x.Op >= len(f.Delta) {
			continue
		}
		op := f.Delta[x.Op]
		inc := resolveRef(op.Increment, f.Applied)
		if inc == "" && o.Type == api.IncrementType && (op.Op == api.OpGoalAdd || op.Op == api.OpFeatureAdd) {
			inc = o.ID
		}
		if inc == "" || !aimsOf(op) {
			continue
		}
		if _, err := incs.aim(ctx, api.IncrementTargetInput{ID: x.ID, Increment: inc}); err != nil {
			return err
		}
		if !slices.Contains(aimed, inc) {
			aimed = append(aimed, inc)
		}
	}
	owner := ""
	if o.Type == api.IncrementType {
		owner = o.ID
	}
	orderScreenSwitch(ctx, cwm, owner, f.Delta, f.Applied)
	for _, id := range aimed {
		planIncrementWork(ctx, cwm, incs, id)
	}
	return nil
}

// planIncrementWork has the work of an increment that ran planned - its own
// only: the other increments of its release are planned when they run (define
// now, plan when run).
func planIncrementWork(ctx context.Context, cwm cwmapi.CWMService, incs *Increments, id string) {
	if _, _, err := loadAny(ctx, cwm, id); err != nil {
		slog.Warn("planification: an increment aimed at is not there", "increment", id, "error", err)
		return
	}
	if _, err := incs.PlanWork(ctx, api.IncrementPlanWorkInput{ID: id, By: PlanningActor}); err != nil {
		slog.Warn("planification: an increment did not plan its work", "increment", id, "error", err)
	}
}

// AmendDelta amends one operation of a demand's delta before it is applied:
// a person's act (an agent's session only relaying it, confirmed), the fields
// given replacing the operation's own, the whole delta checked again.
func (s *Service) AmendDelta(ctx context.Context, in api.AmendDeltaInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	if in.By.Kind != "person" {
		return api.RequestOutput{}, invalid("AmendDelta: a person amends a proposal - not %s %s", in.By.Kind, in.By.ID)
	}
	if strings.TrimSpace(in.Why) == "" {
		return api.RequestOutput{}, invalid("AmendDelta: says why (why)")
	}
	if _, err := personsAct(ctx, in.By, fmt.Sprintf("amend-delta:%s#%d", in.ID, in.Op), "AmendDelta: a person amends a proposal themselves (the request view, or their own command)"); err != nil {
		return api.RequestOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	o, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	f := fieldsOf(o)
	if in.Op < 0 || in.Op >= len(f.Delta) {
		return api.RequestOutput{}, invalid("AmendDelta: %s has no operation %d (0 to %d)", in.ID, in.Op, len(f.Delta)-1)
	}
	for _, x := range f.Applied {
		if x.Op == in.Op {
			return api.RequestOutput{}, invalid("AmendDelta: operation %d of %s is already applied (%s): amend what it made instead", in.Op, in.ID, x.ID)
		}
	}
	d := slices.Clone(f.Delta)
	op := &d[in.Op]
	for _, set := range []struct {
		to *string
		v  string
	}{
		{&op.Title, in.Title}, {&op.Details, in.Details}, {&op.Under, in.Under}, {&op.Module, in.Module},
		{&op.Object, in.Object}, {&op.Increment, in.Increment}, {&op.For, in.For},
	} {
		if strings.TrimSpace(set.v) != "" {
			*set.to = strings.TrimSpace(set.v)
		}
	}
	op.Why = strings.TrimSpace(op.Why + " - amended by " + in.By.ID + ": " + in.Why)
	if err := checkDelta(ctx, cwm, d); err != nil {
		return api.RequestOutput{}, err
	}
	f.Delta = d
	if o, err = saveRequest(ctx, o, f); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(o)}, nil
}
