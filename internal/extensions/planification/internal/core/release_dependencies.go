// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"slices"
	"sort"
	"strconv"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE RELEASE'S DEPENDENCIES: before starting agents
// the person sees what waits for what - each increment of the release a
// frame holding its goals, features and requests, every depends-on an arrow,
// the work outside the release that something waits on in a dotted frame
// saying where it is scheduled, and each increment ready to run, waiting or
// running. Computed, never set by hand.

// Readiness answers whether an increment can take an agent now.
func (i *Increments) Readiness(ctx context.Context, in api.GetInput) (api.IncrementReadiness, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementReadiness{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementReadiness{}, err
	}
	return readinessOf(ctx, cwm, o), nil
}

// readinessOf is an increment's readiness: running when an agent or a person
// holds one of its work's to-dos (a live claim - a lapsed one is nobody's),
// else waiting when anything it waits on is open, else ready to run. Nothing
// for an increment done or dropped.
func readinessOf(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) api.IncrementReadiness {
	if releaseState(o) != api.IncrementOpen {
		return api.IncrementReadiness{}
	}
	var held []string
	for _, w := range incrementWork(ctx, cwm, o) {
		todos, _ := todosOf(ctx, w.ID)
		for _, t := range todos {
			if t.State != schedapi.StateClaimed || t.Claim == nil {
				continue
			}
			if _, lapsed := claimLapsed(t, deckNow()); lapsed {
				continue
			}
			if id := t.Claim.By.ID; id != "" && !slices.Contains(held, id) {
				held = append(held, id)
			}
		}
	}
	if len(held) > 0 {
		return api.IncrementReadiness{State: api.ReadinessRunning, HeldBy: held}
	}
	if waits := incrementWaits(ctx, cwm, o); len(waits) > 0 {
		return api.IncrementReadiness{State: api.ReadinessWaiting, WaitsFor: waits}
	}
	return api.IncrementReadiness{State: api.ReadinessReady}
}

// readinessBadge says an increment's readiness in a word or two.
func readinessBadge(r api.IncrementReadiness) string {
	switch r.State {
	case api.ReadinessRunning:
		return "running: " + joinShort(r.HeldBy)
	case api.ReadinessWaiting:
		return "waiting: " + joinShort(r.WaitsFor)
	case api.ReadinessReady:
		return "ready to run"
	}
	return ""
}

func joinShort(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return xs[0] + " +" + strconv.Itoa(len(xs)-1)
}

// incrementWork is what an increment holds and what works on it: its goals
// and features, the requests serving or carried by each, its own proposal -
// each once, in that order.
func incrementWork(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) []cwmapi.Object {
	var out []cwmapi.Object
	seen := map[string]bool{}
	add := func(x cwmapi.Object) {
		if !seen[x.ID] {
			seen[x.ID] = true
			out = append(out, x)
		}
	}
	if hasRequest(o) {
		add(o)
	}
	scope, err := scopeOf(ctx, cwm, o)
	if err != nil {
		return out
	}
	for _, t := range scope.Targets {
		to, err := loadObject(ctx, cwm, t.ID)
		if err != nil {
			continue
		}
		add(to)
		serving, _ := incoming(ctx, cwm, t.ID, api.EdgeServes)
		for _, r := range serving {
			if ro, err := loadObject(ctx, cwm, r); err == nil && hasRequest(ro) {
				add(ro)
			}
		}
	}
	return out
}

// workStatus is how a piece of work stands, in the shared status words: a
// request by its plan and its to-dos (running when held, waiting on a
// person, blocked), a goal or feature by whether it is done.
func workStatus(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) string {
	if !hasRequest(o) || o.Type == api.IncrementType {
		if isDone(ctx, cwm, o) {
			return "done"
		}
		return "todo"
	}
	p, _ := planOf(o)
	switch {
	case p.State == api.PlanDone:
		return "done"
	case p.State == api.PlanDropped:
		return "dropped"
	case p.State == api.PlanBlocked:
		return "blocked"
	}
	todos, _ := todosOf(ctx, o.ID)
	for _, t := range todos {
		if t.State == schedapi.StateClaimed || t.State == schedapi.StateReported {
			return "running"
		}
	}
	if requestWaits(ctx, o, "") != "" {
		return "waiting"
	}
	return "todo"
}

