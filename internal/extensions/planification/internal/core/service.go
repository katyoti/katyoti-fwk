// SPDX-License-Identifier: Apache-2.0

// Package core implements the request service over the CWM Object API. Work
// asked for is the `request` capability of an object — a `request` filed on
// its own, or a goal, a feature, a use case given one: its fields are the
// request's group, its state the request's own (beside the object's
// lifecycle), its dependencies its `depends-on` edges. Nothing is stored here.
package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/projectcmd"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// Service implements api.RequestService.
type Service struct{}

// New returns the request service.
func New() *Service { return &Service{} }

var _ api.RequestService = (*Service)(nil)

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return nil, err
	}
	return projectCWM{cwm}, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: requests: "+format, append([]any{cwmapi.ErrInvalidInput}, args...)...)
}

// personsAct is iam's one rule for the person's own acts (person.AcceptRelayed)
// as planification asks it: by is the person, what names the act and where the
// person does it. Refused as invalid input; else the relay to record, nil for
// the person's own call.
func personsAct(ctx context.Context, by cwmapi.Actor, act, what string) (*api.Relayed, error) {
	r, err := person.AcceptRelayed(ctx, by.ID, act, what)
	if err != nil {
		return nil, invalid("%s", err.Error())
	}
	if r == nil {
		return nil, nil
	}
	c, _ := cause.CauseFrom(ctx)
	return &api.Relayed{By: c.ID, Confirmed: r.Confirmed, At: r.At}, nil
}

// checkRequester checks where a request comes from: a person or an agent; a
// module, a feature or a goal that exists (of that type); an external system
// named by its id.
func checkRequester(ctx context.Context, cwm cwmapi.CWMService, by cwmapi.Actor) error {
	switch by.Kind {
	case "person", "agent":
		return checkActor(by)
	case api.RequesterExternal:
		if strings.TrimSpace(by.ID) == "" {
			return invalid("an external requester needs the system's id")
		}
		return nil
	case api.RequesterModule, api.RequesterFeature, api.RequesterGoal:
		if by.ID == "" {
			return invalid("a %s requester needs the %s's id", by.Kind, by.Kind)
		}
		o, err := loadObject(ctx, cwm, by.ID)
		if err != nil {
			return fmt.Errorf("requests.File: requester %s %s: %w", by.Kind, by.ID, err)
		}
		// A module requester is any object at the module level - a system
		// module too - asked by its capability, never by its type's name.
		isModule := by.Kind == api.RequesterModule && carriesCapability(ctx, cwm, o.ID, cwmapi.CapModule)
		if o.Type != by.Kind && !isModule {
			return invalid("requester %s is a %s, not a %s", by.ID, o.Type, by.Kind)
		}
		return nil
	default:
		return invalid("a requester's kind is one of %v, got %+v", api.RequesterKinds, by)
	}
}

// checkActor refuses a move nobody is recorded as making.

func checkActor(by cwmapi.Actor) error {
	if by.ID == "" || (by.Kind != "person" && by.Kind != "agent") {
		return invalid("by needs an id and a kind (person or agent), got %+v", by)
	}
	return nil
}

// hasRequest reports an object carrying a request.
func hasRequest(o cwmapi.Object) bool {
	_, ok := o.Capabilities[api.CapRequest]
	return ok
}

// load reads an object carrying a request and refuses any other.
func load(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.Object{}, err
	}
	if !hasRequest(got.Object) {
		return cwmapi.Object{}, invalid("%s (a %s) carries no request", id, got.Object.Type)
	}
	return inProject(got.Object, id), nil
}

// stateOf is a request's state: its plan's (idle while it has none).
func stateOf(o cwmapi.Object) string { return planState(o) }

// loadNode reads a node of a release's network: a request or a step.
func loadNode(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, error) {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	if !hasRequest(o) && o.Type != api.StepType && o.Type != api.IncrementType {
		return cwmapi.Object{}, invalid("%s is a %q carrying no request; only requests, steps and increments are ordered", id, o.Type)
	}
	return o, nil
}

// nodeState is a network node's state, its workflow's initial one when unset:
// a request's (its plan's), a step's lifecycle.
func nodeState(o cwmapi.Object) string {
	if hasRequest(o) {
		return stateOf(o)
	}
	if st := lifeState(o); st != "" {
		return st
	}
	if o.Type == api.StepType {
		return api.StepOpen
	}
	return api.PlanIdle
}

// nodeDone: a request is done (its deliverables verified), a step reached.
func nodeDone(o cwmapi.Object) bool {
	st := nodeState(o)
	return st == api.StateDone || (o.Type == api.StepType && st == api.StepReached)
}

// decided reports a decision its own workflow has settled: no longer open
// (it applies, or was superseded, retired or dropped). Its end is its
// workflow's, never a plan's `done`: what waits on it goes on (its own plan
// still ends by its steps).
func decided(o cwmapi.Object) bool {
	if _, ok := o.Capabilities["decision"]; !ok {
		return false
	}
	st := ownState(o)
	return st != "" && st != "open"
}

// nodeEnded: done, reached or dropped — nothing more will come of it.
func nodeEnded(o cwmapi.Object) bool {
	return nodeDone(o) || nodeState(o) == api.StateDropped
}

func fieldsOf(o cwmapi.Object) api.RequestFields {
	f, _, _ := cwmapi.DecodeCapability[api.RequestFields](o, api.CapRequest)
	return f
}

// dependsOn is the request's depends-on targets, in authored order.
func dependsOn(o cwmapi.Object) []string {
	var out []string
	for _, e := range o.Edges {
		if e.Kind == api.EdgeDependsOn {
			out = append(out, e.To)
		}
	}
	return out
}

func toRequest(o cwmapi.Object) api.Request {
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	f := fieldsOf(o)
	r := api.Request{
		ID: o.ID, Type: o.Type, Module: module, Title: titleOf(o), State: stateOf(o),
		DependsOn: dependsOn(o), RequestFields: f,
		SplitFrom: o.EdgeTo(api.EdgeSplitFrom), FollowsUp: o.EdgeTo(api.EdgeFollowsUp), Resolves: o.EdgeTo(api.EdgeResolves),
	}
	for _, e := range o.Edges {
		switch e.Kind {
		case api.EdgeAskedIn:
			r.AskedIn = append(r.AskedIn, e.To)
		case api.EdgeServes:
			r.Serves = append(r.Serves, e.To)
		}
	}
	if pf, has := planOf(o); has {
		pf.State = stateOf(o)
		r.Plan = &pf
	}
	return r
}

// FilingActor is who plans a request's intake when its source — a module, a
// feature, a goal, an external system — files but moves nothing.
var FilingActor = cwmapi.Actor{ID: "planification", Kind: "agent"}

// IntakeTask is the task of a request's first step.
const IntakeTask = "intake"

