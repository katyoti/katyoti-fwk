// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// GATES — a move a named gate refused plans its resolution. For each unmet
// requirement with a `resolve`, the planner files the request that meets it
// (once: not again while one is open, at most maxResolveAttempts times); a
// person's requirement — after its work, when it has some — is asked as an
// `approve` human task about the object, satisfying the requirement
// (gatesapi.ConfirmationItem): its approving answer records the
// confirmation on the object, by whoever answered. The object carries the wait
// (gate-wait: the move); what resolves it is read back — the requests that
// resolve the object (or its own request), from the edges, filed since the
// wait began; the human tasks satisfying one of its requirements for this
// wait, open in the scheduler or answered under the object. When nothing
// resolving it is open any more the move is tried again, as the actor who
// asked for it. A retry the gate refuses again is the next round; a
// confirmation a person refuses ends the wait.

// maxResolveAttempts bounds the requests filed for one requirement of one wait.
const maxResolveAttempts = 3

// askPrefix opens who asks a gate's human task (its asked_by), followed by
// <gate>/<requirement>@<since>. The inbox validations it files while IN-039's
// switch is off (gates_inbox.go) carry it as their requested_by.
const askPrefix = "planification:gate:"

// resolution is one thing filed to meet a requirement: a request (the work) or
// a human task (a person confirms).
type resolution struct {
	Requirement string
	Request     string
	State       string // the request's state
	// Ask is the human task's to-do - or, with Entry, the inbox validation
	// asked before IN-039; Answered once its answer is kept.
	Ask      string
	Entry    bool
	Answered bool
	// Approved: the answer settles the requirement; By and Note are who
	// answered and what they said.
	Approved bool
	By       cwmapi.Actor
	Note     string
}

func (r resolution) open() bool {
	if r.Request != "" {
		return r.State != api.StateDone && r.State != api.StateDropped
	}
	return !r.Answered
}

// GateRefused plans the resolution of a move a guard refused because a gate()
// it calls failed; a refusal for any other reason is not the planner's.
// planner is who the gates' planning moves objects as.
var planner = FilingActor

// requestLocks serialises the gates' planning per object, so two news of one
// object never plan its resolution twice.
var requestLocks sync.Map

func lockRequest(id string) func() {
	m, _ := requestLocks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *Service) GateRefused(ctx context.Context, p cwmapi.ObjectTransitionRefusedEventPayload) error {
	cl, ok := gatesapi.FailingChecklist(p.Calls)
	if !ok {
		return nil
	}
	if p.Capability != "" {
		// A capability's own workflow (a plan's finish refused by the
		// module-updated gate) is settled by its owner — the end that tried it
		// plans the module update again; a gate wait follows an object's
		// lifecycle only. Returned before the lock: the end holding it may
		// hear its own refusal synchronously.
		return nil
	}
	gate := cl.Gate
	defer lockRequest(p.ObjectID)()
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, _, err := loadAny(ctx, cwm, p.ObjectID)
	if err != nil {
		return err
	}
	if lifeState(o) == p.To {
		return nil // already moved (a person forced it): the refusal is stale
	}
	w, waiting := gateWaitOf(o)
	same := waiting && w.Gate == gate && w.To == p.To && w.By == p.By && w.Transition == p.Transition
	if !same {
		if !waiting || w.Gate != gate || w.To != p.To {
			w = api.GateWaitFields{Since: time.Now().UTC().Format(time.RFC3339Nano)}
		}
		w.Gate, w.Transition, w.To, w.By = gate, p.Transition, p.To, p.By
		if err := saveGateWait(ctx, cwm, o, w, waiting); err != nil {
			return err
		}
	}
	rs, err := resolutionsOf(ctx, cwm, o, w)
	if err != nil {
		return err
	}
	for _, u := range cl.Unmet() {
		if err := s.resolve(ctx, cwm, o, w, u, rs); err != nil {
			return fmt.Errorf("gate %s on %s, requirement %s: %w", gate, p.ObjectID, u.Name, err)
		}
	}
	return nil
}

