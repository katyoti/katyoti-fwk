// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// INCREMENTS — one sentence of user value holding the goals and features that
// make it. An increment carries the release capability: its targets, scope and
// grade are a release's (release.go), never a copy; it ships in at most one
// release, whose scope then holds its targets (through it, as a module
// release's); it is done once each of them is (the release-gate hook).

// Increments implements api.IncrementService.
type Increments struct{}

var _ api.IncrementService = (*Increments)(nil)

// NewIncrements returns the increment service.
func NewIncrements() *Increments { return &Increments{} }

func toIncrement(ctx context.Context, o cwmapi.Object) api.Increment {
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	inc := api.Increment{ID: o.ID, Module: module, Title: o.Name, Description: o.Description, State: releaseState(o), ShipsIn: edgeTo(o, api.EdgeShipsIn), PlannedFor: edgeTo(o, api.EdgePlannedFor), Stream: edgeTo(o, api.EdgeInStream), Delivery: releaseFields(o).Delivery,
		Held: heldIncrement(ctx, o.ID), DependsOn: edgesTo(o, api.EdgeDependsOn), Running: ranIncrement(o), Priority: incrementPriority(o), Moved: releaseFields(o).Moved,
		DeliveryReview: releaseFields(o).DeliveryReview, TestDelivery: releaseFields(o).TestDelivery}
	// A dropped release keeps its planned-for edges as the record: the
	// increment is planned for nothing and says why it is back in the backlog.
	if inc.PlannedFor != "" {
		if cwm, err := cwmOf(ctx); err == nil {
			if r, _, err := loadAny(ctx, cwm, inc.PlannedFor); err == nil && releaseState(r) == api.ReleaseDropped {
				inc.Returned = "its planned release " + titleOf(r) + " was dropped - plan it again"
				inc.PlannedFor = ""
			}
		}
	}
	return inc
}

// incrementStanding is where an increment stands (api.IncrementStanding), the
// state of the release it ships in read once it is done: shipped or done.
func incrementStanding(ctx context.Context, cwm cwmapi.CWMService, inc api.Increment, done, total int) string {
	in := api.IncrementStandingInput{Increment: inc, Done: done, Total: total}
	if inc.State == api.IncrementDone && inc.ShipsIn != "" {
		if r, _, err := loadAny(ctx, cwm, inc.ShipsIn); err == nil {
			in.ReleaseState = releaseState(r)
		}
	}
	return api.IncrementStanding(in)
}

// incrementPriority is an increment's priority: its plan's, the default when
// it has none.
func incrementPriority(o cwmapi.Object) string {
	if p, ok := planOf(o); ok && p.Priority != "" {
		return p.Priority
	}
	return api.DefaultPriority
}

// Summary answers where an increment stands, counted as its pane counts it -
// once per view carrying the read cache: a view that draws an increment's
// line and then its row (the deck's Overview) counts it once.
func (i *Increments) Summary(ctx context.Context, in api.GetInput) (api.IncrementSummaryOutput, error) {
	ctx = withReadCache(ctx) // one listing, one queue and one hold reader for every request
	c, _ := ctx.Value(readCacheKey{}).(*readCache)
	c.mu.Lock()
	got, ok := c.summaries[in.ID]
	c.mu.Unlock()
	if ok {
		return got.out, got.err
	}
	out, err := i.summary(ctx, in)
	c.mu.Lock()
	if c.summaries == nil {
		c.summaries = map[string]summaryRead{}
	}
	c.summaries[in.ID] = summaryRead{out: out, err: err}
	c.mu.Unlock()
	return out, err
}

// summaryRead is an increment's summary, read once for a view.
type summaryRead struct {
	out api.IncrementSummaryOutput
	err error
}

