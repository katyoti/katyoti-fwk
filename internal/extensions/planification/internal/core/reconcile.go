// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE RECONCILER - one deterministic sweep over the
// stored state, in the order of api.Repairs, each element in id order. A
// finding is a repair when it is safe - it undoes no act, re-plans no started
// work, starts no session, calls no LLM - and an incident otherwise. Dry-run
// first: without apply, or for a repair not switched on, the sweep only says
// what it would do. Applied, a repair reads its element again and goes on only
// if it is as observed (its revision for an object, its stored state for a
// to-do). Above flapLimit repairs of one element it stops repairing it and
// raises « flapping »; above putLimit to-dos put in a day it switches itself
// to dry-run. The incidents of the one reader (planning check, K6) that no
// repair covers are raised too, each once.

// ReconcilerActor records the reconciler's repairs.
var ReconcilerActor = cwmapi.Actor{ID: api.ReconcilerActorID, Kind: "agent"}

const (
	// flapLimit is how many times one element may be repaired: the next
	// finding of it is « flapping ».
	flapLimit = 3
	// putLimit is how many to-dos the repairs may put in a day before the
	// reconciler switches itself to dry-run.
	putLimit = 20
)

// reconState is what the reconciler keeps between its sweeps, per store: the
// repairs applied to each element, the to-dos put today, whether it switched
// itself to dry-run. It lives in the lease holder's memory; the incidents'
// list keeps all of it across a restart (F-105): a
// state first used reads it back (restore), every sweep writes it (keep).
type reconState struct {
	mu      sync.Mutex
	applied map[string]int // check:element → repairs applied
	day     string
	puts    int
	stopped string // why it switched itself to dry-run
	loaded  bool   // applied was read back from the incidents' list
}

var (
	reconStatesMu sync.Mutex
	reconStates   = map[cwmapi.CWMService]*reconState{}
)

// reconStateOf is the reconciler's state for the store a CWM service holds.
func reconStateOf(cwm cwmapi.CWMService) *reconState {
	reconStatesMu.Lock()
	defer reconStatesMu.Unlock()
	s, ok := reconStates[cwm]
	if !ok {
		s = &reconState{applied: map[string]int{}}
		reconStates[cwm] = s
	}
	return s
}

// restore reads back what the incidents' list kept of a previous process: the
// repairs applied to each element, and - the same day only - the to-dos put
// and why it switched itself to dry-run. A count kept higher than the one in
// memory wins. The caller holds st.mu.
func (st *reconState) restore(doc incidentsDoc, today string) {
	for k, n := range doc.Attempts {
		if n > st.applied[k] {
			st.applied[k] = n
		}
	}
	if doc.Day == today && (st.day == "" || st.day == today) {
		st.day = today
		if doc.Puts > st.puts {
			st.puts = doc.Puts
		}
		if st.stopped == "" {
			st.stopped = doc.Stopped
		}
	}
	st.loaded = true
}

// keep writes what a restart must not lose into the incidents' list. The
// caller holds st.mu.
func (st *reconState) keep(doc *incidentsDoc) {
	doc.Attempts = map[string]int{}
	for k, n := range st.applied {
		doc.Attempts[k] = n
	}
	doc.Day, doc.Puts, doc.Stopped = st.day, st.puts, st.stopped
}

// newDay starts the day's count afresh when the day changed, and lifts a
// dry-run the day's limit switched on.
func (st *reconState) newDay(today string) {
	if st.day != today {
		st.day, st.puts = today, 0
		if strings.HasPrefix(st.stopped, "the day's limit") {
			st.stopped = ""
		}
	}
}

// finding is one thing the sweep found on an element.
type finding struct {
	repair api.Repair
	// key names the element (repair:element); still says whether it is as
	// the sweep read it.
	key   string
	still func(ctx context.Context) bool
	// apply does the repair; it answers how many to-dos it put.
	apply func(ctx context.Context) (int, error)
}

// reconSweep reads the stored state once for a sweep.
type reconSweep struct {
	cwm   cwmapi.CWMService
	inbox inboxapi.InboxService
	open  map[string][]api.WorkTodo
	lab   *labelling
	// covered are the elements (work|to-do) a finding is about: the
	// reader's incidents on them are not raised again.
	covered   map[string]bool
	findings  []finding
	incidents []api.Incident
	// batches are the open GUI batch requests, read once when first asked.
	batches     []cwmapi.Object
	batchesRead bool
}