// isDone says whether a goal, a feature or an increment is done - a goal
// once its features deliver it (targetState).
func isDone(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) bool {
	if o.Type == api.IncrementType {
		return releaseState(o) == api.IncrementDone
	}
	return targetState(ctx, cwm, o) == api.TargetDone
}

// DependenciesPage answers a release's Dependencies panel.
func (r *Releases) DependenciesPage(ctx context.Context, in api.GetInput) (render.Def, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	incs := carriedIncrements(ctx, cwm, rel)
	sort.SliceStable(incs, func(i, j int) bool { return incs[i].ID < incs[j].ID })
	g := dependencyGraph(ctx, cwm, incs)
	if len(g.boxes) == 0 {
		return render.Def{Kind: "page", Title: titleOf(rel), DSLTree: render.VBox(render.Text("No increment ships in this release yet: nothing waits for anything.").With("color", "dimmed"))}, nil
	}
	boxes := make([]any, len(g.boxes))
	for i, b := range g.boxes {
		boxes[i] = b
	}
	edges := make([]any, len(g.edges))
	for i, e := range g.edges {
		edges[i] = e
	}
	var ready []string
	for _, b := range g.boxes {
		if b["readiness"] == api.ReadinessReady {
			ready = append(ready, b["title"].(string))
		}
	}
	summary := "Nothing can take an agent now."
	if len(ready) > 0 {
		summary = "Ready to run: " + joinAll(ready) + "."
	}
	graph := render.Node{"kind": "graph", "id": "release-dependencies", "data": map[string]any{"nodes": boxes, "edges": edges},
		"open": render.NavigateTo("Open", "${item.id}", "")}
	return render.Def{Kind: "page", Title: titleOf(rel), DSLTree: render.VBox(render.Text(summary).With("size", "sm"), graph)}, nil
}