func (i *Increments) summary(ctx context.Context, in api.GetInput) (api.IncrementSummaryOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementSummaryOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementSummaryOutput{}, err
	}
	inc := toIncrement(ctx, o)
	out := api.IncrementSummaryOutput{ID: inc.ID, Title: inc.Title, State: inc.State, Held: inc.Held != nil, Priority: inc.Priority, Waits: []string{}}
	scope, err := scopeOf(ctx, cwm, o)
	if err != nil {
		return out, err
	}
	out.Done, out.Total = scope.Done, scope.Total
	w := requestsSummary(ctx, cwm, o, scope)
	out.Awaiting, out.Running, out.ToDo, out.Blocked, out.RequestsDone = w.awaiting, w.running, w.todo, w.blocked, w.done
	out.WaitingOn, out.Idle = w.waiting, w.idle
	out.Requests = w.total()
	if inc.State == api.IncrementDone {
		// Delivered: what is still open about what it holds is follow-up
		// work, never its own.
		out.Awaiting, out.Running, out.ToDo, out.Blocked, out.WaitingOn, out.Idle = 0, 0, 0, 0, 0, 0
	}
	if waits := incrementWaits(ctx, cwm, o); waits != nil {
		out.Waits = waits
	}
	out.Standing = incrementStanding(ctx, cwm, inc, scope.Done, scope.Total)
	return out, nil
}

// requestsSummary counts an increment's proposal and the requests serving
// or carried by what it holds - the pane's Work and progress.
func requestsSummary(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, scope api.ScopeOutput) workSummary {
	var w workSummary
	for _, r := range incrementRequestObjects(ctx, cwm, o, scope) {
		w.add(ctx, r)
	}
	return w
}

// incrementRequestObjects is every request an increment counts, once each:
// itself when it carries one (its proposal), then each target of its scope
// that is a request - a goal's or feature's own, or one filed straight into
// the increment - and the requests serving it. The summary and the pane read
// this one list: a delivery goal's requests on the
// pane are only those among it.
func incrementRequestObjects(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, scope api.ScopeOutput) []cwmapi.Object {
	var out []cwmapi.Object
	// The increment itself is not one of its requests, even when it carries
	// a plan (its proposal): its standing says where that is
	//
	seen := map[string]bool{o.ID: true}
	for _, t := range scope.Targets {
		serving, _ := incoming(ctx, cwm, t.ID, api.EdgeServes)
		for _, r := range append([]string{t.ID}, serving...) {
			if seen[r] {
				continue
			}
			ro, err := loadObject(ctx, cwm, r)
			if err != nil || !hasRequest(ro) {
				continue
			}
			seen[r] = true
			out = append(out, ro)
		}
	}
	return out
}

// Waits answers what an increment waits on.
func (i *Increments) Waits(ctx context.Context, in api.GetInput) (api.IncrementWaitsOutput, error) {
	ctx = withReadCache(ctx) // one listing, one queue and one hold reader for every request
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementWaitsOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementWaitsOutput{}, err
	}
	waits := incrementWaits(ctx, cwm, o)
	if waits == nil {
		waits = []string{}
	}
	return api.IncrementWaitsOutput{Waits: waits}, nil
}

// loadIncrement reads an increment.
func loadIncrement(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, error) {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	if o.Type != api.IncrementType {
		return cwmapi.Object{}, invalid("%s is a %q, not an increment", id, o.Type)
	}
	return o, nil
}

// shipsInto checks the release an increment ships in: a release (never an
// increment: one level), open.
func shipsInto(ctx context.Context, cwm cwmapi.CWMService, release string) error {
	r, err := loadRelease(ctx, cwm, release)
	if err != nil {
		return fmt.Errorf("ships in %s: %w", release, err)
	}
	if r.Type != api.ReleaseType {
		return invalid("%s is an increment: an increment ships in a release, never in an increment", release)
	}
	if st := releaseState(r); !open(st) {
		return invalid("%s is %s: it takes no more increments (a person unfreezes it first)", release, st)
	}
	return nil
}

