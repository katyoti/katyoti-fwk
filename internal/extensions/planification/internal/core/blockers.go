// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// WHAT HOLDS EACH PIECE OF WORK - the one reader. For
// every open piece of work of a scope (an increment, a release, the
// workspace) it reads the stored state once - the objects and their plans, the
// to-dos and their waits (the structured one of a blocked report first), the
// scheduler's queue and modes, the Inbox entries asked in - and sorts each
// into one kind (api.BlockerKinds, K1-K6) with who moves it and since when; a
// piece that waits on other work names that work and the chain down to what
// holds it at the root. It writes nothing: `planning check` prints it.

// uncollectedAfter is how long a reported to-do may wait for its owner's
// collect before the check calls it an incident.
const uncollectedAfter = 10 * time.Minute

// unclassifiedWait begins the reason of a wait that names no kind.
const unclassifiedWait = "an unclassified wait: "

// maxChain bounds how deep the reader follows a wait into the work it waits on.
const maxChain = 32

// holdReader reads the stored state once for a check.
type holdReader struct {
	cwm   cwmapi.CWMService
	lab   *labelling
	scope string // the scope's label (increment:<id>, release:<id>); "" for the census
	// scopeIncs are the increments of the scope (an increment's dependency on
	// one of them stays inside it).
	scopeIncs map[string]bool
	open      map[string][]api.WorkTodo // the open to-dos of each request
	byID      map[string]api.WorkTodo
	queue     map[string]schedapi.QueueEntry
	modes     map[string]string // by the reference Modes names each module by
	refs      *projectRefs      // a to-do's module reference, to read its mode
	inbox     inboxapi.InboxService
	now       time.Time
	firsts    map[string]api.Blocker // each request's first blocker, once
	// follow follows a wait beyond the scope to its root; roots is, for
	// each blocker that waits on other work (rootKey), the blocker at the
	// root of its chain.
	follow bool
	roots  map[string]api.Blocker
	// works are the pieces of work of the scope (the measures keep to them).
	works map[string]bool
	// unnamedAfter is the deadline of a block with no named cause
	// (reconciler.unclassified_hours).
	unnamedAfter time.Duration
	// batches are the open GUI batch requests, read once when first asked.
	batches     []cwmapi.Object
	batchesRead bool
	// parts are each split request's parts (split-from), read once when
	// first asked.
	parts     map[string][]cwmapi.Object
	partsRead bool
	// followUps are each request's follow-ups, read with the parts.
	followUps map[string][]cwmapi.Object
	// endWaits are the requests whose refused end was read for the work it
	// waits on.
	endWaits map[string]bool
	// takersSince is when the scheduler's pull record started (a ready
	// to-do's live takers are known since then).
	takersSince string
}

// guiBatches is the open GUI batch requests, read once.
func (r *holdReader) guiBatches(ctx context.Context) []cwmapi.Object {
	if !r.batchesRead {
		r.batches, r.batchesRead = guiBatchRequests(ctx, r.cwm), true
	}
	return r.batches
}

// rootKey names a blocker: its work and its to-do.
func rootKey(b api.Blocker) string { return b.Work + "|" + b.Todo }

// Check answers what holds each open piece of work of the scope, sorted K1
// first, and writes nothing.
func (t *Planning) Check(ctx context.Context, in api.PlanningCheckInput) (api.PlanningCheckOutput, error) {
	n := 0
	for _, set := range []bool{in.Increment != "", in.Release != "", in.Census} {
		if set {
			n++
		}
	}
	if n != 1 {
		return api.PlanningCheckOutput{}, invalid("name one scope: --increment, --release or --census")
	}
	ctx = withReadCache(ctx)
	out, r, err := readScope(ctx, in, false)
	if err != nil || !in.Measure {
		return out, err
	}
	m, err := r.measures(ctx, in, out)
	if err != nil {
		return out, err
	}
	out.Measures = &m
	return out, nil
}

// readScope reads what holds each open piece of work of one scope, sorted K1
// first. follow follows a wait beyond the scope down to its root (the
// increment's blockers) instead of stopping there as outside.
func readScope(ctx context.Context, in api.PlanningCheckInput, follow bool) (api.PlanningCheckOutput, *holdReader, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PlanningCheckOutput{}, nil, err
	}
	r, err := newHoldReader(ctx, cwm)
	if err != nil {
		return api.PlanningCheckOutput{}, nil, err
	}
	r.follow = follow
	var works []cwmapi.Object
	seen := map[string]bool{}
	add := func(o cwmapi.Object) {
		if !seen[o.ID] && hasRequest(o) {
			seen[o.ID] = true
			r.works[o.ID] = true
			works = append(works, o)
		}
	}
	var wants []cwmapi.Object
	var twice []api.Blocker
	addIncrement := func(inc cwmapi.Object) {
		r.scopeIncs[inc.ID] = true
		for _, w := range incrementWork(ctx, cwm, inc) {
			if b, ok := twoTakers(ctx, cwm, w, seen); ok {
				twice = append(twice, b)
			}
			if hasRequest(w) {
				add(w)
			} else if w.Type != api.IncrementType {
				wants = append(wants, w)
			}
		}
	}
	out := api.PlanningCheckOutput{Counts: map[string]int{}}
	switch {
	case in.Increment != "":
		inc, err := loadTyped(ctx, cwm, in.Increment, api.IncrementType)
		if err != nil {
			return api.PlanningCheckOutput{}, nil, err
		}
		r.scope, out.Scope = "increment:"+inc.ID, inc.ID
		addIncrement(inc)
	case in.Release != "":
		rel, err := loadRelease(ctx, cwm, in.Release)
		if err != nil {
			return api.PlanningCheckOutput{}, nil, err
		}
		r.scope, out.Scope = "release:"+rel.ID, rel.ID
		incs, err := incrementsIn(ctx, cwm, rel)
		if err != nil {
			return api.PlanningCheckOutput{}, nil, err
		}
		for _, inc := range incs {
			addIncrement(inc)
		}
	default:
		out.Scope = "census"
		all, err := listRequests(ctx, cwm, "")
		if err != nil {
			return api.PlanningCheckOutput{}, nil, err
		}
		for _, o := range all {
			add(o)
		}
	}
	// What carries the scope's label is the scope's too: a follow-up, a
	// dependency a release loop works first.
	var labelled []string
	for req, todos := range r.open {
		for _, td := range todos {
			if r.scope == "" || slices.Contains(td.Labels, r.scope) {
				labelled = append(labelled, req)
				break
			}
		}
	}
	sort.Strings(labelled)
	for _, req := range labelled {
		if !seen[req] {
			if o, err := loadObject(ctx, cwm, req); err == nil {
				add(o)
			}
		}
	}
	for _, o := range works {
		out.Blockers = append(out.Blockers, r.work(ctx, o)...)
	}
	for _, w := range wants {
		if b, ok := r.orphanWant(ctx, w); ok {
			out.Blockers = append(out.Blockers, b)
		}
	}
	out.Blockers = append(out.Blockers, twice...)
	r.nameCycles(out.Blockers)
	for i := range out.Blockers {
		out.Blockers[i].Class = api.BlockerClass(out.Blockers[i].Kind)
	}
	sort.SliceStable(out.Blockers, func(i, j int) bool {
		a, b := out.Blockers[i], out.Blockers[j]
		if a.Class != b.Class {
			return a.Class < b.Class
		}
		if a.Since != b.Since {
			return a.Since < b.Since // the oldest first
		}
		return a.Work < b.Work
	})
	for _, b := range out.Blockers {
		out.Counts[b.Kind]++
	}
	return out, r, nil
}