func joinAll(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

// depGraph is the Dependencies network: its boxes and its arrows.
type depGraph struct {
	boxes []map[string]any
	edges []map[string]any
}

// dependencyGraph builds the network of increments: a frame per increment
// with its readiness, its work inside, an arrow for every depends-on (from
// what is waited on to what waits) - and a dotted box for what is waited on
// outside them, saying where it is scheduled and how it stands.
func dependencyGraph(ctx context.Context, cwm cwmapi.CWMService, incs []cwmapi.Object) depGraph {
	var g depGraph
	inside := map[string]bool{}
	work := map[string][]cwmapi.Object{}
	for _, inc := range incs {
		inside[inc.ID] = true
		work[inc.ID] = incrementWork(ctx, cwm, inc)
		for _, w := range work[inc.ID] {
			inside[w.ID] = true
		}
	}
	seenEdge := map[string]bool{}
	external := map[string]bool{}
	arrow := func(from, to string) {
		if k := from + ">" + to; !seenEdge[k] && from != to {
			seenEdge[k] = true
			g.edges = append(g.edges, map[string]any{"from": from, "to": to})
		}
	}
	waitOn := func(waiter cwmapi.Object, on string) {
		if !inside[on] && !external[on] {
			external[on] = true
			if x, _, err := loadAny(ctx, cwm, on); err == nil {
				g.boxes = append(g.boxes, externalBox(ctx, cwm, x))
			} else {
				g.boxes = append(g.boxes, map[string]any{"id": on, "title": on, "frame": "dotted", "status": "todo", "meta": "not found"})
			}
		}
		arrow(on, waiter.ID)
	}
	for _, inc := range incs {
		r := readinessOf(ctx, cwm, inc)
		frame := map[string]any{"id": inc.ID, "title": titleOf(inc), "meta": inc.ID + " · " + incrementPriority(inc), "readiness": r.State, "status": readinessStatus(r, inc)}
		if b := readinessBadge(r); b != "" {
			frame["badges"] = []string{b}
		}
		g.boxes = append(g.boxes, frame)
		// An increment that holds no work yet still shows: a box of its own,
		// in its frame.
		if len(work[inc.ID]) == 0 {
			g.boxes = append(g.boxes, map[string]any{"id": inc.ID + "#empty", "title": "no work planned", "group": inc.ID, "status": "todo"})
		}
		for _, w := range work[inc.ID] {
			if w.ID == inc.ID {
				continue
			}
			g.boxes = append(g.boxes, map[string]any{"id": w.ID, "title": titleOf(w), "meta": w.ID, "group": inc.ID, "status": workStatus(ctx, cwm, w)})
		}
	}
	for _, inc := range incs {
		for _, d := range edgesTo(inc, api.EdgeDependsOn) {
			waitOn(inc, d)
		}
		for _, w := range work[inc.ID] {
			if w.ID == inc.ID {
				continue
			}
			for _, d := range edgesTo(w, api.EdgeDependsOn) {
				waitOn(w, d)
			}
		}
	}
	return g
}

// readinessStatus is the frame's colour: its readiness, done when done.
func readinessStatus(r api.IncrementReadiness, inc cwmapi.Object) string {
	switch r.State {
	case api.ReadinessRunning:
		return "running"
	case api.ReadinessWaiting:
		return "waiting"
	case api.ReadinessReady:
		return "todo"
	}
	if releaseState(inc) == api.IncrementDone {
		return "done"
	}
	return "dropped"
}

// externalBox is work outside the release that something in it waits on:
// dotted, saying the increment and release it is scheduled in - or none -
// and whether it runs, is done or not started.
func externalBox(ctx context.Context, cwm cwmapi.CWMService, x cwmapi.Object) map[string]any {
	status := workStatus(ctx, cwm, x)
	if x.Type == api.IncrementType {
		switch releaseState(x) {
		case api.IncrementDone:
			status = "done"
		case api.IncrementOpen:
			status = readinessStatus(readinessOf(ctx, cwm, x), x)
		}
	}
	where := "not scheduled"
	if inc := holderIncrement(ctx, cwm, x); inc != nil {
		where = "in " + titleOf(*inc)
		if rel := edgeTo(*inc, api.EdgeShipsIn); rel != "" {
			where += " · " + titleOfID(ctx, cwm, rel)
		} else {
			where += " · no release"
		}
	}
	run := map[string]string{"done": "done", "running": "running", "todo": "not started", "waiting": "waiting", "blocked": "blocked", "dropped": "dropped"}[status]
	return map[string]any{"id": x.ID, "title": titleOf(x), "meta": x.ID, "frame": "dotted", "status": status, "badges": []string{where, run}}
}

// holderIncrement is the increment a piece of work is scheduled in: the
// increment itself, the one it is aimed at (takerOf), or the one aimed at what it
// serves; nil when none holds it.
func holderIncrement(ctx context.Context, cwm cwmapi.CWMService, x cwmapi.Object) *cwmapi.Object {
	if x.Type == api.IncrementType {
		return &x
	}
	try := append([]string{x.ID}, edgesTo(x, api.EdgeServes)...)
	for _, id := range try {
		if t := takerOf(ctx, cwm, mustLoad(ctx, cwm, id)); t != "" {
			if o, _, err := loadAny(ctx, cwm, t); err == nil && o.Type == api.IncrementType {
				return &o
			}
		}
	}
	return nil
}

func mustLoad(ctx context.Context, cwm cwmapi.CWMService, id string) cwmapi.Object {
	o, _, _ := loadAny(ctx, cwm, id)
	return o
}