// guiBatches is the open GUI batch requests, read once a sweep.
func (s *reconSweep) guiBatches(ctx context.Context) []cwmapi.Object {
	if !s.batchesRead {
		s.batches, s.batchesRead = guiBatchRequests(ctx, s.cwm), true
	}
	return s.batches
}

// Reconcile runs one sweep of the reconciler.
func (t *Planning) Reconcile(ctx context.Context, in api.ReconcileInput) (api.ReconcileOutput, error) {
	for _, r := range in.Repairs {
		if !slices.Contains(api.Repairs, r) {
			return api.ReconcileOutput{}, invalid("Reconcile: no repair %q (one of %s)", r, strings.Join(api.Repairs, ", "))
		}
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReconcileOutput{}, err
	}
	start := time.Now()
	cfg := planificationConfig(ctx).Reconciler
	st := reconStateOf(cwm)
	st.mu.Lock()
	defer st.mu.Unlock()
	today := time.Now().UTC().Format("2006-01-02")

	// The incidents' list keeps the repairs' attempts and the day's puts
	// across a restart: a state first used reads them back
	// (F-105) - before the day's count is checked, so a
	// count kept from another day starts afresh.
	list := incidentsPath(ctx, cwm)
	if !st.loaded && list != "" {
		if doc, err := readIncidents(list); err == nil {
			st.restore(doc, today)
		} else {
			slog.Warn("planification: the incidents' list cannot be read", "path", list, "error", err)
		}
	}
	st.newDay(today)

	s, err := newReconSweep(ctx, cwm)
	if err != nil {
		return api.ReconcileOutput{}, err
	}
	if err := s.find(ctx); err != nil {
		return api.ReconcileOutput{}, err
	}
	if err := s.readerIncidents(ctx); err != nil {
		return api.ReconcileOutput{}, err
	}

	out := st.settle(ctx, in, cfg, s.findings, s.incidents)
	sort.SliceStable(out.Incidents, func(i, j int) bool { return out.Incidents[i].Key < out.Incidents[j].Key })
	out.Incidents = dedupIncidents(out.Incidents)
	if list != "" {
		err := editIncidents(list, func(doc *incidentsDoc) {
			doc.Incidents = foldIncidents(doc.Incidents, out.Incidents, time.Now())
			doc.Repaired = foldRepairs(doc.Repaired, out.Repairs, time.Now())
			st.keep(doc)
			out.Open = openIncidents(doc.Incidents)
		})
		if err != nil {
			// The sweep stands; its list is folded again at the next one.
			slog.Warn("planification: the incidents' list cannot be written", "path", list, "error", err)
		}
	}
	out.TookMS = time.Since(start).Milliseconds()
	return out, nil
}

// settle decides each finding of a sweep - skipped (not wanted, flapping,
// dry-run, not switched on, stopped, or moved since the sweep read it) or
// applied - and answers the sweep's output. The caller holds st.mu.
func (st *reconState) settle(ctx context.Context, in api.ReconcileInput, cfg api.ReconcilerConfig, findings []finding, incidents []api.Incident) api.ReconcileOutput {
	out := api.ReconcileOutput{Repairs: []api.Repair{}, Incidents: incidents}
	switch {
	case !in.Apply:
		out.DryRun, out.Why = true, "dry-run: nothing is applied without apply"
	case cfg.Off:
		out.DryRun, out.Why = true, "the reconciler's kill switch is on (Planification.yaml, reconciler.off)"
	case st.stopped != "":
		out.DryRun, out.Why = true, st.stopped
	}
	for _, f := range findings {
		r := f.repair
		wanted := len(in.Repairs) == 0 || slices.Contains(in.Repairs, r.Repair)
		switch {
		case !wanted:
			continue
		case st.applied[f.key] >= flapLimit:
			r.Skipped = "flapping: repaired " + fmt.Sprint(st.applied[f.key]) + " times already"
			out.Incidents = append(out.Incidents, api.Incident{Key: api.IncidentFlapping + ":" + f.key, Check: api.IncidentFlapping,
				Work: r.Work, Todo: r.Todo, Reason: "the reconciler repaired it " + fmt.Sprint(st.applied[f.key]) + " times and it came back: " + r.Reason,
				Actor: "the person: find what undoes the repair"})
		case out.DryRun:
			r.Skipped = "dry-run"
		case !cfg.On(r.Repair):
			r.Skipped = "dry-run: the repair " + r.Repair + " is not switched on (Planification.yaml, reconciler.repairs)"
		case st.stopped != "":
			r.Skipped = st.stopped
		case !f.still(ctx):
			r.Skipped = "it moved since the sweep read it"
		default:
			puts, err := f.apply(ctx)
			st.puts += puts
			if err != nil {
				r.Skipped = "failed: " + err.Error()
				break
			}
			r.Applied = true
			st.applied[f.key]++
			if st.puts > putLimit {
				st.stopped = fmt.Sprintf("the day's limit of put to-dos is passed (%d > %d): the reconciler is back to dry-run until tomorrow", st.puts, putLimit)
			}
		}
		out.Repairs = append(out.Repairs, r)
	}
	return out
}