// Create creates an increment, open.
func (i *Increments) Create(ctx context.Context, in api.CreateIncrementInput) (api.IncrementOutput, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return api.IncrementOutput{}, invalid("Create: its short title required")
	}
	if strings.TrimSpace(in.Stream) == "" && strings.TrimSpace(in.ShipsIn) == "" {
		// An increment is always created in its stream: named, or given by
		// the release it ships in.
		return api.IncrementOutput{}, invalid("Create: an increment is created in its stream - name the stream (stream) or the release it ships in (ships_in)")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	module := in.Module
	if module == "" && in.Stream != "" {
		// Created in the module of the stream it aims at.
		so, err := loadStream(ctx, cwm, in.Stream)
		if err != nil {
			return api.IncrementOutput{}, err
		}
		module, _ = cwmapi.ModuleOfObjectID(so.ID)
	}
	if module == "" {
		if module, _, err = ensureModule(ctx, cwm); err != nil {
			return api.IncrementOutput{}, err
		}
	} else if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: module}); err != nil {
		return api.IncrementOutput{}, fmt.Errorf("increments.Create: module %s: %w", module, err)
	}
	o := cwmapi.Object{Type: api.IncrementType}
	o.Parent, o.Name, o.Description = module, title, strings.TrimSpace(in.Description)
	if err := o.SetCapability(api.CapRelease, api.ReleaseFields{Breaking: in.Breaking}); err != nil {
		return api.IncrementOutput{}, err
	}
	// A node of the plan's network: it depends and is depended on.
	o.Capabilities[api.CapNetworkNode] = map[string]any{}
	if in.ShipsIn != "" {
		if err := shipsInto(ctx, cwm, in.ShipsIn); err != nil {
			return api.IncrementOutput{}, err
		}
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeShipsIn, To: in.ShipsIn})
		// It takes its release's stream when it names none; a release of
		// another stream is refused.
		if r, err := loadRelease(ctx, cwm, in.ShipsIn); err == nil {
			if sm := edgeTo(r, api.EdgeInStream); sm != "" {
				if in.Stream != "" && in.Stream != sm {
					return api.IncrementOutput{}, invalid("Create: %s is in stream %q, not %q: an increment ships in a release of its own stream", in.ShipsIn, sm, in.Stream)
				}
				in.Stream = sm
			}
		}
	}
	if in.Stream != "" {
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeInStream, To: in.Stream})
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		return api.IncrementOutput{}, err
	}
	created := out.Object
	if len(cleanGoals(in.Goals)) > 0 {
		// Its delivery goals: kept on it; nothing planned until a person runs it.
		if created, err = setDelivery(ctx, cwm, created, in.Goals, nil, in.By); err != nil {
			return api.IncrementOutput{}, err
		}
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, created), Path: out.Path}, nil
}

// Get reads one increment.
func (i *Increments) Get(ctx context.Context, in api.GetInput) (api.IncrementOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, o)}, nil
}

// List lists increments of a module, in a state — both optional.
func (i *Increments) List(ctx context.Context, in api.ListIncrementsInput) (api.ListIncrementsOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListIncrementsOutput{}, err
	}
	all, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module, Type: api.IncrementType})
	if err != nil {
		return api.ListIncrementsOutput{}, err
	}
	out := api.ListIncrementsOutput{Increments: []api.Increment{}}
	for _, o := range all.Objects {
		if in.State == "" || releaseState(o) == in.State {
			out.Increments = append(out.Increments, toIncrement(ctx, o))
		}
	}
	sort.Slice(out.Increments, func(a, b int) bool { return out.Increments[a].ID < out.Increments[b].ID })
	return out, nil
}

