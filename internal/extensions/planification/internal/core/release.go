// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Releases implements api.ReleaseService over the Object API. A release is a
// `release` object; its targets are the `targets` edges pointing at it; a
// module release's project release is its `ships-in` edge.
type Releases struct{}

// NewReleases returns the release service.
func NewReleases() *Releases { return &Releases{} }

var _ api.ReleaseService = (*Releases)(nil)

// loadAny reads any object in full.
func loadAny(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, string, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, "", err
	}
	o, err := getObject(ctx, cwm, module, id)
	if err != nil {
		return cwmapi.Object{}, "", err
	}
	return o, module, nil
}

// loadRelease reads what carries the release capability: a release, or an
// increment (which shares its targets, scope and grade).
func loadRelease(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, error) {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	if o.Type != api.ReleaseType && o.Type != api.IncrementType {
		return cwmapi.Object{}, invalid("%s is a %q, not a release", id, o.Type)
	}
	return o, nil
}

// releaseLike reports an object carrying the release capability's role: a
// release or an increment.
func releaseLike(o cwmapi.Object) bool {
	return o.Type == api.ReleaseType || o.Type == api.IncrementType
}

// lifeState is an object's lifecycle state — for a `request`, which has no
// lifecycle of its own, its request capability's; empty when it has neither.
func lifeState(o cwmapi.Object) string {
	if l, ok := o.Lifecycle(); ok {
		return l.State
	}
	if hasRequest(o) {
		return stateOf(o)
	}
	return ""
}

func releaseState(o cwmapi.Object) string {
	// Its own lifecycle only: an increment carrying its proposal (a request)
	// is open while that request runs.
	if l, ok := o.Lifecycle(); ok && l.State != "" {
		return l.State
	}
	if o.Type == api.IncrementType {
		return api.IncrementOpen
	}
	return api.ReleasePlanned
}

func releaseFields(o cwmapi.Object) api.ReleaseFields {
	f, _, _ := cwmapi.DecodeCapability[api.ReleaseFields](o, api.CapRelease)
	return f
}

func edgeTo(o cwmapi.Object, kind string) string {
	for _, e := range o.Edges {
		if e.Kind == kind {
			return e.To
		}
	}
	return ""
}

func toRelease(o cwmapi.Object) api.Release {
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	return api.Release{
		ID: o.ID, Module: module, Name: o.Name, State: releaseState(o),
		Project: module == api.SystemModule, ShipsIn: edgeTo(o, api.EdgeShipsIn), Stream: edgeTo(o, api.EdgeInStream), ReleaseFields: releaseFields(o),
	}
}

// titleOf is what a target is read by: its name.
func titleOf(o cwmapi.Object) string {
	return o.Name
}

func toTarget(o cwmapi.Object, via string) api.Target {
	st := lifeState(o)
	return api.Target{ID: o.ID, Type: o.Type, Title: titleOf(o), State: st, Done: slices.Contains(targetDoneStates, st), Via: via}
}

// A target counts by how its own lifecycle ends, whatever its type: done -
// or a decision in force (applies) - is done; dropped, superseded, retired
// (no longer in force) and a request split (its parts carry the work, each
// counted where it belongs) count in neither done nor total.
var (
	targetDoneStates  = []string{api.TargetDone, "applies"}
	targetOutOfCounts = []string{api.StateDropped, "superseded", "retired", api.PlanSplit}
)

// goalType is the goal's object type: a direction, never done in its own
// workflow.
const goalType = "goal"

// goalDelivered: a goal shipped by an increment is delivered when at least
// one feature serves it and every feature serving it that counts is done -
// never by its request's plan, which a goal added by a delta also carries
func goalDelivered(ctx context.Context, cwm cwmapi.CWMService, id string) (bool, error) {
	ids, err := incoming(ctx, cwm, id, api.EdgeServes)
	if err != nil {
		return false, err
	}
	features := 0
	for _, fid := range ids {
		f, _, err := loadAny(ctx, cwm, fid)
		if err != nil || f.Type != "feature" {
			continue
		}
		st := lifeState(f)
		if uncounted(st) {
			continue
		}
		if !slices.Contains(targetDoneStates, st) {
			return false, nil
		}
		features++
	}
	return features > 0, nil
}