func newReconSweep(ctx context.Context, cwm cwmapi.CWMService) (*reconSweep, error) {
	s := &reconSweep{cwm: cwm, open: map[string][]api.WorkTodo{}, lab: newLabelling(), covered: map[string]bool{}}
	all, err := todosOf(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, t := range all {
		s.open[t.Request] = append(s.open[t.Request], t)
	}
	if ib, err := application.UseService[inboxapi.InboxService](ctx); err == nil {
		s.inbox = ib
	}
	return s, nil
}

// find runs every check over every request, in id order.
func (s *reconSweep) find(ctx context.Context) error {
	reqs, err := listRequests(ctx, s.cwm, "")
	if err != nil {
		return err
	}
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].ID < reqs[j].ID })
	for _, o := range reqs {
		if !hasRequest(o) {
			continue
		}
		s.endPlan(ctx, o)
		s.putStep(ctx, o)
		s.applyAnswer(ctx, o)
		s.repointWait(ctx, o)
		s.clearWait(ctx, o)
		s.retryUnnamed(ctx, o)
		s.labels(ctx, o)
	}
	return nil
}

// add records a finding on an element.
func (s *reconSweep) add(f finding) {
	if f.key == "" {
		f.key = f.repair.Repair + ":" + orDefault(f.repair.Todo, f.repair.Work)
	}
	s.covered[f.repair.Work+"|"+f.repair.Todo] = true
	s.findings = append(s.findings, f)
}

// incident records what the sweep cannot repair safely.
func (s *reconSweep) incident(check, work, todo, cause, reason, actor string) {
	s.covered[work+"|"+todo] = true
	s.incidents = append(s.incidents, api.Incident{Key: check + ":" + orDefault(todo, work), Check: check, Work: work, Todo: todo, Cause: cause, Reason: reason, Actor: actor})
}

// revision is an object's stored revision; "" when it cannot be read.
func (s *reconSweep) revision(ctx context.Context, id string) string {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return ""
	}
	got, err := s.cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return ""
	}
	return got.Revision
}

// sameObject says an object is still at the revision the sweep read.
func (s *reconSweep) sameObject(id string, rev string) func(context.Context) bool {
	return func(ctx context.Context) bool { return rev != "" && s.revision(ctx, id) == rev }
}

// todoPrint is a to-do's stored state, to compare.
func todoPrint(t api.WorkTodo) string {
	raw, _ := json.Marshal(t)
	return string(raw)
}

// sameTodo says a to-do is still as the sweep read it.
func sameTodo(t api.WorkTodo) func(context.Context) bool {
	before := todoPrint(t)
	return func(ctx context.Context) bool {
		got, err := getTodo(ctx, t.ID)
		return err == nil && got.Archived == "" && todoPrint(got) == before
	}
}

// both says two observations still hold.
func both(a, b func(context.Context) bool) func(context.Context) bool {
	return func(ctx context.Context) bool { return a(ctx) && b(ctx) }
}

// entryStatus is an Inbox entry's status; "" when it cannot be read.
func (s *reconSweep) entryStatus(ctx context.Context, entry string) string {
	if s.inbox == nil || entry == "" {
		return ""
	}
	got, err := s.inbox.Get(ctx, inboxapi.GetInput{ID: entry})
	if err != nil {
		return ""
	}
	return got.Entry.Status
}