// Target aims a goal or a feature at an open increment - at once before it
// runs; on a running increment a new goal or feature is a small proposal
// holding only the addition, aimed once a person approves it
func (i *Increments) Target(ctx context.Context, in api.IncrementTargetInput) (api.TargetOutput, error) {
	if in.ID == "" || in.Increment == "" {
		return api.TargetOutput{}, invalid("Target: id and increment required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TargetOutput{}, err
	}
	if inc, err := loadIncrement(ctx, cwm, in.Increment); err == nil && open(releaseState(inc)) && hasRequest(inc) && !approvedAim(ctx, cwm, in) {
		if o, _, err := loadAny(ctx, cwm, in.ID); err == nil && notIncrementWork(o) && takerOf(ctx, cwm, o) != in.Increment {
			if open := openAddition(ctx, cwm, o.ID, inc.ID); open != "" {
				return api.TargetOutput{ID: in.ID, Release: takerOf(ctx, cwm, o), Proposed: open}, nil
			}
			id, err := proposeAddition(ctx, cwm, o, inc, in.By)
			return api.TargetOutput{ID: in.ID, Release: takerOf(ctx, cwm, o), Proposed: id}, err
		}
	}
	return i.aim(ctx, in)
}

// aim aims a goal or a feature at an open increment at once: Target before
// the increment runs, and an approved delta's operations.
func (i *Increments) aim(ctx context.Context, in api.IncrementTargetInput) (api.TargetOutput, error) {
	if in.ID == "" || in.Increment == "" {
		return api.TargetOutput{}, invalid("Target: id and increment required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TargetOutput{}, err
	}
	inc, err := loadIncrement(ctx, cwm, in.Increment)
	if err != nil {
		return api.TargetOutput{}, err
	}
	if st := releaseState(inc); !open(st) {
		return api.TargetOutput{}, invalid("Target: %s is %s and takes no new goal or feature", in.Increment, st)
	}
	o, _, err := loadAny(ctx, cwm, in.ID)
	if err != nil {
		return api.TargetOutput{}, err
	}
	if !releaseLike(o) && !carriesWant(ctx, cwm, in.ID) {
		return api.TargetOutput{}, invalid("Target: %s is a %s - an increment holds goals and features (what wants)", in.ID, o.Type)
	}
	return retarget(ctx, cwm, in.ID, in.Increment)
}

// Untarget takes a goal or a feature out of its increment.
func (i *Increments) Untarget(ctx context.Context, in api.UntargetInput) (api.TargetOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TargetOutput{}, err
	}
	return retarget(ctx, cwm, in.ID, "")
}

// ShipIn ships an increment in an open release, replacing the one it shipped in.
func (i *Increments) ShipIn(ctx context.Context, in api.ShipInInput) (api.IncrementOutput, error) {
	return shipIn(ctx, in, true)
}

// shipIn ships an increment in an open release; a person's ShipIn (next) is
// refused unless the release is its stream's next planned one (decision
// : increments are assigned to the next release only) -
// the stream's cut and a freeze taking the increments that ride along are not.
func shipIn(ctx context.Context, in api.ShipInInput, next bool) (api.IncrementOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if st := releaseState(o); st != api.IncrementOpen && st != api.IncrementDone {
		return api.IncrementOutput{}, invalid("ShipIn: %s is %s", in.ID, st)
	}
	if in.In == "" && next {
		// None named: its stream's next planned release (OD-014), found here.
		sm := edgeTo(o, api.EdgeInStream)
		if sm == "" {
			return api.IncrementOutput{}, invalid("ShipIn: %s is aimed at no stream: name the release", in.ID)
		}
		n, ok := nextPlannedRelease(ctx, cwm, sm)
		if !ok {
			return api.IncrementOutput{}, invalid("ShipIn: %s has no planned release to ship %s in", sm, in.ID)
		}
		in.In = n
	}
	if err := shipsInto(ctx, cwm, in.In); err != nil {
		// The freeze taking in what rode along ships into the release it just froze.
		r, rerr := loadRelease(ctx, cwm, in.In)
		if next || rerr != nil || releaseState(r) != api.ReleaseFrozen {
			return api.IncrementOutput{}, err
		}
	}
	relStream := ""
	if r, _, err := loadAny(ctx, cwm, in.In); err == nil {
		relStream = edgeTo(r, api.EdgeInStream)
		if why, err := beyondOpenTwo(ctx, cwm, r); err != nil {
			return api.IncrementOutput{}, err
		} else if why != "" {
			return api.IncrementOutput{}, invalid("ShipIn: %s", why)
		}
	}
	if relStream != "" {
		// On every path - a person's ship-in, a plan-for, a cut, a freeze -
		// an increment ships in a release of its own stream only
		// ; one in none takes the release's below.
		if own := edgeTo(o, api.EdgeInStream); own != "" && own != relStream {
			return api.IncrementOutput{}, invalid("ShipIn: %s is in stream %q, %s in %q: an increment ships in a release of its own stream", in.In, relStream, in.ID, own)
		}
	}
	if next && relStream != "" {
		if n, ok := nextPlannedRelease(ctx, cwm, relStream); ok && n != in.In {
			return api.IncrementOutput{}, invalid("ShipIn: %s is not the next release of %s (%s): an increment is assigned to its stream's next release only - a later one takes what its priority brings once it is next", in.In, relStream, n)
		}
	}
	edges := o.Edges[:0:0]
	for _, e := range o.Edges {
		// Shipped in a release, it is no longer only planned for one.
		if e.Kind != api.EdgeShipsIn && e.Kind != api.EdgePlannedFor {
			edges = append(edges, e)
		}
	}
	o.Edges = append(edges, cwmapi.ObjectEdge{Kind: api.EdgeShipsIn, To: in.In})
	if relStream != "" && edgeTo(o, api.EdgeInStream) == "" {
		// An increment in no stream takes its release's: never hidden from
		// the views that list increments by stream.
		o = withEdge(o, api.EdgeInStream, relStream)
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return api.IncrementOutput{}, err
	}
	// The to-dos of what it holds follow it to its release: their labels.
	if err := settleAll(ctx, PlanningActor); err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, out.Object)}, nil
}