// refile files a request again on an object whose request ended: its fields
// are the new request's, its plan is refiled (done or dropped → ready, the
// trail kept). An open one is refused: answer it, or add to it.
func refile(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, f api.RequestFields, by cwmapi.Actor) (cwmapi.Object, error) {
	if st := stateOf(o); !planEnded(st) {
		return o, fmt.Errorf("%w: File: %s already carries an open request (%s) — finish it, or add to it", cwmapi.ErrAlreadyExists, o.ID, st)
	}
	// What an increment's proposal made of it stays its record: its delivery
	// approval, filed on it again, shows each delivery goal and what it
	// became; goals changed later are proposed again against what was applied.
	// Another demand filed again starts afresh: an old apply is never replayed.
	prior := fieldsOf(o)
	if o.Type == api.IncrementType && len(f.Delta) == 0 && len(prior.Delta) > 0 {
		f.Delta, f.Applied = prior.Delta, prior.Applied
	}
	keepFiling(&f, prior)
	if err := o.SetCapability(api.CapRequest, f); err != nil {
		return o, err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
		return o, err
	}
	if by.Kind != "person" && by.Kind != "agent" {
		by = FilingActor
	}
	return planTransition(ctx, cwm, o.ID, api.PlanReady, "refile", by, "filed again")
}

// keepFiling carries what the earlier filing recorded into a refiling: who
// asked first stays the requester (whoever refiles it — no verb sets a
// requester back), and the assessment — priority, estimate, depth — is kept
// where the new filing states none; the intake that follows may assess it
// again. The kind stays the new filing's: it names the next rung of the work.
func keepFiling(f *api.RequestFields, prior api.RequestFields) {
	if prior.Requester.ID != "" {
		f.Requester = prior.Requester
	}
	if f.Priority == "" {
		f.Priority = prior.Priority
	}
	if f.Estimate == 0 {
		f.Estimate = prior.Estimate
	}
	if f.Depth == "" {
		f.Depth = prior.Depth
	}
}

// planIntake makes a request's plan ready for its intake — its first to-do,
// in the requests phase, planned by Work.Plan — and reads it back.
func planIntake(ctx context.Context, cwm cwmapi.CWMService, id string, by cwmapi.Actor, note string) (cwmapi.Object, error) {
	if by.Kind != "person" && by.Kind != "agent" {
		by = FilingActor
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	in := api.PlanNextInput{ID: id, By: by, Note: note}
	if f, has := planOf(o); !has || f.Priority == "" {
		in.Priority = fieldsOf(o).Priority
	}
	if _, err := NewPlans().plan(ctx, in); err != nil {
		return cwmapi.Object{}, fmt.Errorf("plan the intake of %s: %w", id, err)
	}
	return loadObject(ctx, cwm, id)
}

// checkAsk refuses a kind, a priority or a deliverable no request may have.
// kept are the deliverables already asked (a revise's own, a split's
// original): one unchanged keeps its check, runnable or not — the rule binds
// what is written now, never erases what a person verified by hand.
func checkAsk(verb, kind, priority string, deliverables, kept []api.Deliverable) error {
	if priority != "" && !slices.Contains(api.Priorities, priority) {
		return invalid("%s: priority %q is not one of %v", verb, priority, api.Priorities)
	}
	if kind != "" && !slices.Contains(api.Kinds, kind) {
		return invalid("%s: kind %q is not one of %v", verb, kind, api.Kinds)
	}
	for _, d := range deliverables {
		if strings.TrimSpace(d.Title) == "" {
			return invalid("%s: every deliverable needs a title", verb)
		}
		if d.Verified {
			return invalid("%s: deliverable %q is filed verified; it is verified by its check, after the work", verb, d.Title)
		}
		if slices.ContainsFunc(kept, func(k api.Deliverable) bool {
			return k.Title == d.Title && k.Check == d.Check && k.Timeout == d.Timeout
		}) {
			continue
		}
		if err := checkRunnable(d.Check); err != nil {
			return invalid("%s: deliverable %q: %v", verb, d.Title, err)
		}
		if _, _, err := projectcmd.ParseTimeout(d.Timeout); err != nil {
			return invalid("%s: deliverable %q: %v", verb, d.Title, err)
		}
	}
	return nil
}

// keptBySplit is what a split part's filing keeps as asked: Split checked its
// deliverables against the original's already.
func keptBySplit(in api.FileInput, splitFrom string) []api.Deliverable {
	if splitFrom == "" {
		return nil
	}
	return in.Deliverables
}

// File files a request — on an object (fileOn), or as a request of its own in
// the planning space of the module it targets (sys-planification when it names
// none) — and plans its intake.
func (s *Service) File(ctx context.Context, in api.FileInput) (api.FileOutput, error) {
	inc := strings.TrimSpace(in.Increment)
	if inc == "" {
		return s.fileRequest(ctx, in)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.FileOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, inc)
	if err != nil {
		return api.FileOutput{}, err
	}
	reopen := false
	if st := releaseState(o); st == api.IncrementDone && carriedUnreleased(ctx, cwm, o) {
		// A fix on what a release not yet released carries - or on what
		// rides along in its stream's next one - reopens it (decision
		// ), once the fix is filed.
		reopen = true
	} else if st != api.IncrementOpen {
		return api.FileOutput{}, invalid("File: %s is %s: work joins only an open increment, or a done one its release does not ship yet", inc, st)
	}
	if why := definedNotRun(o); why != "" {
		return api.FileOutput{}, invalid("File: %s", why)
	}
	out, err := s.fileRequest(ctx, in)
	if err != nil {
		return out, err
	}
	if reopen {
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: inc, To: api.IncrementOpen, By: in.By,
			Note: "reopened: " + out.Request.ID + " joins it"}); err != nil {
			return out, fmt.Errorf("File: %s was filed but did not reopen %s: %w", out.Request.ID, inc, err)
		}
	}
	proposed, err := joinIncrement(ctx, cwm, out.Request.ID, inc, in.By)
	if err != nil {
		return out, fmt.Errorf("File: %s was filed but did not join %s: %w", out.Request.ID, inc, err)
	}
	out.Proposed = proposed
	return out, nil
}

// carriedUnreleased is whether a done increment ships in a live release not yet
// released (planned or frozen), or rides along in its stream's next one: a
// fix on it reopens it.
func carriedUnreleased(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object) bool {
	r := edgeTo(inc, api.EdgeShipsIn)
	if r != "" {
		if ro, _, err := loadAny(ctx, cwm, r); err == nil {
			switch releaseState(ro) {
			case api.ReleasePlanned, api.ReleaseFrozen:
				return true
			case api.ReleaseDropped:
				r = "" // freed: it rides along again
			default:
				return false
			}
		}
	}
	if sm := edgeTo(inc, api.EdgeInStream); r == "" && sm != "" {
		_, ok := nextPlannedRelease(ctx, cwm, sm)
		return ok
	}
	return false
}

// joinIncrement aims what a request was filed on - a goal or a feature, the
// increment holding it - or the request itself, as direct work in it, at a
// running increment; every to-do settles at once, labelled with it. A goal or
// a feature the running increment does not hold yet is proposed instead
// : the proposal's id is answered and nothing joins
// until a person approves it. Only a request of its own is recorded joined
// (markJoined); a want newly aimed is recorded by the increment's takes.
func joinIncrement(ctx context.Context, cwm cwmapi.CWMService, id, inc string, by cwmapi.Actor) (string, error) {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return "", err
	}
	if notIncrementWork(o) {
		out, err := NewIncrements().Target(ctx, api.IncrementTargetInput{ID: o.ID, Increment: inc, By: by})
		if err != nil {
			return "", err
		}
		if out.Proposed != "" {
			return out.Proposed, settleAll(ctx, PlanningActor)
		}
	} else if _, err := retarget(ctx, cwm, o.ID, inc); err != nil {
		return "", err
	}
	if err := markJoined(ctx, cwm, o, inc); err != nil {
		return "", err
	}
	return "", settleAll(ctx, PlanningActor)
}