// resolve files what meets one unmet requirement, or nothing when something
// filed for it is still open, it declares no resolve, or its attempts ran out.
func (s *Service) resolve(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, w api.GateWaitFields, u gatesapi.GateItem, rs []resolution) error {
	var requests, worked int
	for _, r := range rs {
		if r.Requirement != u.Name {
			continue
		}
		if r.open() {
			return nil
		}
		if r.Request != "" {
			requests++
			if r.State == api.StateDone {
				worked++
			}
		}
	}
	title, _ := u.Resolve["title"].(string)
	person := u.Kind == "person"
	if u.Name == guiTestedRequirement {
		// A built feature's GUI test goes to its release's batch, not to a
		// request of its own.
		if batched, err := s.batchGUITests(ctx, cwm, o); batched || err != nil {
			return err
		}
	}
	switch {
	case title != "" && !(person && worked > 0):
		if requests >= maxResolveAttempts {
			return nil
		}
		kind, _ := u.Resolve["kind"].(string)
		module, err := cwmapi.ModuleOfObjectID(o.ID)
		if err != nil {
			return err
		}
		in := api.FileInput{
			Title: strings.ReplaceAll(title, "{object}", nameOf(o)),
			Details: fmt.Sprintf("Meets %q, a requirement of the %s gate, so that %s can move to %s.",
				labelOf(u), w.Gate, o.ID, w.To),
			Module: module, Kind: kind, By: planner, Resolves: o.ID, GateRequirement: w.Gate + "/" + u.Name,
			// A gate refusing a move is a rule failing: it says so on what it files.
			Source: &api.RequestSource{Kind: api.SourceGate, Name: w.Gate + "/" + u.Name, Object: o.ID, Times: 1,
				Change: "its move to " + w.To + " was refused: " + labelOf(u) + " is not met", FiredAt: time.Now().UTC().Format(time.RFC3339)},
		}
		// `on: object` asks the work on the object itself — its type×state
		// recipe plans it (goal-decompose, feature-specify). An object already
		// carrying a request is that work: while it is open, nothing is filed.
		if on, _ := u.Resolve["on"].(string); on == "object" {
			if hasRequest(o) {
				return nil
			}
			in.On, in.Resolves = o.ID, ""
		}
		_, err = s.File(ctx, in)
		return err
	case person:
		return askConfirmation(ctx, cwm, o, w, u)
	}
	return nil
}

// GateProgress follows a waiting object: it records the answered validations
// (an approval confirms its requirement), and once nothing resolving the wait
// is open, tries the move again.
func (s *Service) GateProgress(ctx context.Context, id string) error {
	defer lockRequest(id)()
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return err
	}
	w, waiting := gateWaitOf(o)
	if !waiting {
		return nil
	}
	if lifeState(o) == w.To {
		// Moved meanwhile and GateMoved did not end the wait (a remote gater,
		// an error): the journal's last move says passed or waived
		//
		return s.passed(ctx, cwm, o, w, waivedBy(movedWith(ctx, id, w.To), w))
	}
	rs, err := resolutionsOf(ctx, cwm, o, w)
	if err != nil {
		return err
	}
	pending := false
	for _, r := range rs {
		switch {
		case r.open():
			pending = true
		case r.Ask == "":
		case !r.Approved:
			// A person refused to confirm: the move is not tried again.
			return endGateWait(ctx, cwm, id)
		default:
			if err := confirm(ctx, id, w.Gate, r); err != nil {
				return err
			}
		}
	}
	if pending {
		return nil
	}
	// Nothing open: the move is tried again, the wait kept until it passes. A
	// refusal by the gate is published, and plans the next round from what was
	// filed since the wait began; any other refusal ends the wait.
	note := "tried again: what resolved the " + w.Gate + " gate ended"
	if o.Type == api.ReleaseType && w.To == api.ReleaseReleased && len(releaseFields(o).Candidates) > 0 {
		// A delivered release is released by promoting its last candidate:
		// its gates checked again, its build recorded as the official one.
		_, err = NewReleases().Promote(ctx, api.CandidateInput{ID: id, By: w.By, Note: note})
	} else {
		_, err = cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: w.To, Transition: w.Transition, By: w.By, Note: note})
	}
	if _, refused := gatesapi.RefusedChecklist(err); refused {
		return nil
	}
	if end := endGateWait(ctx, cwm, id); end != nil && err == nil {
		err = end
	}
	return err
}

// GateMoved follows a transition: an object that reached the state its wait
// was for — forced past the gate, moved by hand — no longer waits. Reached by
// another transition than the one refused (a person's confirm: R-006's
// waiver), the gate was not passed: its wait ends, but the work filed to
// resolve it stays open - a GUI test stays planned.
func (s *Service) GateMoved(ctx context.Context, id, to, transition string) error {
	// Locked before reading: a refusal being planned for this object finishes
	// first, so its wait is seen here (and ended), never written just after.
	defer lockRequest(id)()
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return err
	}
	if w, waiting := gateWaitOf(o); waiting && w.To == to {
		return s.passed(ctx, cwm, o, w, waivedBy(transition, w))
	}
	return nil
}