// endPlan: the object's lifecycle is done while its plan is open. Safe when
// the move is later than the plan's last step and nothing of it runs or waits
// on a person: the plan ends.
func (s *reconSweep) endPlan(ctx context.Context, o cwmapi.Object) {
	ps := planState(o)
	if ownState(o) != api.StateDone || planEnded(ps) || ps == api.PlanSplit {
		return
	}
	reason := "its lifecycle is done while its plan is still " + ps
	unsafe := func(why string) {
		s.incident(api.RepairEndPlan, o.ID, "", "", reason+" - "+why, "the person: end its plan or reopen it")
	}
	if n := len(s.open[o.ID]); n > 0 {
		if onlyProving(s.open[o.ID]) {
			return // its proof steps end the plan when they report
		}
		unsafe(fmt.Sprintf("%d of its to-dos are still open", n))
		return
	}
	pf, _ := planOf(o)
	switch ps {
	case api.PlanReady:
	case api.PlanIdle:
		if asked := edgesTo(o, api.EdgeAskedIn); len(asked) > 0 && s.entryStatus(ctx, asked[len(asked)-1]) == inboxapi.StatusPending {
			unsafe("its question waits for an answer")
			return
		}
		if open := openFollowUps(ctx, s.cwm, o.ID); len(open) > 0 {
			unsafe("it waits on its follow-ups " + strings.Join(open, ", "))
			return
		}
	case api.PlanBlocked:
		if !strings.HasPrefix(pf.Note, doneButNote) {
			unsafe("its plan was blocked for another reason: " + firstLine(pf.Note))
			return
		}
		if len(guiBatchWait(o, s.guiBatches(ctx))) > 0 {
			return // it waits on its GUI batch: the plan finishes once the walks are verified
		}
	default:
		unsafe("its plan is " + ps)
		return
	}
	moved := s.movedDoneAt(ctx, o.ID)
	last := lastPlanActivity(pf)
	if moved == "" || moved < last {
		unsafe("its move to done is not later than its plan's last step")
		return
	}
	rev := s.revision(ctx, o.ID)
	s.add(finding{
		repair: api.Repair{Repair: api.RepairEndPlan, Work: o.ID, Reason: reason, Action: "end its plan done"},
		still:  s.sameObject(o.ID, rev),
		apply: func(ctx context.Context) (int, error) {
			_, err := NewPlans().Finish(ctx, api.FinishInput{ID: o.ID, By: ReconcilerActor, Note: "the reconciler: its lifecycle was done at " + moved + ", after its plan's last step"})
			return 0, err
		},
	})
}

// movedDoneAt is when an object last moved to done, from its journal; "" when
// no journal says.
func (s *reconSweep) movedDoneAt(ctx context.Context, id string) string {
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil || journal == nil {
		return ""
	}
	got, err := journal.Transitions(ctx, journalapi.TransitionsInput{ID: id})
	if err != nil {
		return ""
	}
	for i := len(got.Entries) - 1; i >= 0; i-- {
		if got.Entries[i].To == api.StateDone {
			return normTime(got.Entries[i].At)
		}
	}
	return ""
}

// lastPlanActivity is the latest moment a plan moved or a step of it ended.
func lastPlanActivity(pf api.PlanFields) string {
	last := ""
	for _, e := range pf.Trail {
		if at := normTime(e.At); at > last {
			last = at
		}
	}
	for _, st := range pf.Steps {
		if at := normTime(st.EndedAt); at > last {
			last = at
		}
	}
	return last
}

// normTime writes an RFC 3339 time in one form, to the second (the journal
// and the plan may not keep the same precision), so two compare as strings.
func normTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339)
}