// markJoined records that a request joined a running increment: the joined
// edge from the request, a planning object, to the increment - the delivery
// card and the GUI batch read it apart from the work the increment's own
// proposal planned. A goal or a feature carries none
// (rule): the increment's takes edge records it.
func markJoined(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, inc string) error {
	if notIncrementWork(o) {
		return nil
	}
	return addEdge(ctx, cwm, o.ID, api.EdgeJoined, inc)
}

// addEdge gives an object one edge of a kind to a target, once.
func addEdge(ctx context.Context, cwm cwmapi.CWMService, id, kind, to string) error {
	o, module, err := loadAny(ctx, cwm, id)
	if err != nil {
		return err
	}
	for _, e := range o.Edges {
		if e.Kind == kind && e.To == to {
			return nil
		}
	}
	o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: kind, To: to})
	_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	return err
}

// fileRequest files a request (File, the increment aside).
func (s *Service) fileRequest(ctx context.Context, in api.FileInput) (api.FileOutput, error) {
	return s.file(ctx, in, "")
}

// file files a request — splitFrom names the request it replaces, if any —
// and plans it at once: its to-dos, the intake first, in its module's queue.
// A plan that cannot be made leaves the request filed, unplanned, and says so
// in the log (`work plan` makes it later).
func (s *Service) file(ctx context.Context, in api.FileInput, splitFrom string) (api.FileOutput, error) {
	out, err := s.fileOnly(ctx, in, splitFrom)
	if err != nil {
		return out, err
	}
	if _, perr := NewWork().Plan(ctx, api.PlanWorkInput{Request: out.Request.ID, By: PlanningActor}); perr != nil {
		slog.Warn("planification: a request filed was not planned", "request", out.Request.ID, "error", perr)
	}
	if splitFrom == "" {
		awaitAnswer(ctx, out.Request)
	}
	return out, nil
}

// awaitAnswer records, when a module asks another, that it awaits the answer
// (the scheduler's waits: a file with a deadline, removed when the request
// ends). A part of a split is covered by the wait on what was split.
func awaitAnswer(ctx context.Context, r api.Request) {
	by := r.Requester
	if by.Kind != "module" || by.ID == r.Module {
		return
	}
	waits, err := application.UseService[schedapi.WaitService](ctx)
	if err != nil {
		return // a binary without the scheduler keeps no waits
	}
	// Both modules named in the request's project (several projects served).
	ref := requestRef(ctx, r.ID)
	if _, err := waits.Add(ctx, schedapi.AddWaitInput{Module: moduleIn(ref, by.ID), On: bareID(ref), From: moduleIn(ref, r.Module), What: r.Title, By: PlanningActor}); err != nil {
		slog.Warn("planification: what a module awaits was not recorded", "module", by.ID, "request", r.ID, "error", err)
	}
}

// fileOnly files a request.
func (s *Service) fileOnly(ctx context.Context, in api.FileInput, splitFrom string) (api.FileOutput, error) {
	if err := s.checkLinks(ctx, in); err != nil {
		return api.FileOutput{}, err
	}
	if in.On != "" {
		return s.fileOn(ctx, in, splitFrom)
	}
	if strings.TrimSpace(in.Title) == "" {
		return api.FileOutput{}, invalid("File: title required")
	}
	if in.Kind == "" && in.By.Kind == "person" && splitFrom == "" {
		// What a person asks, with no kind, is a demand: it reconciles and does
		// no work itself. Direct work is filed as such (a fix, a quick build);
		// a module's, a trigger's, an agent's ask keeps intake's kind.
		in.Kind = api.KindDemand
	}
	if err := checkAsk("File", in.Kind, in.Priority, in.Deliverables, keptBySplit(in, splitFrom)); err != nil {
		return api.FileOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.FileOutput{}, err
	}
	if err := checkRequester(ctx, cwm, in.By); err != nil {
		return api.FileOutput{}, err
	}
	module := in.Module
	if module == "" {
		if module, _, err = ensureModule(ctx, cwm); err != nil {
			return api.FileOutput{}, err
		}
	} else if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: module}); err != nil {
		return api.FileOutput{}, fmt.Errorf("requests.File: module %s: %w", module, err)
	}
	for _, d := range in.DependsOn {
		if _, err := loadNode(ctx, cwm, d); err != nil {
			return api.FileOutput{}, fmt.Errorf("requests.File: depends on %s: %w", d, err)
		}
	}
	o := cwmapi.Object{Type: api.RequestType}
	o.Parent = module
	o.Name = strings.TrimSpace(in.Title)
	if err := o.SetCapability(api.CapRequest, filed(in, splitFrom)); err != nil {
		return api.FileOutput{}, err
	}
	for _, d := range in.DependsOn {
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeDependsOn, To: d})
	}
	o.SetEdgeTo(api.EdgeSplitFrom, splitFrom)
	o.SetEdgeTo(api.EdgeResolves, in.Resolves)
	o.SetEdgeTo(api.EdgeAbout, in.About)
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		return api.FileOutput{}, err
	}
	// Filed in a module named in its project (<key>:<module>), the request is
	// that project's: it is named so from here on (two projects may hold its id).
	rid := moduleIn(module, out.Object.ID)
	planned, err := planIntake(ctx, cwm, rid, in.By, "filed")
	if err != nil {
		// Filed with its plan or not at all: a request left with none would sit
		// unplanned.
		if _, derr := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: module, ID: rid, Detach: true}); derr != nil {
			slog.Warn("planification: a request whose plan failed stays filed without one", "request", rid, "error", derr)
		}
		return api.FileOutput{}, err
	}
	r := toRequest(planned)
	r.ID = rid
	return api.FileOutput{Request: r, Path: out.Path}, nil
}

// fileOn files a request on an object — a goal, a feature, a use case asked
// to be worked on: the object carries it beside its own lifecycle, with a plan,
// and becomes a node a release's plan can hold (network-node), unless it
// already is one.
func (s *Service) fileOn(ctx context.Context, in api.FileInput, splitFrom string) (api.FileOutput, error) {
	if err := checkAsk("File", in.Kind, in.Priority, in.Deliverables, keptBySplit(in, splitFrom)); err != nil {
		return api.FileOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.FileOutput{}, err
	}
	if err := checkRequester(ctx, cwm, in.By); err != nil {
		return api.FileOutput{}, err
	}
	if err := ensureNetworkNode(ctx, cwm, in.On); err != nil {
		return api.FileOutput{}, err
	}
	o, err := loadObject(ctx, cwm, in.On)
	if err != nil {
		return api.FileOutput{}, err
	}
	if hasRequest(o) {
		// One open request per object; the next rung of its work (a feature
		// specified, then built) refiles the one that ended.
		if o, err = refile(ctx, cwm, o, filed(in, splitFrom), in.By); err != nil {
			return api.FileOutput{}, err
		}
	} else {
		fields, err := capabilityMap(filed(in, splitFrom))
		if err != nil {
			return api.FileOutput{}, err
		}
		out, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: in.On, Capability: api.CapRequest, Fields: fields})
		if err != nil {
			return api.FileOutput{}, err
		}
		o = out.Object
	}
	for kind, to := range map[string]string{api.EdgeSplitFrom: splitFrom, api.EdgeResolves: in.Resolves, api.EdgeAbout: in.About} {
		if to == "" || to == in.On {
			continue
		}
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: in.On, To: to, Kind: cwmapi.EdgeType(kind)}); err != nil {
			return api.FileOutput{}, fmt.Errorf("requests.File: %s %s: %w", kind, to, err)
		}
		if o, err = load(ctx, cwm, in.On); err != nil {
			return api.FileOutput{}, err
		}
	}
	for _, d := range in.DependsOn {
		if _, err := s.Depend(ctx, api.DependInput{ID: in.On, On: d}); err != nil {
			return api.FileOutput{}, fmt.Errorf("requests.File: depends on %s: %w", d, err)
		}
	}
	if o, err = planIntake(ctx, cwm, in.On, in.By, "filed"); err != nil {
		return api.FileOutput{}, err
	}
	return api.FileOutput{Request: toRequest(o)}, nil
}