// twoTakers is the incident of module work taken by more than one live
// increment or release - written behind the service, which keeps one
// -, once per object.
func twoTakers(ctx context.Context, cwm cwmapi.CWMService, w cwmapi.Object, seen map[string]bool) (api.Blocker, bool) {
	if ownsTarget(w) || releaseLike(w) || seen["takers|"+w.ID] {
		return api.Blocker{}, false
	}
	seen["takers|"+w.ID] = true
	live := liveTakers(ctx, cwm, w.ID)
	if len(live) < 2 {
		return api.Blocker{}, false
	}
	return api.Blocker{Work: w.ID, Title: w.Name, Kind: api.BlockerIncident, Cause: api.CauseTakenTwice,
		Reason: "taken by " + strings.Join(live, " and ") + ": an object has one open increment or release - aim it at one (increments target), the other gives it up",
		Actor:  "the person: aim it at one", ActorKind: api.ActorPerson}, true
}

func newHoldReader(ctx context.Context, cwm cwmapi.CWMService) (*holdReader, error) {
	r := &holdReader{cwm: cwm, lab: newLabelling(), scopeIncs: map[string]bool{}, open: map[string][]api.WorkTodo{},
		byID: map[string]api.WorkTodo{}, queue: map[string]schedapi.QueueEntry{}, modes: map[string]string{},
		now: time.Now().UTC(), firsts: map[string]api.Blocker{}, roots: map[string]api.Blocker{}, works: map[string]bool{},
		unnamedAfter: planificationConfig(ctx).Reconciler.UnclassifiedDeadline()}
	all, err := todosOf(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, t := range all {
		r.open[t.Request] = append(r.open[t.Request], t)
		r.byID[t.ID] = t
	}
	q, err := queueOf(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, e := range q.Queue {
		r.queue[e.Todo.ID] = e
	}
	r.takersSince = q.TakersSince
	if s, err := sched(ctx); err == nil {
		if m, err := s.Modes(ctx, schedapi.ModesInput{}); err == nil {
			var several bool
			r.modes, several = modesByRef(m.Modules)
			r.refs = newProjectRefs(ctx, several)
		}
	}
	if ib, err := application.UseService[inboxapi.InboxService](ctx); err == nil {
		r.inbox = ib
	}
	return r, nil
}

// work is what holds one request: one blocker per to-do at its front - the
// later steps waiting on its own earlier ones are not repeated - or one read
// on the object alone when it has no open to-do; the inconsistencies between
// its plan, its lifecycle and its to-dos are incidents of their own.
func (r *holdReader) work(ctx context.Context, o cwmapi.Object) []api.Blocker {
	ps := planState(o)
	if planEnded(ps) || ps == api.PlanSplit {
		return nil
	}
	base := api.Blocker{Work: o.ID, Title: o.Name}
	var out []api.Blocker
	todos := r.open[o.ID]
	if lifeState(o) == api.StateDone && ps == api.PlanBlocked && len(todos) == 0 {
		if batches := guiBatchWait(o, r.guiBatches(ctx)); len(batches) > 0 {
			// Done, its plan finishes once its GUI batch has verified its
			// walks: a wait on that batch, not an incident.
			pf, _ := planOf(o)
			b := base
			b.Since = lastMove(pf)
			return []api.Blocker{r.dependency(ctx, b, guiBatchWaits+strings.Join(batches, ", "), batches[0], nil)}
		}
	}
	if lifeState(o) == api.StateDone && !onlyProving(todos) {
		b := base
		b.Kind, b.Reason = api.BlockerIncident, "its lifecycle is done while its plan is still "+ps
		b.Actor, b.ActorKind = "the reconciler: end its plan", api.ActorSystem
		out = append(out, b)
		if len(todos) == 0 && ps == api.PlanReady {
			// One inconsistency, said once: its plan has nothing to do
			// because its object is done - the reconciler's end-plan reads
			// it, never its put-step.
			return out
		}
	}
	if len(todos) == 0 || ps == api.PlanIdle && len(edgesTo(o, api.EdgeAskedIn)) > 0 && r.asking(ctx, o) {
		// A request that asked its requester waits on that answer, whatever
		// steps it kept.
		return append(out, r.unplanned(ctx, o, ps, base))
	}
	if ps == api.PlanBlocked {
		for _, t := range todos {
			if t.State == schedapi.StateReady && t.For == schedapi.ForAgent {
				b := base
				b.Kind, b.Todo, b.Step = api.BlockerIncident, t.ID, t.Packet.Step
				b.Reason = "its plan is blocked while its step " + t.Packet.Step + " is offered to agents"
				b.Actor, b.ActorKind = "the person: look at the plan's block", api.ActorPerson
				out = append(out, b)
				break
			}
		}
	}
	for _, t := range r.front(o.ID, todos) {
		out = append(out, r.todo(ctx, o, t, nil))
	}
	return out
}

// onlyProving says a request's open to-dos are all proof steps (phase
// verify: a verify, a review, an auto-test) - none builds. A lifecycle moved
// done ahead of them (a person accepted it before its proof ran) is no
// inconsistency: the plan ends by itself when they report (moveEnded leaves a
// node already there), and each to-do says what holds it. Build steps open
// on a done object are one: its plan waits on work it no longer needs.
func onlyProving(todos []api.WorkTodo) bool {
	for _, t := range todos {
		if t.Packet.Phase != api.PhaseVerify {
			return false
		}
	}
	return len(todos) > 0
}

// front is a request's to-dos that do not just wait on its own earlier steps.
func (r *holdReader) front(request string, todos []api.WorkTodo) []api.WorkTodo {
	var out []api.WorkTodo
	for _, t := range todos {
		if t.State == schedapi.StateWaiting && strings.HasPrefix(t.Waits, "after ") && len(t.After) > 0 {
			// An ended earlier step holds nothing: the step waits on its
			// own request when every open one it waits after is its own.
			own, open := true, false
			for _, a := range t.After {
				if p, ok := r.byID[a]; ok {
					open = true
					own = own && p.Request == request
				}
			}
			if own && open {
				continue
			}
		}
		out = append(out, t)
	}
	if len(out) == 0 && len(todos) > 0 {
		out = todos[:1] // a chain of its own steps with no head: show its first
	}
	return out
}

// unplanned reads a request with no open to-do from its plan.
func (r *holdReader) unplanned(ctx context.Context, o cwmapi.Object, ps string, b api.Blocker) api.Blocker {
	pf, _ := planOf(o)
	b.Since = lastMove(pf)
	switch ps {
	case api.PlanIdle:
		if asked := edgesTo(o, api.EdgeAskedIn); len(asked) > 0 {
			entry := asked[len(asked)-1]
			b.On = entry
			if st := r.entryStatus(ctx, entry); st != "" && st != inboxapi.StatusPending {
				b.Kind, b.Reason = api.BlockerIncident, "its question in "+entry+" was answered ("+st+") but the request did not resume"
				b.Actor, b.ActorKind = "the reconciler: collect the answer", api.ActorSystem
				return b
			}
			b.Kind, b.Reason = api.BlockerAnswer, "its question waits for an answer in "+entry
			b.Actor, b.ActorKind = r.requesterOf(o), api.ActorPerson
			return b
		}
		b.Kind, b.Act, b.Reason = api.BlockerAct, api.ActPlan, "nothing is planned: what next is someone's call (plan its work)"
		if pf.Note != "" {
			b.Reason += " - " + firstLine(pf.Note)
		}
		b.Actor, b.ActorKind = "the person: plan it", api.ActorPerson
	case api.PlanBlocked:
		// Its end refused while open work linked to it is not done: that
		// work holds it - read once per request, so
		// two ends waiting on each other are no endless read.
		if endRefused(pf) && !r.endWaits[o.ID] {
			if r.endWaits == nil {
				r.endWaits = map[string]bool{}
			}
			r.endWaits[o.ID] = true
			if dep := r.endWaitsOn(ctx, o); dep != "" {
				return r.dependency(ctx, b, endWaitsOnWork+dep, dep, nil)
			}
		}
		b.Kind, b.Act, b.Reason = api.BlockerAct, api.ActUnblock, "its plan is blocked"
		if pf.Note != "" {
			b.Reason += ": " + firstLine(pf.Note)
		}
		b.Actor, b.ActorKind = "the person: look at the block", api.ActorPerson
	default:
		b.Kind, b.Reason = api.BlockerIncident, "its plan is "+ps+" with no step to do"
		b.Actor, b.ActorKind = "the reconciler: put its planned step", api.ActorSystem
	}
	return b
}

// todo is what holds one open to-do. chain is the work already followed down
// to it (a cycle guard).
func (r *holdReader) todo(ctx context.Context, o cwmapi.Object, t api.WorkTodo, chain []string) api.Blocker {
	b := api.Blocker{Work: t.Request, Title: o.Name, Todo: t.ID, Step: t.Packet.Step, Since: t.Made}
	switch t.State {
	case schedapi.StateReported:
		if t.Result != nil {
			b.Since = t.Result.At
		}
		if at, err := time.Parse(time.RFC3339Nano, b.Since); err == nil && r.now.Sub(at) > uncollectedAfter {
			b.Kind, b.Reason = api.BlockerIncident, "its result was reported and never collected"
			b.Actor, b.ActorKind = "the reconciler: collect it again", api.ActorSystem
			return b
		}
		b.Kind, b.Reason = api.BlockerHeld, "its result is being collected"
		b.Actor, b.ActorKind = api.Owner, api.ActorSystem
		return b
	case schedapi.StateClaimed:
		if live(t, r.now) {
			b.Since = t.Claim.At
			b.Kind, b.Reason = api.BlockerHeld, "a worker holds it"
			b.Actor, b.ActorKind = t.Claim.By.ID, actorKind(t.Claim.By)
			return b
		}
		return r.ready(ctx, t, b, "its claim by "+t.Claim.By.ID+" lapsed at "+t.Claim.Until)
	case schedapi.StateReady:
		return r.ready(ctx, t, b, "")
	}
	return r.waiting(ctx, o, t, b, chain)
}

// ready is what holds a to-do the queue may offer: a person's step, a module
// off, or the queue's word on it - its structured wait read through
// queueWaitWords, the one mapping the pull's wait namer uses too
func (r *holdReader) ready(ctx context.Context, t api.WorkTodo, b api.Blocker, lapsed string) api.Blocker {
	if t.For == schedapi.ForPerson {
		if t.Packet.AskedIn != "" {
			b.On = t.Packet.AskedIn
			if st := r.entryStatus(ctx, t.Packet.AskedIn); st != "" && st != inboxapi.StatusPending {
				b.Kind, b.Reason = api.BlockerIncident, "its question in "+t.Packet.AskedIn+" was answered ("+st+") but its step did not resume"
				b.Actor, b.ActorKind = "the reconciler: report the step with the answer", api.ActorSystem
				return b
			}
		}
		return queueWaitWords(t, schedapi.QueueWait{Todo: t.ID, Cause: schedapi.WaitPerson}).on(b, "")
	}
	if offFor(r.modes, r.refs, t) {
		b.Kind, b.Reason = api.BlockerHeld, "module "+t.Module+" is off: no worker takes its to-dos"
		b.Actor, b.ActorKind = "the person: set module "+t.Module+"'s mode", api.ActorPerson
		return b
	}
	e, queued := r.queue[t.ID]
	switch {
	case !queued:
		b.Kind, b.Reason = api.BlockerIncident, "it is ready but the queue does not offer it"
		b.Actor, b.ActorKind = "the reconciler", api.ActorSystem
	case !e.Takeable:
		w := schedapi.QueueWait{Todo: t.ID}
		if e.Wait != nil {
			w = *e.Wait
		}
		b = queueWaitWords(t, w).on(b, e.Waits)
	default:
		if e.Only != nil {
			b = queueWaitWords(t, *e.Only).on(b, "")
			break
		}
		b.Kind, b.Reason = api.BlockerReady, "any agent may take it now"
		b.Actor, b.ActorKind = "an agent: a loop on "+r.loopOn(t), api.ActorAgent
		if tk, ok := livePull(e.Takers); ok {
			b.Actor, b.ActorKind = r.takerWords(tk, t), actorKind(tk.By)
		} else if r.takersSince != "" {
			b.Reason += " - no live taker since " + r.takersSince
		} else {
			b.Reason += " - no live taker"
		}
	}
	if lapsed != "" && b.Kind == api.BlockerReady {
		b.Reason = lapsed + ": " + b.Reason
	}
	return b
}

// livePull is the taker of a ready to-do the reader names
// : a pull waiting on it now first, then a session
// busy on another to-do, then the latest pull ended within the lease.
func livePull(takers []schedapi.Taker) (schedapi.Taker, bool) {
	if len(takers) == 0 {
		return schedapi.Taker{}, false
	}
	rank := func(tk schedapi.Taker) int {
		switch {
		case tk.Waiting:
			return 0
		case tk.Holds != "":
			return 1
		}
		return 2
	}
	best := takers[0]
	for _, tk := range takers[1:] {
		if r, b := rank(tk), rank(best); r < b || (r == b && r == 2 && tk.At > best.At) {
			best = tk
		}
	}
	return best, true
}

// takerWords names a live taker of a ready to-do: its worker and what it
// does - waiting on the to-do's scope, working the request its session
// holds, or pulled n minutes ago.
func (r *holdReader) takerWords(tk schedapi.Taker, t api.WorkTodo) string {
	switch {
	case tk.Waiting:
		return tk.By.ID + " (waiting on " + r.loopOn(t) + ")"
	case tk.Holds != "":
		on := tk.Holds
		if h, ok := r.byID[tk.Holds]; ok && h.Request != "" {
			on = h.Request
		}
		return tk.By.ID + " (working " + on + ")"
	}
	if at, err := time.Parse(time.RFC3339, tk.At); err == nil {
		return fmt.Sprintf("%s (pulled %d min ago)", tk.By.ID, int(r.now.Sub(at).Minutes()))
	}
	return tk.By.ID + " (pulled lately)"
}

// loopOn is the scope a loop takes a ready to-do in: the check's scope when
// it carries its label, else its own increment, else its module.
func (r *holdReader) loopOn(t api.WorkTodo) string {
	if r.scope != "" && slices.Contains(t.Labels, r.scope) {
		return r.scope
	}
	for _, l := range t.Labels {
		if strings.HasPrefix(l, "increment:") {
			return l
		}
	}
	return "module:" + t.Module
}

// waiting is what holds a to-do that waits, from its wait: the structured
// wait of a blocked report, else the reason settle wrote.
func (r *holdReader) waiting(ctx context.Context, o cwmapi.Object, t api.WorkTodo, b api.Blocker, chain []string) api.Blocker {
	w := t.Waits
	stop, _ := stopOf(t)
	switch {
	case stop == schedapi.StatusFail:
		if t.Result != nil {
			b.Since = t.Result.At
		}
		if failedToAgent(t) {
			// An agent's failure is agent work held: an agent retries it
			// ; past the cap it is the person's.
			b.Kind, b.Reason = api.BlockerHeld, firstLine(w)
			b.Actor, b.ActorKind = "an agent: work retry "+t.ID, api.ActorAgent
			return b
		}
		b.Kind, b.Reason = api.BlockerAct, firstLine(w)
		b.Actor, b.ActorKind = "the person: retry it or drop it", api.ActorPerson
		return b
	case stop == schedapi.StatusBlocked:
		return r.blocked(ctx, t, b, chain)
	case strings.HasPrefix(w, "after "):
		for _, a := range t.After {
			p, ok := r.byID[a]
			if !ok {
				continue
			}
			if p.Request == t.Request {
				// An earlier step of its own request: ordinary sequencing,
				// held by what holds that step. A cycle is a step met
				// again down its own chain, never its request.
				if slices.Contains(chain, p.ID) {
					b.On = p.ID
					b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseCycle, waitCycle+strings.Join(append(append(slices.Clone(chain), t.ID), p.ID), " → ")
					b.Actor, b.ActorKind = "the person: break one of its waits", api.ActorPerson
					return b
				}
				return r.onWork(ctx, b, w, p.Request, chain, func(d cwmapi.Object) api.Blocker { return r.todo(ctx, d, p, append(chain, t.ID)) })
			}
			return r.onWork(ctx, b, w, p.Request, chain, func(d cwmapi.Object) api.Blocker { return r.todo(ctx, d, p, append(chain, t.Request)) })
		}
		b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseStepClosed, w+", which is no longer open"
		b.Actor, b.ActorKind = "the reconciler: settle it again", api.ActorSystem
		return b
	case strings.HasPrefix(w, "held by the review of "):
		return r.dependency(ctx, b, w, strings.TrimPrefix(w, "held by the review of "), chain)
	case strings.HasPrefix(w, waitsOnDependency):
		return r.dependency(ctx, b, w, strings.TrimPrefix(w, waitsOnDependency), chain)
	case strings.HasPrefix(w, guiBatchWaits):
		batch, _, _ := strings.Cut(strings.TrimPrefix(w, guiBatchWaits), ",")
		return r.dependency(ctx, b, w, strings.TrimSpace(batch), chain)
	case strings.HasPrefix(w, blockedByHandWaits):
		// Its request blocked by hand holds it: the person's block, lifted
		// by the person.
		b.Kind, b.Act, b.Reason = api.BlockerAct, api.ActUnblock, "its plan is blocked: "+firstLine(strings.TrimPrefix(w, blockedByHandWaits))
		b.Actor, b.ActorKind = "the person: look at the block", api.ActorPerson
		return b
	case strings.HasPrefix(w, "its increment ") && strings.Contains(w, " is on hold"):
		b.Kind, b.Reason = api.BlockerAct, w
		b.Actor, b.ActorKind = "the person: resume the increment", api.ActorPerson
		return b
	}
	if _, dep, ok := incrementWait(w); ok {
		return r.dependency(ctx, b, w, dep, chain)
	}
	b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseUnclassified, unclassifiedWait+orDefault(w, "it waits, saying nothing")
	b.Actor, b.ActorKind = "the reconciler: settle it again", api.ActorSystem
	return b
}

// blocked reads a blocked report's structured wait: what kind of thing holds
// it. A report that named none is an unclassified wait, owned by who reported
// it.
func (r *holdReader) blocked(ctx context.Context, t api.WorkTodo, b api.Blocker, chain []string) api.Blocker {
	h := t.Held
	if t.Result != nil {
		b.Since = t.Result.At
	}
	if batches := verifyBatchWait(t, r.guiBatches(ctx)); len(batches) > 0 {
		// A verify that could run all but its GUI walks: a wait on the
		// batch that runs them.
		got := r.dependency(ctx, b, guiBatchWaits+strings.Join(batches, ", "), batches[0], chain)
		if h != nil && h.Since != "" {
			got.Since = h.Since
		}
		return got
	}
	if h == nil || h.Kind == "" {
		_, summary := stopOf(t)
		b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseUnclassified, unclassifiedWait+firstLine(summary)
		b.Actor, b.ActorKind = "who reported it", api.ActorAgent
		if h != nil {
			b.Since, b.Actor = h.Since, h.By.ID
		} else if t.Result != nil {
			b.Actor = t.Result.By.ID
		}
		b.Due = r.unnamedDue(t, b.Since)
		return b
	}
	b.Since, b.On, b.Reason = h.Since, h.Target, h.Reason
	switch h.Kind {
	case schedapi.HeldPerson:
		b.Kind, b.Actor, b.ActorKind = api.BlockerAct, orDefault(h.Target, "the person"), api.ActorPerson
	case schedapi.HeldResource, schedapi.HeldExternal:
		b.Kind, b.Actor, b.ActorKind = api.BlockerOutside, orDefault(h.Target, "an outside party"), api.ActorOutside
	case schedapi.HeldObject:
		got := r.dependency(ctx, b, h.Reason, h.Target, chain)
		got.Since = h.Since
		return got
	default:
		b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseUnknownKind, "a wait of an unknown kind "+h.Kind+": "+h.Reason
		b.Actor, b.ActorKind = h.By.ID, api.ActorAgent
	}
	return b
}

// dependency is a wait on another object: held while that work moves inside
// the scope, outside when it lies beyond it, an incident when nothing will
// move it.
func (r *holdReader) dependency(ctx context.Context, b api.Blocker, why, dep string, chain []string) api.Blocker {
	return r.onWork(ctx, b, why, dep, chain, nil)
}

// onWork classifies a wait on dep. first, when set, is what holds the very
// to-do waited on (a previous step); otherwise dep's own first blocker.
func (r *holdReader) onWork(ctx context.Context, b api.Blocker, why, dep string, chain []string, first func(cwmapi.Object) api.Blocker) api.Blocker {
	b.On, b.Reason = dep, why
	// An earlier step of its own request is followed step by step (the
	// caller guards that chain): its request met again is no cycle.
	ownStep := first != nil && dep == b.Work
	if !ownStep && (slices.Contains(chain, dep) || dep == b.Work) {
		b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseCycle, waitCycle+strings.Join(append(append(slices.Clone(chain), b.Work), dep), " → ")
		b.Actor, b.ActorKind = "the person: break one of its waits", api.ActorPerson
		return b
	}
	d, _, err := loadAny(ctx, r.cwm, dep)
	if err != nil {
		b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseUnreadable, why+" - "+dep+waitUnreadable
		b.Actor, b.ActorKind = "the person: remove the wait", api.ActorPerson
		return b
	}
	if first == nil && waitEnded(d) {
		// Superseded: the work goes on in its last successor, which holds
		// the wait while it is open.
		if succ, sd, ok := successorOf(ctx, r.cwm, d); ok && !waitEnded(sd) && !slices.Contains(chain, succ) && succ != b.Work {
			dep, d = succ, sd
			b.On = dep
		}
	}
	if first == nil && waitEnded(d) {
		b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseWaitNotCleared, why+" - "+dep+" has ended, the wait was not cleared"
		b.Actor, b.ActorKind = "the reconciler: clear the wait", api.ActorSystem
		return b
	}
	if !ownStep && !r.follow && !r.inScope(ctx, d) {
		b.Kind, b.ActorKind = api.BlockerOutside, api.ActorOutside
		b.Actor = r.whereIs(ctx, d)
		b.Chain = []string{dep}
		return b
	}
	var root api.Blocker
	switch {
	case first != nil:
		root = first(d)
	case d.Type == api.IncrementType:
		// An increment waited on is held by what it holds: its first piece
		// of open work.
		// The increment joins the chain: a wait that comes back to it is
		// a cycle, never a walk without end.
		root = r.incrementFirst(ctx, d, append(chain, b.Work, d.ID))
	default:
		if !hasRequest(d) || len(r.open[dep]) == 0 && planState(d) == api.PlanReady {
			b.Kind, b.Cause, b.Reason = api.BlockerIncident, api.CauseNobodyWorks, why+waitNobody+dep+" (no open plan)"
			b.Actor, b.ActorKind = "the person: plan "+dep, api.ActorPerson
			return b
		}
		root = r.first(ctx, d, append(chain, b.Work))
	}
	deep := root
	if got, ok := r.roots[rootKey(root)]; ok {
		deep = got
	}
	r.roots[rootKey(b)] = deep
	if root.Kind == api.BlockerIncident && root.Cause == api.CauseCycle {
		b.Kind, b.Cause, b.Reason, b.Actor, b.ActorKind = root.Kind, root.Cause, root.Reason, root.Actor, root.ActorKind
		return b
	}
	b.Kind = api.BlockerHeld
	b.Actor, b.ActorKind = root.Actor, root.ActorKind
	if root.Kind != api.BlockerHeld && root.Kind != api.BlockerReady {
		// What moves it is what moves its root: the person's, an outside
		// party's, an incident's.
		b.Reason = why + " - at its root: " + root.Reason
	}
	b.Chain = append([]string{dep}, root.Chain...)
	if root.On != "" && !slices.Contains(b.Chain, root.On) && root.Kind != api.BlockerAnswer {
		b.Chain = append(b.Chain, root.On)
	}
	if root.Kind == api.BlockerAnswer || root.Kind == api.BlockerAct || root.Kind == api.BlockerOutside || root.Kind == api.BlockerIncident {
		b.Kind = root.Kind
	}
	return b
}

// first is a request's first blocker, read once.
func (r *holdReader) first(ctx context.Context, d cwmapi.Object, chain []string) api.Blocker {
	if got, ok := r.firsts[d.ID]; ok {
		return got
	}
	var got api.Blocker
	if todos := r.open[d.ID]; len(todos) > 0 {
		got = r.todo(ctx, d, r.front(d.ID, todos)[0], chain)
	} else if planState(d) == api.PlanSplit {
		got = r.splitFirst(ctx, d, chain)
	} else {
		got = r.unplanned(ctx, d, planState(d), api.Blocker{Work: d.ID, Title: d.Name})
	}
	r.firsts[d.ID] = got
	return got
}

// splitFirst is what holds a split request waited on: it has no step of its
// own - its parts carry its work, then its follow-ups, and it ends once they
// have (endSplits) - so the wait is on its first open part, else its first
// open follow-up. None open is an incident: nothing is left to end it
func (r *holdReader) splitFirst(ctx context.Context, d cwmapi.Object, chain []string) api.Blocker {
	b := api.Blocker{Work: d.ID, Title: d.Name}
	pf, _ := planOf(d)
	b.Since = lastMove(pf)
	for _, p := range r.partsOf(ctx, d.ID) {
		if !planEnded(planState(p)) {
			return r.dependency(ctx, b, splitCarries+"part "+p.ID+splitCarriesEnd, p.ID, chain)
		}
	}
	if open := openFollowUps(ctx, r.cwm, d.ID); len(open) > 0 {
		return r.dependency(ctx, b, splitCarries+"follow-up "+open[0]+splitCarriesEnd, open[0], chain)
	}
	b.Kind, b.Reason = api.BlockerIncident, "its plan is split and none of its parts or follow-ups is open"
	b.Actor, b.ActorKind = "the person: end it", api.ActorPerson
	return b
}

// The reason a wait on a split request reads: its part, or follow-up, X
// carries its work.
const (
	splitCarries    = "its plan is split: its "
	splitCarriesEnd = " carries its work"
)

// partsOf is a split request's parts, in id order, read once for all.
func (r *holdReader) partsOf(ctx context.Context, id string) []cwmapi.Object {
	r.readLinks(ctx)
	return r.parts[id]
}

// followUpsOf is a request's follow-ups, in id order, read once for all.
func (r *holdReader) followUpsOf(ctx context.Context, id string) []cwmapi.Object {
	r.readLinks(ctx)
	return r.followUps[id]
}

// readLinks reads once the parts and the follow-ups of every request.
func (r *holdReader) readLinks(ctx context.Context) {
	if r.partsRead {
		return
	}
	r.parts, r.followUps, r.partsRead = map[string][]cwmapi.Object{}, map[string][]cwmapi.Object{}, true
	got, err := r.cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: api.CapPlan})
	if err != nil {
		return
	}
	sort.Slice(got.Objects, func(i, j int) bool { return got.Objects[i].ID < got.Objects[j].ID })
	for _, o := range got.Objects {
		if up := o.EdgeTo(api.EdgeSplitFrom); up != "" {
			r.parts[up] = append(r.parts[up], o)
		}
		if up := o.EdgeTo(api.EdgeFollowsUp); up != "" {
			r.followUps[up] = append(r.followUps[up], o)
		}
	}
}