// putStep: a plan ready with no to-do. Safe when none of its steps ran - its
// template is planned - or when its last step ran and carried the move its
// end makes - its end is tried again. Otherwise no step exists to put.
func (s *reconSweep) putStep(ctx context.Context, o cwmapi.Object) {
	if planState(o) != api.PlanReady || ownState(o) == api.StateDone || len(s.open[o.ID]) > 0 {
		return
	}
	reason := "its plan is ready with no step to do"
	done, err := doneOf(ctx, o.ID)
	if err != nil {
		return
	}
	ran := false
	for _, t := range done {
		if t.Result != nil {
			ran = true // a removed one never ran
		}
	}
	rev := s.revision(ctx, o.ID)
	if !ran {
		s.add(finding{
			repair: api.Repair{Repair: api.RepairPutStep, Work: o.ID, Reason: reason + ", and none of its steps ran", Action: "plan its template's steps"},
			still:  both(s.sameObject(o.ID, rev), s.noOpenTodo(o.ID)),
			apply: func(ctx context.Context) (int, error) {
				got, err := NewWork().Plan(ctx, api.PlanWorkInput{Request: o.ID, By: ReconcilerActor})
				return len(got.Todos), err
			},
		})
		return
	}
	moves := false
	for _, t := range done {
		if t.Packet.Moves != "" && t.Result != nil && t.Result.Status == schedapi.StatusSuccess {
			moves = true
		}
	}
	if !moves && !movesNothing(ctx, o.ID, done) {
		s.incident(api.RepairPutStep, o.ID, "", "", reason+": its steps ran and none is left to put", "the person: plan its next step or end it")
		return
	}
	s.add(finding{
		repair: api.Repair{Repair: api.RepairPutStep, Work: o.ID, Reason: reason + ", its last step ran", Action: "end it as its last step's collect would have"},
		still:  both(s.sameObject(o.ID, rev), s.noOpenTodo(o.ID)),
		apply: func(ctx context.Context) (int, error) {
			return 0, endRequest(ctx, s.cwm, o.ID, ReconcilerActor)
		},
	})
}

// noOpenTodo says a request still has no open to-do.
func (s *reconSweep) noOpenTodo(request string) func(context.Context) bool {
	return func(ctx context.Context) bool {
		got, err := todosOf(ctx, request)
		return err == nil && len(got) == 0
	}
}

// applyAnswer: a question answered in the Inbox that its asking step - or its
// idle request - did not take up. The answer is applied as its updated event
// would have.
func (s *reconSweep) applyAnswer(ctx context.Context, o cwmapi.Object) {
	if s.inbox == nil {
		return
	}
	for _, t := range s.open[o.ID] {
		if t.For != schedapi.ForPerson || t.State != schedapi.StateReady || t.Packet.AskedIn == "" {
			continue
		}
		if st := s.entryStatus(ctx, t.Packet.AskedIn); st != "" && st != inboxapi.StatusPending {
			entry := t.Packet.AskedIn
			s.add(finding{
				repair: api.Repair{Repair: api.RepairApplyAnswer, Work: o.ID, Todo: t.ID,
					Reason: "its question in " + entry + " was answered (" + st + ") but its step did not resume", Action: "report the step with the answer"},
				still: sameTodo(t),
				apply: func(ctx context.Context) (int, error) {
					_, err := New().Answered(ctx, entry)
					return 0, err
				},
			})
		}
	}
	if planState(o) != api.PlanIdle {
		return
	}
	asked := edgesTo(o, api.EdgeAskedIn)
	if len(asked) == 0 {
		return
	}
	entry := asked[len(asked)-1]
	if st := s.entryStatus(ctx, entry); st != "" && st != inboxapi.StatusPending {
		rev := s.revision(ctx, o.ID)
		s.add(finding{
			repair: api.Repair{Repair: api.RepairApplyAnswer, Work: o.ID,
				Reason: "its question in " + entry + " was answered (" + st + ") but the request did not resume", Action: "resume the request with the answer"},
			still: s.sameObject(o.ID, rev),
			apply: func(ctx context.Context) (int, error) {
				_, err := New().Answered(ctx, entry)
				return 0, err
			},
		})
	}
}

// waitTarget is the object a to-do's wait names, and whether it is one the
// sweep reads: a dependency, a review, a blocked report's object.
func waitTarget(t api.WorkTodo) (string, bool) {
	for _, p := range []string{waitsOnDependency, "held by the review of "} {
		if dep, ok := strings.CutPrefix(t.Waits, p); ok {
			return strings.TrimSpace(dep), false
		}
	}
	if stopStatus(t) == schedapi.StatusBlocked && t.Held != nil && t.Held.Kind == schedapi.HeldObject && t.Held.Target != "" {
		return t.Held.Target, true
	}
	return "", false
}