// filed is the request group a filing writes.
func filed(in api.FileInput, splitFrom string) api.RequestFields {
	return api.RequestFields{
		Requester: in.By, Details: in.Details, Deliverables: in.Deliverables, BudgetUSD: in.BudgetUSD,
		Estimate: in.Estimate, Kind: in.Kind, Priority: in.Priority, GateRequirement: in.GateRequirement,
		FiledAt: time.Now().UTC().Format(time.RFC3339Nano), Source: in.Source, Review: in.Review,
	}
}

// Split replaces a request by its parts — one per module it affects, one per
// unmet goal, or one to route it — each filed new, keeping the request's
// requester and dependencies and naming it (split_from); the request is then
// dropped, naming its parts. A split into one part is a move: the part keeps
// the request's deliverables (verified ones stay verified) unless it names its
// own; the parts of a wider split each carry their own. The parts are checked
// before anything is written.
func (s *Service) Split(ctx context.Context, in api.SplitInput) (api.SplitOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.SplitOutput{}, err
	}
	if len(in.Parts) == 0 {
		return api.SplitOutput{}, invalid("Split: at least one part")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.SplitOutput{}, err
	}
	// Any demand splits: a request, a feature, a goal — what it asks is its
	// request's when it carries one.
	o, err := loadObject(ctx, cwm, in.ID)
	if err != nil {
		return api.SplitOutput{}, err
	}
	if planEnded(planState(o)) || nodeEnded(o) {
		return api.SplitOutput{}, invalid("Split: %s has ended (%s)", in.ID, nodeState(o))
	}
	var was api.RequestFields
	if hasRequest(o) {
		was = fieldsOf(o)
	}
	inputs := make([]api.FileInput, len(in.Parts))
	for i, p := range in.Parts {
		forms := 0
		for _, set := range []bool{p.Module != "", p.On != "", p.Sub} {
			if set {
				forms++
			}
		}
		if forms != 1 {
			return api.SplitOutput{}, invalid("Split: part %d names a module, an object to file on, or sub — one of the three", i+1)
		}
		if p.Sub && strings.TrimSpace(p.Title) == "" {
			return api.SplitOutput{}, invalid("Split: part %d is a new %s under %s: it needs a title", i+1, o.Type, in.ID)
		}
		f := api.FileInput{
			Module: p.Module, On: p.On, Title: p.Title, Details: p.Details, Deliverables: p.Deliverables,
			Estimate: p.Estimate, Kind: p.Kind, Priority: p.Priority,
			DependsOn: dependsOn(o), By: was.Requester,
		}
		if f.Title == "" && f.On == "" {
			f.Title = titleOf(o)
		}
		if f.Details == "" {
			f.Details = was.Details
		}
		if f.Kind == "" {
			f.Kind = was.Kind
		}
		if f.Priority == "" {
			f.Priority = was.Priority
		}
		if f.By.ID == "" {
			f.By = in.By
		}
		if err := checkAsk("Split", f.Kind, f.Priority, f.Deliverables, was.Deliverables); err != nil {
			return api.SplitOutput{}, err
		}
		inputs[i] = f
	}
	// A move keeps what was asked: filed unverified (a filing is never
	// verified), the verified ones are written back once the part is filed.
	var moved []api.Deliverable
	if len(inputs) == 1 && len(inputs[0].Deliverables) == 0 && len(was.Deliverables) > 0 {
		moved = slices.Clone(was.Deliverables)
		asked := make([]api.Deliverable, len(moved))
		for i, d := range moved {
			asked[i] = api.Deliverable{Title: d.Title, Check: d.Check}
		}
		inputs[0].Deliverables = asked
	}
	out := api.SplitOutput{}
	var ids []string
	for i, f := range inputs {
		if in.Parts[i].Sub {
			sub, err := s.subOf(ctx, cwm, o, f.Title)
			if err != nil {
				return out, fmt.Errorf("requests.Split %s: part %d: %w", in.ID, i+1, err)
			}
			f.On = sub
		}
		made, err := s.file(ctx, f, in.ID)
		if err != nil {
			return out, fmt.Errorf("requests.Split %s: part %d: %w", in.ID, i+1, err)
		}
		if moved != nil && slices.ContainsFunc(moved, func(d api.Deliverable) bool { return d.Verified }) {
			if made.Request, err = keepVerified(ctx, cwm, made.Request.ID, moved); err != nil {
				return out, fmt.Errorf("requests.Split %s: part %d: %w", in.ID, i+1, err)
			}
		}
		out.Parts = append(out.Parts, made.Request)
		ids = append(ids, made.Request.ID)
	}
	note := "split into " + strings.Join(ids, ", ")
	if in.Note != "" {
		note += " — " + in.Note
	}
	// The request stays, split: its requester follows it, and it ends when its
	// parts have (the planner ends it).
	if _, has := planOf(o); !has {
		if o, err = writePlan(ctx, cwm, o, api.PlanFields{}, false); err != nil {
			return out, err
		}
	}
	parent, err := planTransition(ctx, cwm, in.ID, api.PlanSplit, "split", in.By, note)
	if err != nil {
		return out, err
	}
	out.Split = toRequest(parent)
	// Its own steps leave the queue now, not at some later settle: its parts
	// carry its work (inside a collection, the collection's own settle does).
	if !collecting(ctx) {
		if err := settleRequest(ctx, in.ID, in.By); err != nil {
			return out, err
		}
	}
	return out, nil
}

// keepVerified writes a moved request's deliverables, verification included,
// onto the part it moved to.
func keepVerified(ctx context.Context, cwm cwmapi.CWMService, id string, deliverables []api.Deliverable) (api.Request, error) {
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return api.Request{}, err
	}
	f := fieldsOf(o)
	f.Deliverables = deliverables
	saved, err := saveRequest(ctx, o, f)
	if err != nil {
		return api.Request{}, err
	}
	return toRequest(saved), nil
}

// capabilityMap is a field-group as the map ObjectAddCapability takes.
func capabilityMap(v any) (map[string]any, error) {
	var o cwmapi.Object
	if err := o.SetCapability("x", v); err != nil {
		return nil, err
	}
	return o.Capabilities["x"], nil
}