// waivedBy says whether a move by transition past a wait waived its gate
// rather than passed it: another transition than the one refused (a person's
// confirm, R-006). An unknown transition is a pass, as before.
func waivedBy(transition string, w api.GateWaitFields) bool {
	return transition != "" && w.Transition != "" && transition != w.Transition
}

// movedWith is the transition of id's last move to `to`, read from its
// journal; empty when the journal cannot say.
func movedWith(ctx context.Context, id, to string) string {
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		return ""
	}
	got, err := journal.Transitions(ctx, journalapi.TransitionsInput{ID: id})
	if err != nil {
		return ""
	}
	for i := len(got.Entries) - 1; i >= 0; i-- {
		if e := got.Entries[i]; e.To == to {
			return e.Transition
		}
	}
	return ""
}

// passed ends a wait whose move happened: what was filed to resolve it and is
// still open has no purpose left — the requests are dropped, the questions
// withdrawn, each saying why. The object's own request is its own work, kept.
// A waived gate (moved by another transition) keeps its requests: what they
// do is still to be done; only its questions are withdrawn.
func (s *Service) passed(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, w api.GateWaitFields, waived bool) error {
	rs, err := resolutionsOf(ctx, cwm, o, w)
	if err != nil {
		return err
	}
	why := fmt.Sprintf("%s moved to %s: the %s gate it resolved was passed", o.ID, w.To, w.Gate)
	if waived {
		why = fmt.Sprintf("%s moved to %s past the %s gate (waived, not passed)", o.ID, w.To, w.Gate)
	}
	for _, r := range rs {
		if !r.open() {
			continue
		}
		switch {
		case waived && r.Request != "":
			// Its work stays to do.
		case r.Request != "" && r.Request != o.ID:
			if _, err := NewPlans().Finish(ctx, api.FinishInput{ID: r.Request, Outcome: api.PlanDropped, By: planner, Note: why}); err != nil {
				return fmt.Errorf("drop %s: %w", r.Request, err)
			}
		case r.Entry:
			if err := dismissEntry(ctx, r.Ask); err != nil {
				return fmt.Errorf("dismiss %s: %w", r.Ask, err)
			}
		case r.Ask != "":
			if err := withdrawConfirmation(ctx, r.Ask, why); err != nil {
				return fmt.Errorf("withdraw %s: %w", r.Ask, err)
			}
		}
	}
	return endGateWait(ctx, cwm, o.ID)
}

// withdrawConfirmation takes back a gate's question no one answered: its to-do is
// archived, unanswered, and the run waiting on it stopped - no answer will
// come.
func withdrawConfirmation(ctx context.Context, todo, why string) error {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return err
	}
	got, err := todos.Get(ctx, schedapi.GetInput{ID: todo})
	if err != nil {
		return err
	}
	if _, err := todos.Archive(ctx, schedapi.ArchiveInput{ID: todo, Note: "withdrawn: " + why}); err != nil {
		return err
	}
	if run, _ := got.Todo.Work["run"].(string); run != "" {
		if ts, err := application.UseService[taskapi.TaskService](ctx); err == nil {
			_, _ = ts.Stop(ctx, taskapi.StopRequest{RunID: run})
		}
	}
	return nil
}

// confirm records an approving answer on the object, once, by whoever answered.
func confirm(ctx context.Context, id, gate string, r resolution) error {
	gates, err := application.UseService[gatesapi.GatesService](ctx)
	if err != nil {
		return err
	}
	chk, err := gates.Check(ctx, gatesapi.CheckInput{ID: id, Gate: gate})
	if err != nil {
		return err
	}
	for _, it := range chk.Checklist.Items {
		if it.Name == r.Requirement && it.ConfirmedBy != nil {
			return nil
		}
	}
	_, err = gates.Confirm(ctx, gatesapi.ConfirmInput{ID: id, Gate: gate, Requirement: r.Requirement, By: r.By, Note: r.Note})
	return err
}

// GateRequestEnded follows the object a request that just ended resolves —
// the one it points at, or the object carrying it.
func (s *Service) GateRequestEnded(ctx context.Context, request string) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	target := o.EdgeTo(api.EdgeResolves)
	if _, waiting := gateWaitOf(o); target == "" && waiting {
		target = o.ID
	}
	if target == "" {
		return nil
	}
	return s.GateProgress(ctx, target)
}