// clearWait: a to-do not started waits on an object that has ended. A wait
// settle wrote is settled again; a blocked report's wait on the object is
// cleared, and its step settled.
func (s *reconSweep) clearWait(ctx context.Context, o cwmapi.Object) {
	for _, t := range s.open[o.ID] {
		if !notStarted(t) || t.State != schedapi.StateWaiting {
			continue
		}
		reason := ""
		dep, reported := waitTarget(t)
		if inc, on, ok := incrementWait(t.Waits); ok {
			// Its increment's dependency: gone once that increment no longer
			// depends on it, or once it has ended.
			io, _, err := loadAny(ctx, s.cwm, inc)
			if err != nil {
				s.incident(api.RepairClearWait, o.ID, t.ID, api.CauseUnreadable, "its increment "+inc+" cannot be read", "the person: remove the wait")
				continue
			}
			switch {
			case isDoneIncrement(io):
				// A step of an increment that has ended waits on nothing
				// its increment still holds.
				reason = "its increment " + inc + " has ended"
			case !slices.Contains(edgesTo(io, api.EdgeDependsOn), on):
				reason = "its increment " + inc + " no longer depends on " + on
			}
			dep = on
		}
		if dep == "" {
			continue
		}
		if reason == "" {
			d, _, err := loadAny(ctx, s.cwm, dep)
			if err != nil {
				s.decideWait(ctx, o, t, dep, "it waits on "+dep+", which"+waitUnreadable)
				continue
			}
			if !waitEnded(d) {
				continue
			}
			if _, sd, ok := successorOf(ctx, s.cwm, d); ok && !waitEnded(sd) {
				continue // superseded: repoint-wait moves the wait to its successor
			}
			reason = "it waits on " + dep + ", which has ended"
		}
		t := t
		s.add(finding{
			repair: api.Repair{Repair: api.RepairClearWait, Work: o.ID, Todo: t.ID,
				Reason: reason, Action: "clear the wait and settle the step again"},
			still: sameTodo(t),
			apply: func(ctx context.Context) (int, error) {
				if reported {
					clearStop(&t)
					if _, err := put(ctx, t); err != nil {
						return 0, err
					}
				}
				return 0, settleRequest(ctx, t.Request, ReconcilerActor)
			},
		})
	}
}

// repointWait: a to-do not started waits on a request that was superseded,
// whose last successor is still open - the work goes on there
// . A blocked report's wait is moved to the successor
// a depends-on edge left on the superseded request is re-pointed as
// requests supersede does. A successor that cannot be read is a wait no rule
// resolves: a prepared decision.
func (s *reconSweep) repointWait(ctx context.Context, o cwmapi.Object) {
	for _, t := range s.open[o.ID] {
		if !notStarted(t) || t.State != schedapi.StateWaiting {
			continue
		}
		dep, reported := waitTarget(t)
		if dep == "" {
			continue
		}
		d, _, err := loadAny(ctx, s.cwm, dep)
		if err != nil || !waitEnded(d) {
			continue
		}
		next := edgeTo(d, api.EdgeSupersededBy)
		if next == "" {
			continue
		}
		succ, sd, ok := successorOf(ctx, s.cwm, d)
		if !ok {
			s.decideWait(ctx, o, t, dep, "it waits on "+dep+", superseded by "+next+", which"+waitUnreadable)
			continue
		}
		if waitEnded(sd) {
			continue // ended too: clear-wait clears it
		}
		t := t
		rev := s.revision(ctx, o.ID)
		s.add(finding{
			repair: api.Repair{Repair: api.RepairRepointWait, Work: o.ID, Todo: t.ID,
				Reason: "it waits on " + dep + ", superseded by " + succ + ", which is open", Action: "wait on " + succ + " instead"},
			still: both(sameTodo(t), s.sameObject(o.ID, rev)),
			apply: func(ctx context.Context) (int, error) {
				if reported {
					h := *t.Held
					h.Target = succ
					h.Reason = strings.TrimSpace(h.Reason + " (" + dep + " superseded by " + succ + ")")
					t.Held = &h
					if _, err := put(ctx, t); err != nil {
						return 0, err
					}
					return 0, settleRequest(ctx, t.Request, ReconcilerActor)
				}
				return 0, repoint(ctx, s.cwm, o.ID, dep, succ)
			},
		})
	}
}