// checkLinks refuses an `about` or an `on` that names no object, before
// anything is written — they take an object's id, never free text.
func (s *Service) checkLinks(ctx context.Context, in api.FileInput) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	for field, id := range map[string]string{"about": in.About, "on": in.On} {
		if id == "" {
			continue
		}
		if _, err := loadObject(ctx, cwm, strings.TrimSpace(id)); err != nil {
			return invalid("File: %s %q is not the id of an object of the project — name one (e.g. billing.UC-001) or leave %s empty", field, id, field)
		}
	}
	return nil
}

// subOf creates a part of o of o's own type under it — a sub-feature — and
// answers its id.
func (s *Service) subOf(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, title string) (string, error) {
	module, err := cwmapi.ModuleOfObjectID(o.ID)
	if err != nil {
		return "", err
	}
	sub := cwmapi.Object{Type: o.Type}
	sub.Parent = o.ID
	sub.Name = strings.TrimSpace(title)
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: sub})
	if err != nil {
		return "", fmt.Errorf("a new %s under %s: %w", o.Type, o.ID, err)
	}
	return out.Object.ID, nil
}

// Get reads one request.
func (s *Service) Get(ctx context.Context, in api.GetInput) (api.GetOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GetOutput{}, err
	}
	o, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.GetOutput{}, err
	}
	return api.GetOutput{Request: toRequest(o)}, nil
}

// List lists the requests of a module, in a state — both optional.
func (s *Service) List(ctx context.Context, in api.ListInput) (api.ListOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListOutput{}, err
	}
	all, err := listRequests(ctx, cwm, in.Module)
	if err != nil {
		return api.ListOutput{}, err
	}
	out := api.ListOutput{Requests: []api.Request{}}
	for _, o := range all {
		if in.State != "" && stateOf(o) != in.State {
			continue
		}
		if in.Requester != "" && fieldsOf(o).Requester.ID != in.Requester {
			continue
		}
		out.Requests = append(out.Requests, toRequest(o))
	}
	return out, nil
}

// listRequests lists the objects carrying a request, of a module or all.
func listRequests(ctx context.Context, cwm cwmapi.CWMService, module string) ([]cwmapi.Object, error) {
	out, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Capability: api.CapRequest})
	if err != nil {
		return nil, err
	}
	sort.Slice(out.Objects, func(i, j int) bool { return out.Objects[i].ID < out.Objects[j].ID })
	return out.Objects, nil
}

// Assess records intake's assessment on a request that has not ended: its
// kind, priority and estimate — only what is given changes — and the want it
// serves (a serves edge; refused for an object that wants nothing). A request
// whose template changes a module and that serves no want is answered a
// proposal — the wants it could serve — of which nothing is written.
func (s *Service) Assess(ctx context.Context, in api.AssessInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	if err := checkAsk("Assess", in.Kind, in.Priority, nil, nil); err != nil {
		return api.RequestOutput{}, err
	}
	if in.Estimate < 0 {
		return api.RequestOutput{}, invalid("Assess: estimate %v is negative", in.Estimate)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if inc, _, err := loadAny(ctx, cwm, in.ID); err == nil && inc.Type == api.IncrementType && (!hasRequest(inc) || nodeEnded(inc)) {
		return assessIncrement(ctx, cwm, inc, in)
	}
	o, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if nodeEnded(o) {
		return api.RequestOutput{}, invalid("Assess: %s has ended (%s)", in.ID, stateOf(o))
	}
	serves := strings.TrimSpace(in.Serves)
	if serves != "" {
		if _, err := loadObject(ctx, cwm, serves); err != nil {
			return api.RequestOutput{}, invalid("Assess: serves %q is not the id of an object of the project", serves)
		}
		if !carriesWant(ctx, cwm, serves) {
			return api.RequestOutput{}, invalid("Assess: %s wants nothing — a request serves a goal, a feature or a use case", serves)
		}
	}
	// An increment given delivery goals is a demand by construction: its
	// proposal is the one place the person approves what delivers it, so no
	// assessment makes it direct work.
	if o.Type == api.IncrementType && len(releaseFields(o).Delivery) > 0 {
		if in.Kind != "" && in.Kind != api.KindDemand {
			return api.RequestOutput{}, invalid("Assess: %s carries delivery goals - it stays a demand (its proposal is approved once), not %s", in.ID, in.Kind)
		}
		if in.Depth == api.DepthQuick {
			return api.RequestOutput{}, invalid("Assess: %s carries delivery goals - its proposal is never skipped (depth quick)", in.ID)
		}
	}
	f := fieldsOf(o)
	if in.Kind != "" {
		f.Kind = in.Kind
	}
	// An increment keeps the priority it carries when an agent assesses it
	// again - the intake of a reconcile or a re-proposal: only a person
	// moves it.
	if pf, has := planOf(o); has && o.Type == api.IncrementType && pf.Priority != "" && in.By.Kind != "person" {
		in.Priority = pf.Priority
	}
	if in.Priority != "" {
		f.Priority = in.Priority
		// The planner orders by the plan's priority: it moves with the
		// request's, or a re-prioritised request would keep its old place.
		if pf, has := planOf(o); has {
			pf.Priority = in.Priority
			if err := o.SetCapability(api.CapPlan, pf); err != nil {
				return api.RequestOutput{}, err
			}
		}
	}
	if in.Estimate > 0 {
		f.Estimate = in.Estimate
	}
	if in.Depth != "" {
		if !slices.Contains(api.Depths, in.Depth) {
			return api.RequestOutput{}, invalid("Assess: depth %q is not one of %v", in.Depth, api.Depths)
		}
		f.Depth = in.Depth
	}
	if in.Review != "" {
		if in.Review != api.ReviewAgent && in.Review != api.ReviewPerson {
			return api.RequestOutput{}, invalid("Assess: review %q is agent or person", in.Review)
		}
		f.Review = in.Review
	}
	if err := o.SetCapability(api.CapRequest, f); err != nil {
		return api.RequestOutput{}, err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return api.RequestOutput{}, err
	}
	o = out.Object
	if in.Priority != "" {
		if err := reprioritise(ctx, o.ID, in.Priority); err != nil {
			return api.RequestOutput{}, err
		}
		if o.Type == api.IncrementType {
			// The work it holds is pulled at its priority too.
			if err := settleAll(ctx, PlanningActor); err != nil {
				return api.RequestOutput{}, err
			}
		}
	}
	if in.Review != "" {
		if err := reresolveReviews(ctx, o); err != nil {
			return api.RequestOutput{}, err
		}
		if err := settleRequest(ctx, o.ID, PlanningActor); err != nil {
			return api.RequestOutput{}, err
		}
	}
	if serves != "" && serves != o.ID && !slices.ContainsFunc(o.Edges, func(e cwmapi.ObjectEdge) bool { return e.Kind == api.EdgeServes && e.To == serves }) {
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: o.ID, To: serves, Kind: cwmapi.EdgeType(api.EdgeServes)}); err != nil {
			return api.RequestOutput{}, fmt.Errorf("requests.Assess: serves %s: %w", serves, err)
		}
		if o, err = load(ctx, cwm, o.ID); err != nil {
			return api.RequestOutput{}, err
		}
		// What it serves may be an increment's: its open to-dos take that
		// increment's labels now, not at the next unrelated settle - and its
		// review the reviewer its release or stream asks.
		if err := reresolveReviews(ctx, o); err != nil {
			return api.RequestOutput{}, err
		}
		if err := settleRequest(ctx, o.ID, PlanningActor); err != nil {
			return api.RequestOutput{}, err
		}
	}
	res := api.RequestOutput{Request: toRequest(o)}
	if tmpl, err := templateFor(ctx, o); err == nil && changesModule(tmpl) && servedOf(ctx, cwm, o) == "" && !directWork(ctx, cwm, o) {
		res.Proposal = proposeServes(ctx, cwm, o)
	}
	return res, nil
}