// incrementFirst is the first open piece of an increment's work.
func (r *holdReader) incrementFirst(ctx context.Context, inc cwmapi.Object, chain []string) api.Blocker {
	for _, w := range incrementWork(ctx, r.cwm, inc) {
		if hasRequest(w) && !planEnded(planState(w)) && planState(w) != api.PlanSplit {
			got := r.first(ctx, w, chain)
			got.Chain = append([]string{w.ID}, got.Chain...)
			return got
		}
	}
	return api.Blocker{Work: inc.ID, Kind: api.BlockerAct, Act: api.ActAccept, Reason: "its work is done: it waits to be accepted", Actor: "the person: accept " + inc.ID, ActorKind: api.ActorPerson}
}

// inScope says whether work moves inside the check's scope: it carries the
// scope's label, or is one of the scope's increments; for the census,
// everything is.
func (r *holdReader) inScope(ctx context.Context, d cwmapi.Object) bool {
	if r.scope == "" {
		return true // the census's scope is the whole workspace
	}
	if d.Type == api.IncrementType {
		return r.scopeIncs[d.ID]
	}
	l := r.lab.of(ctx, r.cwm, d.ID)
	if slices.Contains(l.labels, r.scope) || r.scopeIncs[l.inc] {
		return true
	}
	for _, t := range r.open[d.ID] {
		if slices.Contains(t.Labels, r.scope) {
			return true
		}
	}
	return false
}