// Unship takes an increment out of the release it ships in: it waits for
// another (its stream's next cut, or a person's ShipIn).
func (i *Increments) Unship(ctx context.Context, in api.GetInput) (api.IncrementOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if edgeTo(o, api.EdgeShipsIn) != "" {
		if err := unship(ctx, cwm, o); err != nil {
			return api.IncrementOutput{}, err
		}
		if err := settleAll(ctx, PlanningActor); err != nil {
			return api.IncrementOutput{}, err
		}
		if o, err = loadIncrement(ctx, cwm, in.ID); err != nil {
			return api.IncrementOutput{}, err
		}
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, o)}, nil
}

// Scope answers what an increment holds.
func (i *Increments) Scope(ctx context.Context, in api.GetInput) (api.ScopeOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ScopeOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.ScopeOutput{}, err
	}
	return scopeOf(ctx, cwm, o)
}

// Grade answers an increment's grade: a release's, over what it holds.
func (i *Increments) Grade(ctx context.Context, in api.GetInput) (api.GradeOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GradeOutput{}, err
	}
	if _, err := loadIncrement(ctx, cwm, in.ID); err != nil {
		return api.GradeOutput{}, err
	}
	return NewReleases().Grade(ctx, in)
}

// Done marks an increment done; the release-gate hook refuses it while one of
// its goals or features is not, and the gate usable-delivered while its
// delivery review has not passed - a person forces that
// gate with a note. Closing it is a person's (its
// workflow's done is guarded the same way): an agent is refused, told how the
// person closes it.
func (i *Increments) Done(ctx context.Context, in api.IncrementMoveInput) (api.IncrementOutput, error) {
	if in.By.Kind != cwmapi.ActorPerson {
		return api.IncrementOutput{}, invalid("closing %s takes a person: they approve its delivery in « À faire » (asked once everything it holds is done) or use « Mark it done » on its pane", in.ID)
	}
	return moveIncrement(ctx, in, api.IncrementDone)
}

// Drop drops an increment.
func (i *Increments) Drop(ctx context.Context, in api.IncrementMoveInput) (api.IncrementOutput, error) {
	return moveIncrement(ctx, in, api.IncrementDropped)
}

func moveIncrement(ctx context.Context, in api.IncrementMoveInput, to string) (api.IncrementOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if _, err := loadIncrement(ctx, cwm, in.ID); err != nil {
		return api.IncrementOutput{}, err
	}
	if in.Force {
		if err := checkActor(in.By); err != nil {
			return api.IncrementOutput{}, err
		}
		if in.By.Kind != cwmapi.ActorPerson || strings.TrimSpace(in.Note) == "" {
			return api.IncrementOutput{}, invalid("forcing %s takes a person and a note saying why", in.ID)
		}
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: in.ID, To: to, By: in.By, Note: in.Note, Force: true}); err != nil {
			return api.IncrementOutput{}, err
		}
	} else if _, err := moveRelease(ctx, in.ID, to, in.By, in.Note, nil); err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if to == api.IncrementDropped {
		// Dropped: its work ends with it.
		if err := EndDroppedIncrement(ctx, in.ID, to); err != nil {
			slog.Warn("planification: the work of a dropped increment was not ended", "increment", in.ID, "error", err)
		}
	}
	if to == api.IncrementDone {
		// Delivered: it rides along in its stream's next release, assigned or
		// not; an on-change stream it names makes its release.
		cutOnChange(ctx, cwm, o)
		if o, err = loadIncrement(ctx, cwm, in.ID); err != nil {
			return api.IncrementOutput{}, err
		}
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, o)}, nil
}