// Ask parks a new request on one question to its requester: the question is
// filed in the inbox, and the request parked on it (reason question); Answered
// wakes it. A request that cannot be parked leaves no question behind.
// withdraw dismisses a question Ask filed but could not link, as
// planification's own act: the rollback of its own filing, never a person's
// answer — so an agent's session asking does not leave it pending (the inbox
// refuses an agent's dismissal of a person's question).
func withdraw(ctx context.Context, inbox inboxapi.InboxService, id string) {
	own := cause.WithCause(ctx, cause.Cause{Kind: cause.KindReaction, ID: "planification:withdraw@" + id, Depth: cause.DepthFrom(ctx) + 1})
	if _, err := inbox.Dismiss(own, inboxapi.DismissInput{ID: id}); err != nil {
		slog.Warn("planification: a question Ask could not link stays in the inbox", "question", id, "error", err)
	}
}

func (s *Service) Ask(ctx context.Context, in api.AskInput) (api.RequestOutput, error) {
	if strings.TrimSpace(in.Question) == "" {
		return api.RequestOutput{}, invalid("Ask: question required")
	}
	if err := checkActor(in.By); err != nil {
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
	if nodeEnded(o) {
		return api.RequestOutput{}, invalid("Ask: %s has ended (%s)", in.ID, stateOf(o))
	}
	return askRequester(ctx, cwm, o, in)
}

// Answered wakes what asked the inbox question `entry` once it is answered.
// A person's to-do asked there takes the answer as its result. A request that
// asked its requester gets the question and its answer added to its details,
// and its intake planned again, to evaluate it with the answer. A pending
// question wakes nothing.
func (s *Service) Answered(ctx context.Context, entry string) ([]string, error) {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return nil, err
	}
	got, err := inbox.Get(ctx, inboxapi.GetInput{ID: entry})
	if err != nil {
		return nil, err
	}
	e := got.Entry
	if e.Status == "" || e.Status == inboxapi.StatusPending {
		return nil, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return nil, err
	}
	asked, err := incoming(ctx, cwm, entry, api.EdgeAskedIn)
	if err != nil {
		return nil, err
	}
	answer := e.DecisionNote
	if answer == "" {
		answer = e.Status
	}
	var woken []string
	// A person's to-do asked there: their answer is its result.
	if todo, err := todoAnswered(ctx, entry, e, answer); err != nil {
		return woken, err
	} else if todo != "" {
		woken = append(woken, todo)
	}
	for _, id := range asked {
		o, err := load(ctx, cwm, id)
		if err != nil {
			continue // not a request: another's ask (a gate)
		}
		// An ended request takes no answer (: one still open does
		// whatever its plan says - an intake that asked reports its to-do
		// blocked and leaves the plan ready).
		st := stateOf(o)
		if planEnded(st) || uncounted(st) {
			continue
		}
		if err := takeAnswer(ctx, cwm, o, e.Title, answer); err != nil {
			return woken, err
		}
		woken = append(woken, o.ID)
	}
	return woken, nil
}

// takeAnswer gives a request that asked its requester the answer: the
// question and its answer added to its details, its intake planned again when
// it waits idle, and the step that asked and reported itself blocked back in
// the queue.
func takeAnswer(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, question, answer string) error {
	st := stateOf(o)
	f := fieldsOf(o)
	qa := "Q: " + question + "\nA: " + answer
	if f.Details != "" {
		qa = f.Details + "\n\n" + qa
	}
	f.Details = qa
	if err := o.SetCapability(api.CapRequest, f); err != nil {
		return err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
		return fmt.Errorf("answer %s: %w", o.ID, err)
	}
	if st == api.PlanIdle {
		if _, err := planIntake(ctx, cwm, o.ID, FilingActor, "answered: "+answer); err != nil {
			return fmt.Errorf("answer %s: %w", o.ID, err)
		}
	}
	// The step that asked and reported itself blocked comes back too.
	if err := resumeAsked(ctx, o.ID, "answered: "+answer); err != nil {
		return fmt.Errorf("answer %s: %w", o.ID, err)
	}
	return nil
}

// resumeAsked sends back to the queue the to-dos of a request left blocked on
// the question just answered: the step that asked
// takes the answer from the request's details.
func resumeAsked(ctx context.Context, request, note string) error {
	todos, err := listTodos(ctx, request)
	if err != nil {
		return err
	}
	for _, t := range todos {
		if !notStarted(t) || stopStatus(t) != schedapi.StatusBlocked {
			continue
		}
		clearStop(&t)
		if _, err := put(ctx, t); err != nil {
			return err
		}
	}
	return settleRequest(ctx, request, FilingActor)
}

// ensureNetworkNode gives an object the network-node capability - what a
// depends-on edge joins - unless it already carries it.
func ensureNetworkNode(ctx context.Context, cwm cwmapi.CWMService, id string) error {
	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: id})
	if err != nil {
		return err
	}
	if slices.ContainsFunc(eff.Capabilities, func(c cwmapi.CapabilityRef) bool { return c.ID == api.CapNetworkNode }) {
		return nil
	}
	_, err = cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: id, Capability: api.CapNetworkNode})
	return err
}

// Depend orders a network node (a request or a step) after another. CWM does not check object edges for
// cycles, so this does: a request may not end up waiting on itself.
func (s *Service) Depend(ctx context.Context, in api.DependInput) (api.RequestOutput, error) {
	if in.ID == "" || in.On == "" {
		return api.RequestOutput{}, invalid("Depend: id and on required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	o, err := loadNode(ctx, cwm, in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if _, err := loadNode(ctx, cwm, in.On); err != nil {
		return api.RequestOutput{}, fmt.Errorf("requests.Depend: on %s: %w", in.On, err)
	}
	// depends-on joins network nodes: what it orders is one - an increment
	// kept without the capability gets it here.
	for _, id := range []string{in.ID, in.On} {
		if err := ensureNetworkNode(ctx, cwm, id); err != nil {
			return api.RequestOutput{}, err
		}
	}
	if o, err = loadNode(ctx, cwm, in.ID); err != nil {
		return api.RequestOutput{}, err
	}
	if slices.Contains(dependsOn(o), in.On) {
		// Already ordered: its to-dos are settled again all the same, so an
		// edge given twice never leaves a step held on what has ended.
		if err := settleNode(ctx, o); err != nil {
			return api.RequestOutput{}, err
		}
		return api.RequestOutput{Request: toRequest(o)}, nil
	}
	if path, cycle, err := reaches(ctx, cwm, in.On, in.ID); err != nil {
		return api.RequestOutput{}, err
	} else if cycle {
		return api.RequestOutput{}, invalid("Depend: %s depending on %s closes a cycle (%s)", in.ID, in.On, strings.Join(append([]string{in.ID}, path...), " → "))
	}
	o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeDependsOn, To: in.On})
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return api.RequestOutput{}, err
	}
	// What it waits on now holds back its to-dos not started - an
	// increment's, and the work of what it holds.
	if err := settleNode(ctx, o); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(out.Object)}, nil
}