// whereIs names where work outside the scope is scheduled: its increment and
// release, or none.
func (r *holdReader) whereIs(ctx context.Context, d cwmapi.Object) string {
	if d.Type == api.IncrementType {
		if rel := edgeTo(d, api.EdgeShipsIn); rel != "" {
			return d.ID + ", in " + rel
		}
		return d.ID + ", in no release"
	}
	l := r.lab.of(ctx, r.cwm, d.ID)
	if len(l.labels) == 0 {
		return d.ID + ", in no increment or release"
	}
	return d.ID + " (" + strings.Join(l.labels, ", ") + ")"
}

// orphanWant is a want of the scope that no open request works on.
func (r *holdReader) orphanWant(ctx context.Context, w cwmapi.Object) (api.Blocker, bool) {
	if nodeEnded(w) || lifeState(w) == api.StateDone || isDone(ctx, r.cwm, w) {
		return api.Blocker{}, false
	}
	serving, _ := incoming(ctx, r.cwm, w.ID, api.EdgeServes)
	for _, s := range serving {
		if so, err := loadObject(ctx, r.cwm, s); err == nil && hasRequest(so) && !planEnded(planState(so)) {
			return api.Blocker{}, false
		}
	}
	if pf, ok := planOf(w); ok && !planEnded(pf.State) {
		return api.Blocker{}, false
	}
	return api.Blocker{Work: w.ID, Title: w.Name, Kind: api.BlockerIncident, Cause: api.CauseNoWork, Reason: "no work planned: nothing open works on it",
		Actor: "the person: plan its work", ActorKind: api.ActorPerson}, true
}