// incrementCheck is the release-gate hook of an increment, as it is about to
// be: done only once each goal and feature it holds is. A target ended dropped
// (a request superseded or given up by decision) holds nothing back.
func incrementCheck(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object) (api.CheckReadyOutput, error) {
	if releaseState(inc) != api.IncrementDone {
		return api.CheckReadyOutput{Accept: true}, nil
	}
	why, err := heldBack(ctx, cwm, inc)
	if err != nil {
		return api.CheckReadyOutput{}, err
	}
	if len(why) > 0 {
		return api.CheckReadyOutput{Reason: strings.Join(why, "; ")}, nil
	}
	return api.CheckReadyOutput{Accept: true}, nil
}

// heldBack says what holds an increment back from done: each goal or feature
// it holds that is not done.
func heldBack(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object) ([]string, error) {
	scope, err := scopeOf(ctx, cwm, inc)
	if err != nil {
		return nil, err
	}
	var why []string
	for _, t := range scope.Targets {
		if !t.Done && !uncounted(t.State) {
			state := t.State
			if state == "" {
				state = "no lifecycle"
			}
			why = append(why, fmt.Sprintf("%s is not done (%s)", t.ID, state))
		}
	}
	return why, nil
}

// shippedLabel names what ships in a release: an increment, or a module
// release.
func shippedLabel(id string) string {
	if strings.Contains(id, ".IN-") {
		return "increment"
	}
	return "module release"
}

// priorityForm sets an increment's priority.
type priorityForm struct {
	Priority string `json:"priority" jsonschema:"description=P0 now - P1 next - P2 later - P3 someday,enum=P0|P1|P2|P3,required"`
}

// incrementHoldForm puts an increment on hold: the reason is required.
type incrementHoldForm struct {
	Reason string `json:"reason" jsonschema:"description=Why it waits,required"`
}

// The increment verbs its pane runs.
const (
	incrementHoldURI   = "ktw-agents:increments/hold"
	incrementResumeURI = "ktw-agents:increments/resume"
	incrementRunURI    = "ktw-agents:increments/run"
	requestAssessURI   = "ktw-agents:requests/assess"
)

// targetForm is the pane's form adding a goal or feature to the increment.
type targetForm struct {
	ID string `json:"id" jsonschema:"description=The goal or feature to add: its id,required"`
}

// workSummary counts the requests of an increment by where they stand: what
// awaits a person, what runs, what is blocked, what is done - delivered once
// the increment is done and its release released.
type workSummary struct {
	awaiting, running, todo, blocked, done int
	waiting, idle                          int
	delivered                              bool
}

func (w *workSummary) add(ctx context.Context, o cwmapi.Object) {
	p, _ := planOf(o)
	switch {
	case uncounted(p.State):
		// Dropped or split: ships nothing itself, counted nowhere (its parts
		// are counted where they belong) - as the scope counts its targets.
	case planEnded(p.State):
		w.done++
	case p.State == api.PlanBlocked:
		w.blocked++
	case requestWaits(ctx, o, "") != "":
		w.awaiting++
	case stepRuns(ctx, o.ID):
		w.running++
	default:
		switch b, _ := openBucket(ctx, o); b {
		case openToDo:
			w.todo++
		case openWaits:
			w.waiting++
		default:
			w.idle++
		}
	}
}

func (w workSummary) total() int {
	return w.awaiting + w.running + w.todo + w.blocked + w.done + w.waiting + w.idle
}

// Where an open request stands when no step of it runs and no person is
// asked: « to do » only with a step an agent can take.
const (
	openToDo  = "todo"  // a step is ready for an agent
	openWaits = "waits" // it waits on another request, or a step on its previous one
	openIdle  = "idle"  // no step open, nothing awaited: it should end
)

// openState says where an open request stands - a step ready, waiting (on
// which requests, and why), or nothing left - from its to-dos and its
// depends-on edges. Why it waits is what the one reader says holds it
// (holdWords), never a to-do's wait text.
func openState(ctx context.Context, o cwmapi.Object) (bucket, why string, on []string) {
	bucket, on = openBucket(ctx, o)
	switch {
	case bucket == openToDo:
		return bucket, "", nil
	case bucket == openIdle:
		return bucket, "nothing left: no step open and nothing awaited - it should end", nil
	}
	if b, ok := holdOf(ctx, o); ok {
		return bucket, "waiting: " + holdWords(b), on
	}
	if len(on) > 0 {
		return bucket, "waiting on " + strings.Join(on, ", "), on
	}
	return bucket, "waiting: its previous step", nil
}