// assessIncrement sets the priority of an increment whose proposal has ended,
// or which carries no request part yet (an increment made before proposals):
// its plan part carries the priority settle gives the work it holds - added
// when missing - whatever its plan's state. Only a priority is an increment's
// to set here; a kind or a depth belongs to a live request.
func assessIncrement(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, in api.AssessInput) (api.RequestOutput, error) {
	if in.Kind != "" || in.Depth != "" || in.Serves != "" {
		return api.RequestOutput{}, invalid("Assess: %s is an increment whose proposal has ended or that has none: only its priority is set", o.ID)
	}
	if in.Priority == "" {
		return api.RequestOutput{}, invalid("Assess: %s: give the increment's priority", o.ID)
	}
	f, has := planOf(o)
	f.Priority = in.Priority
	saved, err := writePlan(ctx, cwm, o, f, has) // the plan keeps its state: only its priority moves
	if err != nil {
		return api.RequestOutput{}, err
	}
	if err := settleAll(ctx, PlanningActor); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(saved)}, nil
}

// settleNode settles a request's to-dos again after its order or priority
// changed - every to-do for an increment, whose priority and dependencies
// reach the work of what it holds.
func settleNode(ctx context.Context, o cwmapi.Object) error {
	if o.Type == api.IncrementType {
		return settleAll(ctx, PlanningActor)
	}
	return settleRequest(ctx, o.ID, PlanningActor)
}

// reaches walks what each node waits on (waitedOn: its depends-on, its open
// follow-ups and parts) from `from` and reports whether it reaches `to`, with
// the path it took.
func reaches(ctx context.Context, cwm cwmapi.CWMService, from, to string) ([]string, bool, error) {
	seen := map[string]bool{}
	var walk func(id string) ([]string, bool, error)
	walk = func(id string) ([]string, bool, error) {
		if id == to {
			return []string{id}, true, nil
		}
		if seen[id] {
			return nil, false, nil
		}
		seen[id] = true
		// Only the new edge's two ends must be ordered nodes (Depend checks
		// them): a node on the way - a feature carrying no request, never
		// planned or ended - is walked through its own edges, and one gone
		// is a leaf.
		o, _, err := loadAny(ctx, cwm, id)
		if errors.Is(err, cwmapi.ErrNotFound) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		for _, next := range waitedOn(ctx, cwm, o) {
			path, ok, err := walk(next)
			if err != nil || ok {
				return append([]string{id}, path...), ok, err
			}
		}
		// A review a trigger raised holds the to-dos of the work it is
		// about (held_by): that work cannot end before it has - one more
		// wait, named as the hold it is.
		for _, holder := range holdersOf(ctx, cwm, o) {
			path, ok, err := walk(holder)
			if err != nil || ok {
				if len(path) > 0 {
					path[0] = "held by the review of " + path[0]
				}
				return append([]string{id}, path...), ok, err
			}
		}
		return nil, false, nil
	}
	return walk(from)
}

// holdersOf is the open requests that hold one of a request's to-dos not
// started (a review trigger's held_by): the request cannot end before them.
func holdersOf(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) []string {
	if !hasRequest(o) {
		return nil
	}
	todos, err := todosOf(ctx, o.ID)
	if err != nil {
		return nil
	}
	var out []string
	for _, t := range todos {
		if !notStarted(t) {
			continue
		}
		for _, r := range t.Packet.HeldBy {
			if slices.Contains(out, r) {
				continue
			}
			if h, err := loadObject(ctx, cwm, r); err == nil && !nodeEnded(h) {
				out = append(out, r)
			}
		}
	}
	return out
}

// waitedOn is what a node cannot end before: its depends-on targets, then its
// open follow-ups and split parts - a parent ends only once they have, so a
// follow-up ordered after work that leads back to its parent closes a cycle
// as surely as a depends-on edge does.
func waitedOn(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) []string {
	out := dependsOn(o)
	for _, kind := range []string{api.EdgeFollowsUp, api.EdgeSplitFrom} {
		children, err := incoming(ctx, cwm, o.ID, kind)
		if err != nil {
			continue
		}
		for _, c := range children {
			if x, err := loadNode(ctx, cwm, c); err == nil && !nodeEnded(x) {
				out = append(out, c)
			}
		}
	}
	return out
}

// Ensure gives the project its sys-planification module (idempotent).
func (s *Service) Ensure(ctx context.Context, _ api.EnsureInput) (api.EnsureOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.EnsureOutput{}, err
	}
	id, created, err := ensureModule(ctx, cwm)
	if err != nil {
		return api.EnsureOutput{}, err
	}
	return api.EnsureOutput{Module: id, Created: created}, nil
}

// PlanModule gives a module just created the planned capability when it was
// created plain: every capability it carries itself is one its type composes.
// A module created with a domain capability of its own - an étude de
// l'existant, a dossier d'architecture - is its domain's, and planning shows
// nothing on it. No domain is named: a module's own capabilities say it.
func (s *Service) PlanModule(ctx context.Context, in cwmapi.HookCall) (api.PlanModuleOutput, error) {
	o := in.Object
	if o == nil || in.Point != cwmapi.HookOnCreate || levelOf(ctx, o.Type) != cwmapi.CapModule {
		return api.PlanModuleOutput{}, nil
	}
	if _, has := o.Capabilities[api.CapPlanned]; has {
		return api.PlanModuleOutput{Planned: true}, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PlanModuleOutput{}, err
	}
	t, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: o.Type})
	if err != nil {
		return api.PlanModuleOutput{}, err
	}
	for capability := range o.Capabilities {
		if !slices.Contains(t.ObjectType.Capabilities, capability) {
			return api.PlanModuleOutput{}, nil
		}
	}
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: o.ID, Capability: api.CapPlanned}); err != nil {
		return api.PlanModuleOutput{}, err
	}
	return api.PlanModuleOutput{Planned: true}, nil
}

// plannedWork are the object types whose presence in a module says its work
// is planned here.
var plannedWork = []string{"request", "goal", "feature"}

// PlanModules gives the planned capability to every module that already has
// planning work - a request, a goal or a feature of its own, or code packages
// - and does not carry it yet: the modules a project held before the
// capability existed. Idempotent; a module with none of them (an étude de
// l'existant, a dossier d'architecture) is left as it is.
func (s *Service) PlanModules(ctx context.Context, _ api.EnsureInput) (api.PlanModulesOutput, error) {
	out := api.PlanModulesOutput{Planned: []string{}}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	worked := map[string]bool{}
	for _, t := range plannedWork {
		got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: t})
		if err != nil {
			return out, err
		}
		for _, o := range got.Objects {
			if m, err := cwmapi.ModuleOfObjectID(o.ID); err == nil {
				worked[m] = true
			}
		}
	}
	// Every module, whatever its type: the module level capability says it
	//
	for _, capability := range []string{cwmapi.CapModule} {
		mods, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: capability})
		if err != nil {
			return out, err
		}
		for _, m := range mods.Objects {
			if _, has := m.Capabilities[api.CapPlanned]; has {
				continue
			}
			pkgs, _ := m.Capabilities["code"]["code_packages"].([]any)
			if !worked[m.ID] && len(pkgs) == 0 {
				continue
			}
			if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: m.ID, Capability: api.CapPlanned}); err != nil {
				return out, err
			}
			out.Planned = append(out.Planned, m.ID)
		}
	}
	return out, nil
}