// retryUnnamed: a step blocked with no named cause past its deadline
// (reconciler.unclassified_hours), never retried for it, is sent back to the
// queue once; its retry is recorded on the step, so
// its next unnamed block is the person's at once.
func (s *reconSweep) retryUnnamed(ctx context.Context, o cwmapi.Object) {
	after := planificationConfig(ctx).Reconciler.UnclassifiedDeadline()
	now := time.Now()
	for _, t := range s.open[o.ID] {
		if stopStatus(t) != schedapi.StatusBlocked || (t.Held != nil && t.Held.Kind != "") || t.Packet.UnnamedRetries > 0 {
			continue
		}
		since := ""
		if t.Held != nil {
			since = t.Held.Since
		} else if t.Result != nil {
			since = t.Result.At
		}
		at, err := time.Parse(time.RFC3339Nano, since)
		if err != nil || now.Sub(at) < after {
			continue
		}
		if len(verifyBatchWait(t, s.guiBatches(ctx))) > 0 {
			continue // it waits on its GUI batch: retrying it runs nothing new
		}
		t := t
		s.add(finding{
			repair: api.Repair{Repair: api.RepairRetryUnnamed, Work: o.ID, Todo: t.ID,
				Reason: "it was blocked with no named cause since " + normTime(since) + ", past its deadline of " + after.String(),
				Action: "send it back to the queue once"},
			still: sameTodo(t),
			apply: func(ctx context.Context) (int, error) {
				clearStop(&t)
				t.Packet.UnnamedRetries++
				if _, err := put(ctx, t); err != nil {
					return 0, err
				}
				return 1, settleRequest(ctx, t.Request, ReconcilerActor)
			},
		})
	}
}

// decideWait: a wait no rule resolves - a cycle, an unreadable object, a
// dependency nobody works on, a superseded request with no readable
// successor - is put to the person as ONE prepared decision about its
// request: opened once, never a second once the
// reconciler raised one about it; while it is open the decision is the
// person's card in place of the unblock.
func (s *reconSweep) decideWait(ctx context.Context, o cwmapi.Object, t api.WorkTodo, dep, why string) {
	if anyWaitDecision(ctx, s.cwm, o.ID) != "" {
		s.covered[o.ID+"|"+t.ID] = true
		return
	}
	on := dep
	if dep == o.ID {
		on = "itself"
	}
	c := waitChoice(t.Packet.Step, on, why, strings.Contains(why, waitNobody), o.ID)
	s.add(finding{
		repair: api.Repair{Repair: api.RepairDecideWait, Work: o.ID, Todo: t.ID,
			Reason: "no rule ends its wait: " + why, Action: "open one prepared decision for the person"},
		still: both(sameTodo(t), func(ctx context.Context) bool { return anyWaitDecision(ctx, s.cwm, o.ID) == "" }),
		apply: func(ctx context.Context) (int, error) {
			wt := t
			wt.Result = &schedapi.Result{By: ReconcilerActor, Summary: c.Context}
			_, err := openChoice(ctx, wt, c)
			return 0, err
		},
	})
}

// waitChoice is the prepared decision on a wait no rule resolves. Its text
// never names the request (the decision's source and about edge do).
func waitChoice(step, on, why string, nobody bool, request string) api.Choice {
	why = strings.ReplaceAll(why, request, "this request")
	opts := []decisionsapi.OptionDetail{
		{Label: "Remove the wait", Means: "the step " + step + " goes back to the queue without waiting on " + on,
			Consequence: "the work goes on now; what the wait guarded is checked by its own step"},
		{Label: "Drop the request", Means: "the request is dropped, its open steps with it",
			Consequence: "nothing more is done on it"},
	}
	proposal := decisionsapi.Proposal{Option: "Remove the wait", Why: "nothing will end this wait by itself"}
	if nobody {
		opts = append([]decisionsapi.OptionDetail{{Label: "Plan the dependency", Means: "plan the work on " + on + " so the wait can end",
			Consequence: "the step resumes once that work has ended"}}, opts...)
		proposal = decisionsapi.Proposal{Option: "Plan the dependency", Why: "the wait is sound; only nobody works on what it waits on"}
	}
	return api.Choice{
		Question: "How does the wait of the step " + step + " on " + on + " end?",
		Context:  "The reconciler found a wait no rule of it can end: " + why + ".",
		Options:  opts, Proposal: proposal,
	}
}

// labels: a to-do not started whose labels are not those its request's
// increment, release and stream give it now.
func (s *reconSweep) labels(ctx context.Context, o cwmapi.Object) {
	want := s.lab.of(ctx, s.cwm, o.ID).labels
	for _, t := range s.open[o.ID] {
		if !notStarted(t) || slices.Equal(t.Labels, want) {
			continue
		}
		t := t
		s.add(finding{
			repair: api.Repair{Repair: api.RepairLabels, Work: o.ID, Todo: t.ID,
				Reason: "its labels [" + strings.Join(t.Labels, " ") + "] are out of date: [" + strings.Join(want, " ") + "]", Action: "settle it again: its labels recomputed"},
			still: sameTodo(t),
			apply: func(ctx context.Context) (int, error) {
				return 0, settleRequest(ctx, t.Request, ReconcilerActor)
			},
		})
	}
}