// GateAnswerKept follows the objects a kept answer confirms: an answer
// record whose human task satisfied a requirement of a gate
// wait - its object's wait is followed (the confirmation recorded, the move
// tried again). It reports whether the answer confirmed any.
func (s *Service) GateAnswerKept(ctx context.Context, answer string) (bool, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return false, err
	}
	o, err := loadObject(ctx, cwm, answer)
	if err != nil || o.Type != inboxapi.AnswerType {
		return false, err
	}
	rec, ok := answerRecordOf(o)
	if !ok {
		return false, nil
	}
	seen := map[string]bool{}
	for _, item := range rec.Satisfies {
		c, ok := gatesapi.ParseConfirmationItem(item)
		if !ok || seen[c.Object] {
			continue
		}
		seen[c.Object] = true
		if err := s.GateProgress(ctx, c.Object); err != nil {
			return true, err
		}
	}
	return len(seen) > 0, nil
}

// askConfirmation asks a person to confirm one requirement: an `approve`
// human task about the object, satisfying the requirement for this wait. The
// human task runs on its own: its answer, kept under the object, is followed
// by GateAnswerKept.
func askConfirmation(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, w api.GateWaitFields, u gatesapi.GateItem) error {
	verb := "Confirm"
	if in, _ := u.Resolve["interaction"].(string); in != "" {
		verb = strings.ToUpper(in[:1]) + in[1:]
	}
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return err
	}
	ask := inboxapi.HumanTask{
		Kind:  inboxapi.KindApprove,
		Title: fmt.Sprintf("%s: %s — %s", verb, labelOf(u), nameOf(o)),
		Question: fmt.Sprintf("%q is a requirement of the %s gate that a person confirms. %s (%s) waits on it to move to %s. Approve to confirm it — the move is then tried again; a rejection leaves the move refused.",
			labelOf(u), w.Gate, nameOf(o), o.ID, w.To),
		About:     o.ID,
		Satisfies: []string{gatesapi.ConfirmationItem(o.ID, w.Gate, u.Name, w.Since)},
		// A gate's confirmation is a person's act: gates.Confirm refuses an agent.
		AnswerableBy: inboxapi.AnswerablePerson,
		AskedBy:      askPrefix + w.Gate + "/" + u.Name + "@" + w.Since,
	}
	input, err := asInput(ask)
	if err != nil {
		return err
	}
	_, err = ts.Run(ctx, taskapi.TaskRunRequest{
		TaskDef: map[string]any{"name": "gate-confirmation", "type": inboxapi.HumanTaskStrategy, "description": "A person confirms a gate's requirement"},
		Input:   input,
		Origin:  &taskapi.RunOrigin{Kind: taskapi.OriginPlanner, ID: w.Gate + "/" + u.Name},
		Target:  o.ID,
	})
	if err != nil {
		return fmt.Errorf("ask a person to confirm: %w", err)
	}
	return nil
}

// asInput is a human task as a run's input: its JSON fields.
func asInput(ask inboxapi.HumanTask) (map[string]any, error) {
	raw, err := json.Marshal(ask)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	return m, json.Unmarshal(raw, &m)
}

// resolutionsOf reads what was filed for a wait: the requests resolving the
// object (and its own request), from the edges, of the wait's gate filed since
// it began; the human tasks satisfying one of its requirements for this wait
// - open in the scheduler, or answered: kept under the object.
func resolutionsOf(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, w api.GateWaitFields) ([]resolution, error) {
	since, _ := time.Parse(time.RFC3339Nano, w.Since)
	after := func(at string) bool {
		t, err := time.Parse(time.RFC3339Nano, at)
		return err != nil || !t.Before(since)
	}
	requests, err := incoming(ctx, cwm, o.ID, api.EdgeResolves)
	if err != nil {
		return nil, err
	}
	if hasRequest(o) {
		requests = append(requests, o.ID)
	}
	var out []resolution
	for _, id := range requests {
		q, err := load(ctx, cwm, id)
		if err != nil {
			return nil, err
		}
		f := fieldsOf(q)
		gate, req, ok := strings.Cut(f.GateRequirement, "/")
		if !ok || gate != w.Gate || !after(f.FiledAt) {
			continue
		}
		out = append(out, resolution{Requirement: req, Request: id, State: stateOf(q)})
	}
	asks, err := confirmationsAsked(ctx, cwm, o, w)
	if err != nil {
		return nil, err
	}
	entries, err := confirmationEntries(ctx, o, w)
	if err != nil {
		return nil, err
	}
	return append(append(out, asks...), entries...), nil
}

// forWait is the requirement a confirmation item asks for this wait, if it
// does.
func forWait(item string, o cwmapi.Object, w api.GateWaitFields) (string, bool) {
	c, ok := gatesapi.ParseConfirmationItem(item)
	if !ok || c.Object != o.ID || c.Gate != w.Gate || c.Since != w.Since {
		return "", false
	}
	return c.Requirement, true
}