// targetState is the state a target is counted and shown by: its own -
// lifeState - except a goal's, which is done once delivered by its features
// and otherwise its own workflow's state (its initial one when it has none),
// never its request's plan.
func targetState(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) string {
	if o.Type != goalType {
		return lifeState(o)
	}
	if l, ok := o.Lifecycle(); ok && l.State == api.StateDropped {
		return l.State
	}
	if done, err := goalDelivered(ctx, cwm, o.ID); err == nil && done {
		return api.TargetDone
	}
	if l, ok := o.Lifecycle(); ok && l.State != "" {
		return l.State
	}
	return goalInitial
}

// goalInitial is the goal workflow's initial state: a goal with no
// lifecycle is in it.
const goalInitial = "idea"

// isFixRequest: the object is a request of kind fix - a correction, which a
// frozen release still takes.
func isFixRequest(ctx context.Context, cwm cwmapi.CWMService, id string) bool {
	o, _, err := loadAny(ctx, cwm, id)
	return err == nil && hasRequest(o) && fieldsOf(o).Kind == api.KindFix
}

// uncounted: a target in this state ships nothing and holds nothing back.
func uncounted(state string) bool { return slices.Contains(targetOutOfCounts, state) }

// incoming lists the objects pointing at id by a kind of edge, sorted.
func incoming(ctx context.Context, cwm cwmapi.CWMService, id, kind string) ([]string, error) {
	// A read-only view asks the same edges of an object many times: once.
	c, cached := ctx.Value(readCacheKey{}).(*readCache)
	key := id + "|" + kind
	if cached {
		c.mu.Lock()
		r, hit := c.incoming[key]
		c.mu.Unlock()
		if hit {
			return append([]string(nil), r.ids...), r.err
		}
	}
	ids, err := incomingRead(ctx, cwm, id, kind)
	if cached {
		c.mu.Lock()
		if c.incoming == nil {
			c.incoming = map[string]incomingEdges{}
		}
		c.incoming[key] = incomingEdges{ids: ids, err: err}
		c.mu.Unlock()
	}
	return append([]string(nil), ids...), err
}

// incomingEdges is one object's incoming edges of a kind, kept for a view.
type incomingEdges struct {
	ids []string
	err error
}