// ensureModule creates sys-planification when the project has none.
// systemModuleDescription is what sys-planification is, as made.
const systemModuleDescription = "Planification's operational space: requests filed before intake routes them to the module they target (committed)."

func ensureModule(ctx context.Context, cwm cwmapi.CWMService) (string, bool, error) {
	created, err := cwmapi.EnsureSystemModule(ctx, cwm, api.SystemModule, systemModuleDescription)
	if err != nil {
		return "", false, err
	}
	return api.SystemModule, created, nil
}

// Answer answers a question: the answer is recorded on the request, and its
// plan is done.
func (s *Service) Answer(ctx context.Context, in api.AnswerInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	if strings.TrimSpace(in.Text) == "" {
		return api.RequestOutput{}, invalid("Answer: text required")
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
	if f.Kind != api.KindQuestion {
		return api.RequestOutput{}, invalid("Answer: %s is a %q request; only a question is answered", in.ID, f.Kind)
	}
	if nodeEnded(o) {
		return api.RequestOutput{}, invalid("Answer: %s has ended (%s)", in.ID, stateOf(o))
	}
	f.Answer = &api.Answer{Text: strings.TrimSpace(in.Text), By: in.By, At: time.Now().UTC().Format(time.RFC3339)}
	if err := o.SetCapability(api.CapRequest, f); err != nil {
		return api.RequestOutput{}, err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
		return api.RequestOutput{}, err
	}
	if _, err := NewPlans().Finish(ctx, api.FinishInput{ID: in.ID, By: in.By, Note: "answered"}); err != nil {
		return api.RequestOutput{}, err
	}
	if o, err = load(ctx, cwm, in.ID); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(o)}, nil
}

// FixCriterion files the fix of a failed acceptance criterion in one action:
// on the feature it proves, into the open increment holding that feature, its
// title and details naming the criterion and its failure.
func (s *Service) FixCriterion(ctx context.Context, in api.FixCriterionInput) (api.FileOutput, error) {
	id := strings.TrimSpace(in.Criterion)
	if id == "" {
		return api.FileOutput{}, invalid("FixCriterion: criterion required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.FileOutput{}, err
	}
	ac, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return api.FileOutput{}, err
	}
	feature := edgeTo(ac, api.EdgeServes)
	if feature == "" {
		feature = ac.Parent
	}
	f, _, err := loadAny(ctx, cwm, feature)
	if err != nil || !notIncrementWork(f) {
		return api.FileOutput{}, invalid("FixCriterion: %s proves no goal or feature (%s)", id, feature)
	}
	// Its last verdict: the criterion's verdict-<id> beside it.
	status, evidence := "", ""
	module, short := splitID(id)
	if v, _, err := loadAny(ctx, cwm, module+".verdict-"+short); err == nil {
		if c, ok := v.Capability("verdict"); ok {
			status, _ = c["status"].(string)
			evidence, _ = c["evidence"].(string)
		}
	}
	if status != "fail" {
		if status == "" {
			status = "not run"
		}
		return api.FileOutput{}, invalid("FixCriterion: %s has not failed (its last verdict: %s) - nothing to fix", id, status)
	}
	inc := takerOf(ctx, cwm, f)
	if inc == "" {
		return api.FileOutput{}, invalid("FixCriterion: no increment holds %s - file the fix on it (requests file --on %s --kind fix)", feature, feature)
	}
	io, _, err := loadAny(ctx, cwm, inc)
	if err != nil {
		return api.FileOutput{}, err
	}
	if io.Type != api.IncrementType {
		return api.FileOutput{}, invalid("FixCriterion: %s targets the %s %s, not an increment - file the fix on it (requests file --on %s --kind fix)", feature, io.Type, inc, feature)
	}
	if releaseState(io) != api.IncrementOpen {
		return api.FileOutput{}, invalid("FixCriterion: %s's increment %s is %s, not open - file the fix on it (requests file --on %s --kind fix)", feature, inc, releaseState(io), feature)
	}
	// Its fix is filed once: an open one about the criterion is answered.
	if about, _ := incoming(ctx, cwm, id, api.EdgeAbout); len(about) > 0 {
		for _, r := range about {
			if ro, err := loadObject(ctx, cwm, r); err == nil && hasRequest(ro) && fieldsOf(ro).Kind == api.KindFix && !planEnded(planState(ro)) {
				// Answered, in this increment and serving the feature.
				if edgeTo(ro, api.EdgeTargets) != inc {
					if _, err := joinIncrement(ctx, cwm, r, inc, in.By); err != nil {
						return api.FileOutput{}, err
					}
				}
				if err := addEdge(ctx, cwm, r, api.EdgeServes, feature); err != nil {
					return api.FileOutput{}, err
				}
				g, err := s.Get(ctx, api.GetInput{ID: r})
				return api.FileOutput{Request: g.Request}, err
			}
		}
	}
	details := "The acceptance criterion " + id + " - " + titleOf(ac) + " - failed its last check."
	if strings.TrimSpace(evidence) != "" {
		details += "\n\n```\n" + strings.TrimSpace(evidence) + "\n```"
	}
	// A request of its own - its title naming the criterion - about the
	// criterion, serving the feature, joining the increment.
	fm, _ := splitID(feature)
	out, err := s.File(ctx, api.FileInput{Module: fm, About: id, Kind: api.KindFix, Increment: inc,
		Title: id + " fails: " + titleOf(ac), Details: details, By: in.By})
	if err != nil {
		return out, err
	}
	if err := addEdge(ctx, cwm, out.Request.ID, api.EdgeServes, feature); err != nil {
		return out, fmt.Errorf("FixCriterion: %s was filed but does not serve %s: %w", out.Request.ID, feature, err)
	}
	return out, nil
}

// splitID is an object id's module and the rest (-> billing, AC-001).
func splitID(id string) (string, string) {
	if i := strings.Index(id, "."); i > 0 {
		return id[:i], id[i+1:]
	}
	return "", id
}

// levelOf is the level an object type sits at - project or module - read
// from the level capability it carries, never from its name
// empty below them or when the type is unknown.
func levelOf(ctx context.Context, typ string) string {
	cwm, err := cwmOf(ctx)
	if err != nil || typ == "" {
		return ""
	}
	got, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: typ})
	if err != nil {
		return ""
	}
	return got.ObjectType.Level()
}

// ranIncrement is whether an increment was run: it carries a request whose
// plan was not dropped. A dropped one - an approval filed before any run, or
// a run withdrawn - leaves it runnable again.
func ranIncrement(o cwmapi.Object) bool {
	return hasRequest(o) && planState(o) != api.PlanDropped
}