// openBucket is where an open request stands, without why: what the counts
// read (workSummary), which never ask the one reader.
func openBucket(ctx context.Context, o cwmapi.Object) (bucket string, on []string) {
	todos, _ := todosOf(ctx, o.ID)
	for _, t := range todos {
		if t.State == schedapi.StateReady && t.For != schedapi.ForPerson {
			return openToDo, nil
		}
	}
	if cwm, err := cwmOf(ctx); err == nil {
		// A dependency is open as the scheduler reads it (waitsOn): one
		// passed - ended, decided, done in its own lifecycle - or an
		// ancestor is awaited no more.
		for _, dep := range edgesTo(o, api.EdgeDependsOn) {
			if descendsFrom(ctx, cwm, o, dep) || dependencyPassed(ctx, cwm, dep, "", nil) {
				continue
			}
			if d, err := loadObject(ctx, cwm, dep); err == nil && uncounted(planState(d)) {
				continue
			}
			on = append(on, dep)
		}
	}
	if len(on) > 0 {
		return openWaits, on
	}
	for _, t := range todos {
		if t.State == schedapi.StateWaiting {
			return openWaits, nil
		}
	}
	return openIdle, nil
}

// Rename changes an increment's short title or its intent; an empty one is
// kept.
func (i *Increments) Rename(ctx context.Context, in api.RenameIncrementInput) (api.IncrementOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.IncrementOutput{}, err
	}
	title, description := strings.TrimSpace(in.Title), strings.TrimSpace(in.Description)
	if title == "" && description == "" {
		return api.IncrementOutput{}, invalid("Rename: a title or a description required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if _, err := loadIncrement(ctx, cwm, in.ID); err != nil {
		return api.IncrementOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: in.ID})
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o := got.Object
	if title != "" {
		o.Name = title
	}
	if description != "" {
		o.Description = description
	}
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Object: o, Revision: got.Revision})
	if err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, out.Object)}, nil
}

// requestFileURI files a request.
const requestFileURI = "ktw-agents:requests/file"

// joinForm is work filed straight into a running increment.
type joinForm struct {
	Title   string `json:"title"             jsonschema:"description=The work in one line,required"`
	Kind    string `json:"kind"              jsonschema:"description=fix: a correction - build: a small addition - analyse: a question,enum=fix|build|analyse,required"`
	Module  string `json:"module"            jsonschema:"description=The module it changes,required"`
	Details string `json:"details,omitempty" jsonschema:"description=What the one line leaves out"`
}

// incrementAddRecipe is the work of a goal or feature added to a running
// increment: a person approves the addition.
const incrementAddRecipe = "increment-add"

// proposeAddition files a small proposal on a running increment holding only
// one addition - aiming a goal or a feature at it - for a person's approval:
// a demand of depth quick whose delta is that one operation, joined to the
// increment (its card lists it), planned at once with no intake.
func proposeAddition(ctx context.Context, cwm cwmapi.CWMService, want, inc cwmapi.Object, by cwmapi.Actor) (string, error) {
	if by.ID == "" {
		by = PlanningActor
	}
	module, _ := cwmapi.ModuleOfObjectID(inc.ID)
	out, err := New().fileOnly(ctx, api.FileInput{Module: module, Kind: api.KindDemand, About: inc.ID,
		Title:   "Add " + titleOf(want) + " (" + want.ID + ") to " + titleOf(inc),
		Details: "A " + want.Type + " added to the running increment " + inc.ID + ": approve it to aim it at the increment, or send it back - nothing is aimed until then.",
		By:      by}, "")
	if err != nil {
		return "", err
	}
	id := out.Request.ID
	o, err := load(ctx, cwm, id)
	if err != nil {
		return id, err
	}
	f := fieldsOf(o)
	f.Depth = api.DepthQuick
	f.Delta = []api.DeltaOp{{Op: api.OpIncrementTarget, Object: want.ID, Increment: inc.ID,
		Why: "added while " + inc.ID + " runs: it takes it once a person approves"}}
	if _, err := saveRequest(ctx, o, f); err != nil {
		return id, err
	}
	if err := markJoined(ctx, cwm, o, inc.ID); err != nil {
		return id, err
	}
	_, err = NewWork().Plan(ctx, api.PlanWorkInput{Request: id, NoIntake: true, By: PlanningActor})
	return id, err
}