func incomingRead(ctx context.Context, cwm cwmapi.CWMService, id, kind string) ([]string, error) {
	out, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: id, Kind: cwmapi.EdgeType(kind), Direction: cwmapi.DirectionIn})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range out.Edges {
		if e.To == id && !slices.Contains(ids, e.From) {
			ids = append(ids, e.From)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// setFields writes a release's field-group.
func setFields(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, f api.ReleaseFields) (cwmapi.Object, error) {
	if err := o.SetCapability(api.CapRelease, f); err != nil {
		return cwmapi.Object{}, err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return out.Object, nil
}

// moveRelease transitions a release with its frozen scope set to `scope` — the
// field written first, the state last (the transition is the commit); a
// refused transition puts the field back.
func moveRelease(ctx context.Context, id, to string, by cwmapi.Actor, note string, scope func(cwmapi.Object) ([]string, error)) (api.ReleaseOutput, error) {
	if err := checkActor(by); err != nil {
		return api.ReleaseOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	o, err := loadRelease(ctx, cwm, id)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	f := releaseFields(o)
	was := f.FrozenScope
	if scope != nil {
		if f.FrozenScope, err = scope(o); err != nil {
			return api.ReleaseOutput{}, err
		}
		if !slices.Equal(was, f.FrozenScope) {
			if _, err := setFields(ctx, cwm, o, f); err != nil {
				return api.ReleaseOutput{}, err
			}
		}
	}
	out, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: to, By: by, Note: note})
	if err != nil {
		if scope != nil && !slices.Equal(was, f.FrozenScope) {
			if now, lerr := loadRelease(ctx, cwm, id); lerr == nil {
				f.FrozenScope = was
				_, _ = setFields(ctx, cwm, now, f)
			}
		}
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(out.Object)}, nil
}

// open says whether a release — or an increment — still takes changes to its
// scope.
func open(state string) bool { return state == api.ReleasePlanned || state == api.IncrementOpen }

// Plan creates a release, planned, in its module's planning space — the
// project's in sys-planification.
func (r *Releases) Plan(ctx context.Context, in api.PlanInput) (api.ReleaseOutput, error) {
	version := strings.TrimSpace(in.Version)
	if version == "" {
		return api.ReleaseOutput{}, invalid("Plan: version required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	module := in.Module
	if module == "" {
		if module, _, err = ensureModule(ctx, cwm); err != nil {
			return api.ReleaseOutput{}, err
		}
	} else if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: module}); err != nil {
		return api.ReleaseOutput{}, fmt.Errorf("releases.Plan: module %s: %w", module, err)
	}
	existing, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Type: api.ReleaseType})
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	for _, o := range existing.Objects {
		if releaseFields(o).Version == version {
			return api.ReleaseOutput{}, invalid("Plan: %s already has release %s (%s)", module, version, o.ID)
		}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = version
	}
	o := cwmapi.Object{Type: api.ReleaseType}
	o.Parent = module
	o.Name = name
	if in.BudgetUSD < 0 {
		return api.ReleaseOutput{}, invalid("Plan: budget_usd is not negative")
	}
	if err := checkLevel("Plan", in.Level); err != nil {
		return api.ReleaseOutput{}, err
	}
	if err := o.SetCapability(api.CapRelease, api.ReleaseFields{Version: version, Date: in.Date, BudgetUSD: in.BudgetUSD, Level: in.Level}); err != nil {
		return api.ReleaseOutput{}, err
	}
	if in.ShipsIn != "" {
		if err := checkProjectRelease(ctx, cwm, module, in.ShipsIn); err != nil {
			return api.ReleaseOutput{}, err
		}
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeShipsIn, To: in.ShipsIn})
	}
	if in.Stream != "" {
		// Its version follows the stream's scheme.
		if err := checkInStream(ctx, cwm, o, in.Stream); err != nil {
			return api.ReleaseOutput{}, err
		}
		if why, err := noRoomInStream(ctx, cwm, in.Stream); err != nil {
			return api.ReleaseOutput{}, err
		} else if why != "" {
			return api.ReleaseOutput{}, invalid("Plan: %s", why)
		}
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeInStream, To: in.Stream})
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(out.Object), Path: out.Path}, nil
}

// checkProjectRelease refuses a ships-in that is not a module release into an
// open project release.
func checkProjectRelease(ctx context.Context, cwm cwmapi.CWMService, module, project string) error {
	if module == api.SystemModule {
		return invalid("a project release ships in nothing; only a module release ships in one")
	}
	p, err := loadRelease(ctx, cwm, project)
	if err != nil {
		return fmt.Errorf("ships in %s: %w", project, err)
	}
	if p.Type != api.ReleaseType {
		return invalid("%s is an increment: a release ships in a release", project)
	}
	if pm, _ := cwmapi.ModuleOfObjectID(project); pm != api.SystemModule {
		return invalid("%s is a module release; a module release ships in a project release", project)
	}
	if st := releaseState(p); !open(st) {
		return invalid("%s is %s: its scope takes no more module releases (a person unfreezes it first)", project, st)
	}
	return nil
}

// Get reads one release.
func (r *Releases) Get(ctx context.Context, in api.GetInput) (api.ReleaseOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	o, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(o)}, nil
}