// readerIncidents raises the incidents of the one reader (K6) on the work no
// finding covers.
func (s *reconSweep) readerIncidents(ctx context.Context) error {
	out, err := NewPlanning().Check(ctx, api.PlanningCheckInput{Census: true})
	if err != nil {
		return err
	}
	for _, b := range out.Blockers {
		if b.Kind != api.BlockerIncident || s.covered[b.Work+"|"+b.Todo] {
			continue
		}
		if b.Cycle != "" && b.Cycle != b.Work {
			// A cycle of stored waits is one incident, on its root
			//
			continue
		}
		if b.Todo != "" && noRuleResolves(b) {
			if t, ok := s.todoOf(b.Work, b.Todo); ok {
				if o, err := loadObject(ctx, s.cwm, b.Work); err == nil {
					s.decideWait(ctx, o, t, orDefault(b.On, b.Work), b.Reason)
					continue
				}
			}
		}
		s.incident("check", b.Work, b.Todo, b.Cause, b.Reason, b.Actor)
	}
	return nil
}

// todoOf is one open to-do of a request, as the sweep read it.
func (s *reconSweep) todoOf(request, id string) (api.WorkTodo, bool) {
	for _, t := range s.open[request] {
		if t.ID == id {
			return t, true
		}
	}
	return api.WorkTodo{}, false
}

// dedupIncidents keeps the first incident of each key (sorted).
func dedupIncidents(in []api.Incident) []api.Incident {
	out := []api.Incident{}
	for i, x := range in {
		if i > 0 && in[i-1].Key == x.Key {
			continue
		}
		out = append(out, x)
	}
	return out
}

// defaultReconcileEvery is the sweep's period when Planification.yaml sets
// none.
const defaultReconcileEvery = 10 * time.Minute

// reconcileEvery is the sweep's period; 0 when the periodic sweep is off.
func reconcileEvery(ctx context.Context) time.Duration {
	switch m := planificationConfig(ctx).Reconciler.IntervalMinutes; {
	case m < 0:
		return 0
	case m == 0:
		return defaultReconcileEvery
	default:
		return time.Duration(m) * time.Minute
	}
}

// RunReconciler sweeps the stored state as the process starts serving its
// universe - its wake-up - and then every period, until ctx ends. It runs
// only in the serving process, the one holding the CWM lease: the lease is the
// election (a write from a process that lost it is refused). Each sweep
// applies the repairs switched on; with none it is a dry-run.
//
// The sweep is level-triggered: Planning.Reconcile
// (api.ReconcileInput) is its one entry - this loop, the PlanningService's
// published Reconcile and a process engine call it unchanged. It reads the stored state
// only, never an event: a second sweep over the same state repairs nothing
// and puts no to-do, and a restart changes nothing, its flap and day counts
// read back from the incidents' list.
func RunReconciler(ctx context.Context) {
	for {
		every := reconcileEvery(ctx)
		if every == 0 {
			slog.Info("planification: the reconciler's periodic sweep is off (reconciler.interval_minutes)")
			return
		}
		sweepOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// sweepOnce runs one sweep and says what it did.
func sweepOnce(ctx context.Context) {
	out, err := NewPlanning().Reconcile(ctx, api.ReconcileInput{Apply: true})
	if err != nil {
		slog.Warn("planification: the reconciler's sweep failed", "error", err)
		return
	}
	var applied, would []string
	for _, r := range out.Repairs {
		if r.Applied {
			applied = append(applied, r.Repair+":"+orDefault(r.Todo, r.Work))
		} else {
			would = append(would, r.Repair+":"+orDefault(r.Todo, r.Work))
		}
	}
	if len(applied) > 0 {
		slog.Info("planification: the reconciler repaired", "repairs", applied)
	}
	if len(would)+len(out.Incidents) > 0 {
		slog.Info("planification: the reconciler's sweep", "would_repair", len(would), "incidents", len(out.Incidents), "took_ms", out.TookMS, "dry_run", out.DryRun, "why", out.Why)
	}
}