// applyAddition aims what an approved addition proposes; a sent-back one
// ends dropped, nothing aimed.
func applyAddition(ctx context.Context, cwm cwmapi.CWMService, proposal cwmapi.Object, approved bool, note string, by cwmapi.Actor) error {
	if !approved {
		_, err := NewPlans().Finish(ctx, api.FinishInput{ID: proposal.ID, Outcome: api.PlanDropped, By: by, Note: "the addition was sent back: " + firstLine(note)})
		return err
	}
	f := fieldsOf(proposal)
	var applied []api.Applied
	for n, op := range f.Delta {
		if op.Op != api.OpIncrementTarget {
			continue
		}
		if io, err := loadIncrement(ctx, cwm, op.Increment); err != nil || !open(releaseState(io)) {
			// Approved after the increment closed: nothing to aim.
			_, err := NewPlans().Finish(ctx, api.FinishInput{ID: proposal.ID, Outcome: api.PlanDropped, By: by,
				Note: "approved after " + op.Increment + " closed: nothing aimed"})
			return err
		}
		if _, err := NewIncrements().aim(ctx, api.IncrementTargetInput{ID: op.Object, Increment: op.Increment}); err != nil {
			return fmt.Errorf("the addition %s approved: %w", proposal.ID, err)
		}
		applied = append(applied, api.Applied{Op: n, ID: op.Object})
	}
	o, err := load(ctx, cwm, proposal.ID)
	if err != nil {
		return err
	}
	f = fieldsOf(o)
	f.Applied = mergeApplied(f.Applied, applied)
	_, err = saveRequest(ctx, o, f)
	return err
}

// approvedAim is whether the demand Target names holds this very aim in its
// delta: an approved delta's increment-target, applied, is aimed at once.
func approvedAim(ctx context.Context, cwm cwmapi.CWMService, in api.IncrementTargetInput) bool {
	if in.Demand == "" {
		return false
	}
	d, err := load(ctx, cwm, in.Demand)
	if err != nil || !hasRequest(d) {
		return false
	}
	if !demandApproved(ctx, d) {
		return false
	}
	f := fieldsOf(d)
	for _, op := range f.Delta {
		inc := resolveRef(op.Increment, f.Applied)
		if inc == "" && d.Type == api.IncrementType {
			inc = d.ID
		}
		if op.Op != api.OpIncrementTarget || inc != in.Increment {
			continue
		}
		obj := resolveRef(op.Object, f.Applied)
		if obj == in.ID {
			return true
		}
		// A #N this very round makes is not recorded applied yet: a goal or
		// feature that no increment holds yet is the one it makes.
		if isRef(obj) {
			if w, _, err := loadAny(ctx, cwm, in.ID); err == nil && notIncrementWork(w) && takerOf(ctx, cwm, w) == "" {
				return true
			}
		}
	}
	return false
}

// demandApproved is whether a demand's delta was approved: its apply to-do
// was released - ready, held or reported. Every step is planned up front, so
// an apply still waiting (on the approval) is not approved.
func demandApproved(ctx context.Context, d cwmapi.Object) bool {
	todos, err := todosOf(ctx, d.ID)
	if err != nil {
		return false
	}
	for _, t := range todos {
		if step := t.Packet.Step; (step == "apply" || strings.HasSuffix(step, "/apply")) && t.State != schedapi.StateWaiting {
			return true
		}
	}
	return false
}

// openAddition is an open proposal adding want to the increment, if one is
// filed already: a second addition answers it rather than filing another.
func openAddition(ctx context.Context, cwm cwmapi.CWMService, want, inc string) string {
	about, _ := incoming(ctx, cwm, inc, api.EdgeAbout)
	for _, r := range about {
		ro, err := loadObject(ctx, cwm, r)
		if err != nil || !hasRequest(ro) || planEnded(planState(ro)) {
			continue
		}
		for _, op := range fieldsOf(ro).Delta {
			if op.Op == api.OpIncrementTarget && op.Object == want && op.Increment == inc {
				return r
			}
		}
	}
	return ""
}