// List lists releases of a module, in a state — both optional.
func (r *Releases) List(ctx context.Context, in api.ListInput) (api.ListReleasesOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListReleasesOutput{}, err
	}
	all, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module, Type: api.ReleaseType})
	if err != nil {
		return api.ListReleasesOutput{}, err
	}
	out := api.ListReleasesOutput{Releases: []api.Release{}}
	for _, o := range all.Objects {
		if in.State == "" || releaseState(o) == in.State {
			out.Releases = append(out.Releases, toRelease(o))
		}
	}
	sort.Slice(out.Releases, func(i, j int) bool { return out.Releases[i].ID < out.Releases[j].ID })
	return out, nil
}

// retarget aims an object at an increment or a release ("" takes it out): a
// request's one `targets` edge, set or removed; module work is taken by the
// holder (retake). It is the one writer of both edges.
func retarget(ctx context.Context, cwm cwmapi.CWMService, id, release string) (api.TargetOutput, error) {
	o, module, err := loadAny(ctx, cwm, id)
	if err != nil {
		return api.TargetOutput{}, err
	}
	if releaseLike(o) {
		return api.TargetOutput{}, invalid("%s is a %s: it ships in a release (ShipIn), it targets nothing", id, o.Type)
	}
	was := takerOf(ctx, cwm, o)
	if was == release {
		return api.TargetOutput{ID: id, Release: release}, nil
	}
	if was != "" && release == "" {
		if p, err := loadRelease(ctx, cwm, was); err == nil && releaseState(p) == api.ReleaseReleased {
			return api.TargetOutput{}, invalid("%s shipped in %s, which is released", id, was)
		}
	}
	if !ownsTarget(o) {
		// Module work is listed by its holder (rule).
		if _, err := retake(ctx, cwm, o, release); err != nil {
			return api.TargetOutput{}, err
		}
		out := api.TargetOutput{ID: id, Release: release}
		if was != "" && was != release {
			out.Replaced = was
		}
		return out, settleAll(ctx, PlanningActor)
	}
	edges := o.Edges[:0:0]
	for _, e := range o.Edges {
		if e.Kind != api.EdgeTargets {
			edges = append(edges, e)
		}
	}
	if release != "" {
		edges = append(edges, cwmapi.ObjectEdge{Kind: api.EdgeTargets, To: release})
	}
	o.Edges = edges
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
		return api.TargetOutput{}, err
	}
	out := api.TargetOutput{ID: id, Release: release}
	if was != "" {
		out.Replaced = was
	}
	// The to-dos of what it holds follow it: their labels and rank.
	if err := settleAll(ctx, PlanningActor); err != nil {
		return out, err
	}
	return out, nil
}