// confirmationsAsked are the human tasks asked for the wait: the answered
// ones, read from the answers kept under the object, then those still open.
// A to-do answered but not yet kept (its answer being checked) is open.
func confirmationsAsked(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, w api.GateWaitFields) ([]resolution, error) {
	var out []resolution
	kept := map[string]bool{}
	answers, err := childrenOf(ctx, cwm, o.ID, inboxapi.AnswerType)
	if err != nil {
		return nil, err
	}
	for _, a := range answers {
		if len(a.Fields) == 0 {
			if full, err := loadObject(ctx, cwm, a.ID); err == nil {
				a = full
			}
		}
		rec, ok := answerRecordOf(a)
		if !ok {
			continue
		}
		for _, item := range rec.Satisfies {
			req, ok := forWait(item, o, w)
			if !ok {
				continue
			}
			kept[rec.Task] = true
			out = append(out, resolution{Requirement: req, Ask: rec.Task, Answered: true,
				Approved: rec.Status == schedapi.StatusSuccess && rec.Verdict != "rejected",
				By:       cwmapi.Actor{ID: rec.ByID, Kind: rec.ByKind}, Note: answerNote(rec)})
		}
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return out, nil // no scheduler: no human task is open
	}
	open, err := todos.List(ctx, schedapi.ListInput{})
	if err != nil {
		return nil, err
	}
	for _, t := range open.Todos {
		ask, ok := inboxapi.AskOf(t)
		if !ok || kept[t.ID] || ask.About != o.ID {
			continue
		}
		for _, item := range ask.Satisfies {
			if req, ok := forWait(item, o, w); ok {
				out = append(out, resolution{Requirement: req, Ask: t.ID})
			}
		}
	}
	return out, nil
}

// answerRecordOf reads a kept answer's fields.
func answerRecordOf(o cwmapi.Object) (inboxapi.AnswerRecord, bool) {
	var rec inboxapi.AnswerRecord
	raw := o.FieldsJSON()
	if raw == nil || json.Unmarshal(raw, &rec) != nil || rec.Task == "" {
		return inboxapi.AnswerRecord{}, false
	}
	return rec, true
}

// answerNote is what the person said with their answer.
func answerNote(rec inboxapi.AnswerRecord) string {
	for _, s := range []string{rec.Answer, rec.Summary, rec.Details} {
		if s != "" {
			return s
		}
	}
	return ""
}

func gateWaitOf(o cwmapi.Object) (api.GateWaitFields, bool) {
	f, ok, _ := cwmapi.DecodeCapability[api.GateWaitFields](o, api.CapGateWait)
	return f, ok
}

// saveGateWait writes the wait on the object. A first wait is given through
// the capability verb; a wait the object already carries is replaced in ONE
// conditional write (the object read with its revision, the field group set,
// written back at that revision, read again on a conflict) - never taken off
// and given again: between those two writes the object carried no wait, and a
// reader then (a second refusal of the same move by another actor - an
// agent's verify racing planification's own) saw none.
func saveGateWait(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, w api.GateWaitFields, carried bool) error {
	fields, err := capabilityMap(w)
	if err != nil {
		return err
	}
	if !carried {
		_, err = cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: o.ID, Capability: api.CapGateWait, Fields: fields})
		return err
	}
	module, err := cwmapi.ModuleOfObjectID(o.ID)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: o.ID})
		if err != nil {
			return err
		}
		cur := got.Object
		if err := cur.SetCapability(api.CapGateWait, fields); err != nil {
			return err
		}
		_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: cur, Revision: got.Revision})
		if !errors.Is(err, cwmapi.ErrConflict) || attempt >= gateWaitWriteAttempts {
			return err
		}
	}
}

// gateWaitWriteAttempts bounds the rereads of a wait's replacement that a
// concurrent write refused.
const gateWaitWriteAttempts = 8

// endGateWait takes the wait off the object.
func endGateWait(ctx context.Context, cwm cwmapi.CWMService, id string) error {
	_, err := cwm.ObjectRemoveCapability(ctx, cwmapi.ObjectRemoveCapabilityInput{ID: id, Capability: api.CapGateWait})
	return err
}

func nameOf(o cwmapi.Object) string {
	if o.Name != "" {
		return o.Name
	}
	return o.ID
}

func labelOf(u gatesapi.GateItem) string {
	if u.Label != "" {
		return u.Label
	}
	return u.Name
}

// verdict is an answered confirmation's verdict as the views show it.
func (r resolution) verdict() string {
	if r.Approved {
		return inboxapi.StatusApproved
	}
	return inboxapi.StatusRejected
}