// asking says whether a request's last question is still pending, or
// answered and not taken up - what it waits on.
func (r *holdReader) asking(ctx context.Context, o cwmapi.Object) bool {
	asked := edgesTo(o, api.EdgeAskedIn)
	if len(asked) == 0 {
		return false
	}
	return r.entryStatus(ctx, asked[len(asked)-1]) != ""
}

// stepTitle is a to-do's title without its step's prefix.
func stepTitle(t api.WorkTodo) string {
	return strings.TrimPrefix(t.Title, t.Packet.Step+" — ")
}

// entryStatus is an Inbox entry's status; "" when it cannot be read.
func (r *holdReader) entryStatus(ctx context.Context, entry string) string {
	if r.inbox == nil {
		return ""
	}
	got, err := r.inbox.Get(ctx, inboxapi.GetInput{ID: entry})
	if err != nil {
		return ""
	}
	return got.Entry.Status
}

// requesterOf names who a request's question is asked of.
func (r *holdReader) requesterOf(o cwmapi.Object) string {
	if f := fieldsOf(o); f.Requester.ID != "" {
		return f.Requester.ID
	}
	return "the person"
}

// live reports a claim whose lease has not passed.
func live(t api.WorkTodo, now time.Time) bool {
	if t.State != schedapi.StateClaimed || t.Claim == nil {
		return false
	}
	until, err := time.Parse(time.RFC3339Nano, t.Claim.Until)
	return err != nil || now.Before(until)
}