// Target aims a piece of work at a release, replacing the release it targeted
// (an object ships once). A frozen release takes no new target but a fix.
func (r *Releases) Target(ctx context.Context, in api.TargetInput) (api.TargetOutput, error) {
	if in.ID == "" || in.Release == "" {
		return api.TargetOutput{}, invalid("Target: id and release required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TargetOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.Release)
	if err != nil {
		return api.TargetOutput{}, err
	}
	if st := releaseState(rel); !open(st) && !(st == api.ReleaseFrozen && isFixRequest(ctx, cwm, in.ID)) {
		return api.TargetOutput{}, invalid("Target: %s is %s and takes no new target but a fix — a person unfreezes it first", in.Release, st)
	}
	if rel.Type == api.ReleaseType {
		if o, _, err := loadAny(ctx, cwm, in.ID); err == nil && notIncrementWork(o) {
			return api.TargetOutput{}, invalid("Target: %s is a %s - a release holds increments and direct work: aim it at an increment that ships in %s (increments target)", in.ID, o.Type, in.Release)
		}
	}
	return retarget(ctx, cwm, in.ID, in.Release)
}

// Untarget takes a piece of work out of its release (descoping is allowed while
// frozen; a shipped release keeps its work).
func (r *Releases) Untarget(ctx context.Context, in api.UntargetInput) (api.TargetOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TargetOutput{}, err
	}
	return retarget(ctx, cwm, in.ID, "")
}

// ShipIn ships a module release in an open project release.
func (r *Releases) ShipIn(ctx context.Context, in api.ShipInInput) (api.ReleaseOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	o, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	if o.Type == api.IncrementType {
		return api.ReleaseOutput{}, invalid("ShipIn: %s is an increment - ship it in a release with increments ShipIn", in.ID)
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	if err := checkProjectRelease(ctx, cwm, module, in.In); err != nil {
		return api.ReleaseOutput{}, err
	}
	if releaseState(o) == api.ReleaseDropped {
		return api.ReleaseOutput{}, invalid("ShipIn: %s is dropped", in.ID)
	}
	edges := o.Edges[:0:0]
	for _, e := range o.Edges {
		if e.Kind != api.EdgeShipsIn {
			edges = append(edges, e)
		}
	}
	o.Edges = append(edges, cwmapi.ObjectEdge{Kind: api.EdgeShipsIn, To: in.In})
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	if err := settleAll(ctx, PlanningActor); err != nil {
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(out.Object)}, nil
}

// Freeze records the release's scope — its current targets — and freezes it.
func (r *Releases) Freeze(ctx context.Context, in api.MoveInput) (api.ReleaseOutput, error) {
	// The release point: what rides along in its stream's next release - the
	// stream's increments done and assigned to none - is in it from now on
	// (decision). Taken in once the freeze passed: a
	// refused freeze assigns nothing.
	var along []cwmapi.Object
	if cwm, err := cwmOf(ctx); err == nil {
		if ro, err := loadRelease(ctx, cwm, in.ID); err == nil && releaseState(ro) == api.ReleasePlanned {
			if sm := edgeTo(ro, api.EdgeInStream); sm != "" {
				if next, ok := nextPlannedRelease(ctx, cwm, sm); ok && next == in.ID {
					along = ridingAlong(ctx, cwm, sm)
				}
			}
		}
	}
	out, err := r.freeze(ctx, in)
	if err != nil {
		return out, err
	}
	for _, inc := range along {
		if _, err := shipIn(ctx, api.ShipInInput{ID: inc.ID, In: in.ID}, false); err != nil {
			return out, fmt.Errorf("Freeze %s: take in %s: %w", in.ID, inc.ID, err)
		}
	}
	return out, nil
}

// freeze moves the release to frozen, recording its targets as its scope.
func (r *Releases) freeze(ctx context.Context, in api.MoveInput) (api.ReleaseOutput, error) {
	return moveRelease(ctx, in.ID, api.ReleaseFrozen, in.By, in.Note, func(o cwmapi.Object) ([]string, error) {
		cwm, err := cwmOf(ctx)
		if err != nil {
			return nil, err
		}
		return workOf(ctx, cwm, o.ID)
	})
}

// Unfreeze sends a frozen release back to planned, its frozen scope cleared —
// the workflow lets only a person do it.
func (r *Releases) Unfreeze(ctx context.Context, in api.MoveInput) (api.ReleaseOutput, error) {
	return moveRelease(ctx, in.ID, api.ReleasePlanned, in.By, in.Note, func(cwmapi.Object) ([]string, error) { return nil, nil })
}

// Release ships it; the on_transition hook refuses while a target is not done.
func (r *Releases) Release(ctx context.Context, in api.MoveInput) (api.ReleaseOutput, error) {
	return moveRelease(ctx, in.ID, api.ReleaseReleased, in.By, in.Note, nil)
}

// Drop drops a release.
func (r *Releases) Drop(ctx context.Context, in api.MoveInput) (api.ReleaseOutput, error) {
	out, err := moveRelease(ctx, in.ID, api.ReleaseDropped, in.By, in.Note, nil)
	if err != nil {
		return out, err
	}
	// A dropped release keeps no increment: each waits in its stream's
	// backlog, for the next release to take it. Those planned for it keep
	// their planned-for edge as the record: they read planned for nothing,
	// saying their release was dropped (toIncrement).
	if err := freeIncrements(ctx, in.ID); err != nil {
		return out, err
	}
	return out, nil
}

// freeIncrements takes every increment shipping in a release out of it.
func freeIncrements(ctx context.Context, release string) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	ids, err := incoming(ctx, cwm, release, api.EdgeShipsIn)
	if err != nil {
		return err
	}
	freed := false
	for _, id := range ids {
		o, _, err := loadAny(ctx, cwm, id)
		if err != nil || o.Type != api.IncrementType {
			continue
		}
		if err := unship(ctx, cwm, o); err != nil {
			return err
		}
		freed = true
	}
	if freed {
		return settleAll(ctx, PlanningActor)
	}
	return nil
}

// unship takes an increment out of the release it ships in.
func unship(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) error {
	o = withEdge(o, api.EdgeShipsIn, "")
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	_, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	return err
}

// scopeOf is what a release ships: its own targets, then each release or
// increment shipping in it (a module release in a project release; an
// increment in any release) and its targets.
func scopeOf(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) (api.ScopeOutput, error) {
	out := api.ScopeOutput{Release: toRelease(rel), Targets: []api.Target{}}
	own, err := workOf(ctx, cwm, rel.ID)
	if err != nil {
		return out, err
	}
	frozen := releaseFields(rel).FrozenScope
	listed := map[string]bool{}
	add := func(ids []string, via string) error {
		for _, id := range ids {
			// Once each: a done increment keeps what it took, which a later
			// one of the same release may take again.
			if listed[id] {
				continue
			}
			listed[id] = true
			o, _, err := loadAny(ctx, cwm, id)
			if err != nil {
				return fmt.Errorf("target %s: %w", id, err)
			}
			t := toTarget(o, via)
			if o.Type == goalType {
				// A goal is never done by its own workflow: it is delivered
				// when the features serving it are.
				t.State = targetState(ctx, cwm, o)
				t.Done = t.State == api.TargetDone
			}
			out.Targets = append(out.Targets, t)
			// An ended target that ships nothing (dropped, superseded, split…)
			// is listed, counted in neither done nor total.
			if uncounted(t.State) {
				continue
			}
			out.Total++
			if t.Done {
				out.Done++
			}
		}
		return nil
	}
	if err := add(own, ""); err != nil {
		return out, err
	}
	if st := releaseState(rel); st == api.ReleaseFrozen || st == api.ReleaseReleased {
		for _, id := range own {
			// Only corrections enter a frozen release: a fix linked after
			// the freeze is in its scope.
			if !slices.Contains(frozen, id) && !isFixRequest(ctx, cwm, id) {
				out.OffScope = append(out.OffScope, id)
			}
		}
	}
	mods, err := incoming(ctx, cwm, rel.ID, api.EdgeShipsIn)
	if err != nil {
		return out, err
	}
	for _, id := range mods {
		m, err := loadRelease(ctx, cwm, id)
		if err != nil {
			return out, err
		}
		if m.Type == api.IncrementType {
			st := releaseState(m)
			out.Increments = append(out.Increments, api.Target{ID: m.ID, Type: m.Type, Title: titleOf(m), State: st, Done: st == api.IncrementDone})
		} else {
			out.Modules = append(out.Modules, toRelease(m))
		}
		theirs, err := workOf(ctx, cwm, id)
		if err != nil {
			return out, err
		}
		if err := add(theirs, id); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Scope answers what a release ships.
func (r *Releases) Scope(ctx context.Context, in api.GetInput) (api.ScopeOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ScopeOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ScopeOutput{}, err
	}
	return scopeOf(ctx, cwm, rel)
}

// Backlog is a module's work that targets no release and is not finished: its
// requests and what it wants (the `want` capability).
func (r *Releases) Backlog(ctx context.Context, in api.BacklogInput) (api.BacklogOutput, error) {
	if in.Module == "" {
		return api.BacklogOutput{}, invalid("Backlog: module required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.BacklogOutput{}, err
	}
	seen := map[string]bool{}
	out := api.BacklogOutput{Items: []api.Target{}}
	for _, q := range []cwmapi.ObjectListInput{
		{Module: in.Module, Capability: api.CapRequest},
		{Module: in.Module, Capability: cwmapi.CapWant},
	} {
		list, err := cwm.ObjectList(ctx, q)
		if err != nil {
			return api.BacklogOutput{}, err
		}
		for _, o := range list.Objects {
			if seen[o.ID] || takerOf(ctx, cwm, o) != "" {
				continue
			}
			seen[o.ID] = true
			if st := targetState(ctx, cwm, o); st == api.TargetDone || st == api.StateDropped {
				continue
			}
			out.Items = append(out.Items, toTarget(o, ""))
		}
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].ID < out.Items[j].ID })
	return out, nil
}

// CheckTransition is the on_transition hook of a release, given the release as
// it is about to be: a freeze must carry its frozen scope (Freeze writes it); a
// release must have every target done and frozen with it, and every module
// release shipping in it released.
func (r *Releases) CheckTransition(ctx context.Context, in api.HookInput) (api.CheckReadyOutput, error) {
	if in.Object == nil || !releaseLike(*in.Object) {
		return api.CheckReadyOutput{Accept: true}, nil
	}
	rel := *in.Object
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.CheckReadyOutput{}, err
	}
	if rel.Type == api.IncrementType {
		return incrementCheck(ctx, cwm, rel)
	}
	switch releaseState(rel) {
	case api.ReleaseFrozen:
		targets, err := workOf(ctx, cwm, rel.ID)
		if err != nil {
			return api.CheckReadyOutput{}, err
		}
		if frozen := releaseFields(rel).FrozenScope; !slices.Equal(frozen, targets) {
			return api.CheckReadyOutput{Reason: fmt.Sprintf("the frozen scope %v is not its targets %v — freeze through ReleaseService.Freeze", frozen, targets)}, nil
		}
		// A release's plan is trusted only once its scope is ready to plan: its
		// goals decomposed, its features specified — their dependencies known —
		// its own and those of the increments shipping in it.
		scope := slices.Clone(targets)
		incs, err := incrementsIn(ctx, cwm, rel)
		if err != nil {
			return api.CheckReadyOutput{}, err
		}
		for _, inc := range incs {
			theirs, err := workOf(ctx, cwm, inc.ID)
			if err != nil {
				return api.CheckReadyOutput{}, err
			}
			scope = append(scope, theirs...)
		}
		var raw []string
		for _, id := range scope {
			o, _, err := loadAny(ctx, cwm, id)
			if err != nil {
				return api.CheckReadyOutput{}, err
			}
			if st := targetState(ctx, cwm, o); st == "dropped" || st == api.TargetDone {
				continue
			}
			gates, err := application.UseService[gatesapi.GatesService](ctx)
			if err != nil {
				return api.CheckReadyOutput{}, err
			}
			chk, err := gates.Check(ctx, gatesapi.CheckInput{ID: id, Gate: api.GateReadyToPlan})
			if err != nil {
				return api.CheckReadyOutput{}, err
			}
			if !chk.Checklist.Passes {
				var unmet []string
				for _, it := range chk.Checklist.Unmet() {
					unmet = append(unmet, it.Name)
				}
				raw = append(raw, fmt.Sprintf("%s (%s)", id, strings.Join(unmet, ", ")))
			}
		}
		if len(raw) > 0 {
			return api.CheckReadyOutput{Reason: "its scope is not ready to plan: " + strings.Join(raw, ", ")}, nil
		}
		if why, err := levelMismatch(ctx, cwm, rel); err != nil {
			return api.CheckReadyOutput{}, err
		} else if why != "" {
			return api.CheckReadyOutput{Reason: why}, nil
		}
	case api.ReleaseReleased:
		scope, err := scopeOf(ctx, cwm, rel)
		if err != nil {
			return api.CheckReadyOutput{}, err
		}
		var why []string
		for _, t := range scope.Targets {
			// A target that ships nothing (dropped, superseded, split…) holds
			// nothing back, as scopeOf counts it.
			if !t.Done && !uncounted(t.State) {
				state := t.State
				if state == "" {
					state = "no lifecycle"
				}
				why = append(why, fmt.Sprintf("%s is not done (%s)", t.ID, state))
			}
		}
		for _, id := range scope.OffScope {
			why = append(why, id+" was added after the freeze")
		}
		for _, m := range scope.Modules {
			if m.State != api.ReleaseReleased && m.State != api.ReleaseDropped {
				why = append(why, fmt.Sprintf("%s %s is %s", shippedLabel(m.ID), m.ID, m.State))
			}
		}
		for _, inc := range scope.Increments {
			if inc.State != api.IncrementDone && inc.State != api.IncrementDropped {
				why = append(why, fmt.Sprintf("%s %s is %s", shippedLabel(inc.ID), inc.ID, inc.State))
			}
		}
		if len(why) > 0 {
			return api.CheckReadyOutput{Reason: strings.Join(why, "; ")}, nil
		}
	}
	return api.CheckReadyOutput{Accept: true}, nil
}

// Lint is the lint hook of a release: a target added after its freeze; what
// judges it outside what targets it; a version off its stream's scheme; an
// open increment in no stream.
func (r *Releases) Lint(ctx context.Context, in api.HookInput) (api.LintOutput, error) {
	out := api.LintOutput{Status: "ok", Findings: []api.LintFinding{}}
	if in.Object == nil || !releaseLike(*in.Object) {
		return out, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.LintOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.Object.ID)
	if errors.Is(err, cwmapi.ErrNotFound) {
		return out, nil
	} else if err != nil {
		return api.LintOutput{}, err
	}
	scope, err := scopeOf(ctx, cwm, rel)
	if err != nil {
		return api.LintOutput{}, err
	}
	for _, id := range scope.OffScope {
		out.Findings = append(out.Findings, api.LintFinding{
			Rule: "target-after-freeze", Severity: "error",
			Message: fmt.Sprintf("%s targets %s, which is %s, and was not in its frozen scope — a person unfreezes the release to change its scope", id, rel.ID, releaseState(rel)),
		})
	}
	for _, t := range scope.Targets {
		if t.Via == "" && (t.Type == "goal" || t.Type == "feature") {
			out.Findings = append(out.Findings, api.LintFinding{Rule: "target-not-increment", Severity: "warn",
				Message: fmt.Sprintf("%s, a %s, targets %s directly: a release holds increments and direct work - aim it at an increment that ships in it", t.ID, t.Type, rel.ID)})
		}
	}
	if why, err := levelMismatch(ctx, cwm, rel); err != nil {
		return api.LintOutput{}, err
	} else if why != "" {
		out.Findings = append(out.Findings, api.LintFinding{Rule: "level-mismatch", Severity: "warn", Message: why})
	}
	if f := streamLint(ctx, cwm, rel); f != nil {
		out.Findings = append(out.Findings, *f)
	}
	if f := noStreamLint(rel); f != nil {
		out.Findings = append(out.Findings, *f)
	}
	outside, err := judgedOutside(ctx, cwm, rel)
	if err != nil {
		return api.LintOutput{}, err
	}
	if f := judgedOutsideFinding(rel.ID, outside); f != nil {
		out.Findings = append(out.Findings, *f)
	}
	if len(out.Findings) > 0 {
		out.Status = "findings"
	}
	return out, nil
}