func actorKind(a cwmapi.Actor) string {
	if a.Kind == api.ActorPerson {
		return api.ActorPerson
	}
	return api.ActorAgent
}

// lastMove is when a plan last moved, from its trail.
func lastMove(pf api.PlanFields) string {
	if n := len(pf.Trail); n > 0 {
		return pf.Trail[n-1].At
	}
	return ""
}

func isDoneIncrement(o cwmapi.Object) bool {
	return o.Type == api.IncrementType && (releaseState(o) == api.IncrementDone || releaseState(o) == api.IncrementDropped)
}

// waitEnded says a piece of work waited on has ended - the one rule every
// writer and reader of a wait uses: an increment by
// its own state, done or dropped (its proposal's plan ending is not its end);
// any other by its plan, done or dropped, or its lifecycle done.
func waitEnded(o cwmapi.Object) bool {
	if o.Type == api.IncrementType {
		return isDoneIncrement(o)
	}
	return nodeEnded(o) || lifeState(o) == api.StateDone
}

// incrementWait reads the wait settle writes for an increment's dependency
// (labelling.holdOf): « its increment <inc> waits on <dep> ».
func incrementWait(w string) (inc, dep string, ok bool) {
	rest, found := strings.CutPrefix(w, "its increment ")
	if !found {
		return "", "", false
	}
	inc, dep, found = strings.Cut(rest, " waits on ")
	inc, dep = strings.TrimSpace(inc), strings.TrimSpace(dep)
	if !found || inc == "" || dep == "" || strings.Contains(inc, " ") {
		return "", "", false
	}
	return inc, dep, true
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// stopOf reads a to-do its own report stopped: the report's status (fail or
// blocked) and its summary, from the report kept on it while it waits; "" for
// a to-do no report stopped. It is the one reader of a stopped step: every
// view asks it, never the wait's text (RQ-064).
func stopOf(t api.WorkTodo) (status, summary string) {
	if t.State != schedapi.StateWaiting || t.Result == nil {
		return "", ""
	}
	switch t.Result.Status {
	case schedapi.StatusFail, schedapi.StatusBlocked:
		return t.Result.Status, firstLine(t.Result.Summary)
	}
	return "", ""
}

// clearStop ends a stopped to-do's stop: its waits line and the report kept
// on it go together - the one way a stop ends (retried, its wait cleared,
// waiting on the steps its report added), so a to-do back in the queue never
// reads as stopped.
func clearStop(t *api.WorkTodo) {
	t.Waits, t.Result = "", nil
}

// stopped says a to-do's own report stopped it (failed or blocked).
func stopped(t api.WorkTodo) bool {
	s, _ := stopOf(t)
	return s != ""
}

// onThePerson says whether the one reader puts a stopped to-do on the
// person: the person's answer or act (K1, K2), or an
// incident (K6) - what will not move by itself - at the root of its own wait.
// A to-do that waits on other work (a chain) is moved where that work's root
// is, shown there; an outside party's or an agent's hold is never the person's.
func (r *holdReader) onThePerson(ctx context.Context, d cwmapi.Object, t api.WorkTodo) bool {
	b := r.todo(ctx, d, t, nil)
	if !PersonMoves(b) {
		return false
	}
	// A wait the reconciler put to the person as a prepared decision is
	// that decision's card, not an unblock.
	return b.Kind != api.BlockerIncident || waitDecision(ctx, r.cwm, d.ID) == ""
}

// PersonMoves says whether the one reader puts a blocker on the person: the
// person's answer or act, or an incident, at the root of its own wait - and
// not a block with no named cause before its deadline (Due), which stays
// with who reported it until the reconciler retries it.
func PersonMoves(b api.Blocker) bool {
	if len(b.Chain) > 0 || b.Due != "" {
		return false
	}
	return b.ActorKind == api.ActorPerson || b.Kind == api.BlockerIncident
}

// unnamedDue is the deadline of a step blocked with no named cause since
// since, while it is still ahead and the step was never retried for it; ""
// otherwise - the block is then the person's.
func (r *holdReader) unnamedDue(t api.WorkTodo, since string) string {
	if t.Packet.UnnamedRetries > 0 {
		return ""
	}
	at, err := time.Parse(time.RFC3339Nano, since)
	if err != nil {
		return ""
	}
	due := at.Add(r.unnamedAfter)
	if !r.now.Before(due) {
		return ""
	}
	return due.UTC().Format(time.RFC3339)
}

// The reasons the one reader gives a wait no rule resolves (the reconciler's
// decide-wait reads them).
const (
	waitCycle      = "a cycle: "
	waitUnreadable = " cannot be read"
	waitNobody     = " - nobody works on "
)

// noRuleResolves says a blocker is a wait no repair resolves, by its cause:
// a cycle, an unreadable object, a dependency nobody works on.
func noRuleResolves(b api.Blocker) bool {
	return b.Kind == api.BlockerIncident && len(b.Chain) == 0 &&
		(b.Cause == api.CauseCycle || b.Cause == api.CauseUnreadable || b.Cause == api.CauseNobodyWorks)
}

// successorOf follows a superseded object's superseded-by edges to its last
// readable successor (the chain bounded, a loop stopped).
func successorOf(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) (string, cwmapi.Object, bool) {
	seen := map[string]bool{o.ID: true}
	cur, found := o, false
	for i := 0; i < maxChain; i++ {
		next := edgeTo(cur, api.EdgeSupersededBy)
		if next == "" || seen[next] {
			break
		}
		seen[next] = true
		n, _, err := loadAny(ctx, cwm, next)
		if err != nil {
			break
		}
		cur, found = n, true
	}
	return cur.ID, cur, found
}

// waitDecision is the open decision the reconciler raised about a request
// for a wait no rule resolves; "" when none is open.
func waitDecision(ctx context.Context, cwm cwmapi.CWMService, request string) string {
	return reconcilerDecision(ctx, cwm, request, true)
}

// anyWaitDecision is a decision the reconciler raised about a request, open
// or settled: it raises one only (once decided, the step's own card comes
// back for the person to act on the choice).
func anyWaitDecision(ctx context.Context, cwm cwmapi.CWMService, request string) string {
	return reconcilerDecision(ctx, cwm, request, false)
}

func reconcilerDecision(ctx context.Context, cwm cwmapi.CWMService, request string, open bool) string {
	about, err := incoming(ctx, cwm, request, edgeAbout)
	if err != nil {
		return ""
	}
	for _, id := range about {
		o, err := loadObject(ctx, cwm, id)
		if err != nil || o.Type != decisionsapi.DecisionType {
			continue
		}
		raised, _, _ := cwmapi.DecodeCapability[struct {
			Resolved string `json:"resolved"`
			RaisedBy string `json:"raised_by"`
		}](o, decisionsapi.CapDecision)
		if open && (raised.Resolved != "" || orDefault(ownState(o), decisionsapi.StatusOpen) != decisionsapi.StatusOpen) {
			continue
		}
		if raised.RaisedBy == api.ReconcilerActorID {
			return id
		}
	}
	return ""
}

// stopStatus is a stopped to-do's status alone (fail, blocked or "").
func stopStatus(t api.WorkTodo) string {
	s, _ := stopOf(t)
	return s
}

// stopVerb words a stopped to-do's report for the person: failed or is
// blocked, and its summary.
func stopVerb(t api.WorkTodo) (verb, summary string) {
	stop, summary := stopOf(t)
	if stop == schedapi.StatusBlocked {
		return "is blocked", summary
	}
	return "failed", summary
}

// THE VIEWS' WORDS FOR A HOLD - the increment views'
// open-request lines, a request's where-in-work, the demands' reasons, a
// blocked request's line in the increment's waits and the deck's cards read
// what holds a request from the one reader, through one wording, never from a
// to-do's wait text: a view and planning check name one hold alike.

// holdWords is the one wording of a hold: its K-number, its kind and who
// moves it - the head of a pull's waiting line (waitKind.label) - then why.
func holdWords(b api.Blocker) string {
	head := api.BlockerClass(b.Kind) + " " + b.Kind + " by " + b.Actor
	if b.Reason == "" {
		return head
	}
	return head + ": " + firstLine(b.Reason)
}

// viewReader runs read with the one reader of a view: read once for the
// view's read cache - never one per request, which a release page or an
// increments list would make quadratic - and one at a time, since it
// memoizes as it reads. Without a read cache it reads for this call alone.
// False when the stored state could not be read.
func viewReader(ctx context.Context, read func(r *holdReader)) bool {
	c, cached := ctx.Value(readCacheKey{}).(*readCache)
	if !cached {
		cwm, err := cwmOf(ctx)
		if err != nil {
			return false
		}
		r, err := newHoldReader(ctx, cwm)
		if err != nil {
			return false
		}
		read(r)
		return true
	}
	c.holdMu.Lock()
	defer c.holdMu.Unlock()
	if !c.holdBuilt {
		c.holdBuilt = true
		if cwm, err := cwmOf(ctx); err == nil {
			c.hold, _ = newHoldReader(ctx, cwm)
		}
	}
	if c.hold == nil {
		return false
	}
	read(c.hold)
	return true
}

// holdOf is what holds a request as planning check names it: the first of
// its blockers, the lowest K first; false when nothing holds it (its plan
// ended) or the state could not be read.
func holdOf(ctx context.Context, o cwmapi.Object) (b api.Blocker, ok bool) {
	viewReader(ctx, func(r *holdReader) {
		for _, x := range r.work(ctx, o) {
			if !ok || api.BlockerClass(x.Kind) < api.BlockerClass(b.Kind) {
				b, ok = x, true
			}
		}
	})
	return b, ok
}

// stepHold reads what holds one to-do of a request through the one reader,
// and whether the reader puts it on the person; ok false when nothing was
// read. nil reads nothing (a stand read alone).
type stepHold func(t api.WorkTodo) (b api.Blocker, onPerson, ok bool)

// viewStepHold is the stepHold of request id (its title) in a view.
func viewStepHold(ctx context.Context, id, title string) stepHold {
	d := cwmapi.Object{ID: id, Name: title}
	return func(t api.WorkTodo) (b api.Blocker, onPerson, ok bool) {
		ok = viewReader(ctx, func(r *holdReader) {
			b = r.todo(ctx, d, t, nil)
			onPerson = r.onThePerson(ctx, d, t)
		})
		return b, onPerson, ok
	}
}
