// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"math"
	"slices"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A RELEASE'S TO-DOS CARRY ITS LABEL AND THEIR RANK — so a worker pulls the
// work of one increment, release or stream (`todos pull --labels`) and, within
// a priority, the critical path first. A to-do is labelled `increment:<IN>`,
// `release:<RL>` and `stream:<SM>` for what its request reaches: the increment
// or release it targets, or that targets the want it serves (or is filed on)
// or one of that want's parents (a feature under a goal the increment holds);
// the release the increment ships in; the stream the release is in. Its rank
// is 1 on the release network's critical path, else 2 + its slack (whole
// days of estimate). Labels and rank are the scheduler's opaque fields: each
// settle keeps them current.

// labelling remembers, for one settle, each request's labels and rank and
// each release's network.
type labelling struct {
	byRequest map[string]labelled
	networks  map[string]api.NetworkOutput
	incs      map[string]incHold // by increment
	// dependents are, by increment, the releases of the open increments that
	// depend on it, directly or not.
	dependents map[string][]string
	// holds are, by request, the requests whose to-dos it holds (their
	// HeldBy), read once (heldRead).
	holds    map[string][]string
	heldRead bool
}

// incHold is what an increment imposes on the work it holds: its priority,
// why all of it but its intake waits (the increment on hold), and why its
// build waits (an increment it depends on not done); "" when it does not.
type incHold struct {
	priority, held, dependency string
}

type labelled struct {
	labels []string
	rank   int
	// inc is the increment the request's work belongs to (none: ""), and
	// priority the request's own.
	inc, priority string
}

func newLabelling() *labelling {
	return &labelling{byRequest: map[string]labelled{}, networks: map[string]api.NetworkOutput{}, incs: map[string]incHold{}, dependents: map[string][]string{}}
}

// of answers a request's labels and rank.
func (l *labelling) of(ctx context.Context, cwm cwmapi.CWMService, request string) labelled {
	if got, ok := l.byRequest[request]; ok {
		return got
	}
	var out labelled
	o, err := loadObject(ctx, cwm, request)
	if err == nil {
		inc, rel := releaseOfWork(ctx, cwm, o)
		if inc == "" && rel == "" {
			inc, rel = l.heldWork(ctx, cwm, request)
		}
		out.inc = inc
		if pf, ok := planOf(o); ok {
			out.priority = pf.Priority
		}
		if inc != "" {
			out.labels = append(out.labels, "increment:"+inc)
		}
		if rel != "" {
			out.labels = append(out.labels, "release:"+rel)
			if r, _, err := loadAny(ctx, cwm, rel); err == nil {
				if sm := edgeTo(r, api.EdgeInStream); sm != "" {
					out.labels = append(out.labels, "stream:"+sm)
				}
			}
			out.rank = l.rankIn(ctx, rel, request)
		}
		// A release whose increment depends on this one takes its work too:
		// a loop on that release sees what it waits on.
		if inc != "" {
			for _, r := range l.dependentReleases(ctx, cwm, inc) {
				if r != rel {
					out.labels = append(out.labels, "release:"+r)
				}
			}
		}
	}
	l.byRequest[request] = out
	return out
}

// heldWork is the increment and the release of the work a request holds: a
// review filed on a want no scope takes (a feature-changed review) holding a
// step of another request's work (its HeldBy) is that work's - its to-dos
// carry the labels of what waits on it, so a pull by the increment or the
// release hands it out. The first held request
// reaching a scope wins; none: "", "".
func (l *labelling) heldWork(ctx context.Context, cwm cwmapi.CWMService, request string) (inc, rel string) {
	if !l.heldRead {
		l.heldRead = true
		l.holds = map[string][]string{}
		if todos, err := todosOf(ctx, ""); err == nil {
			for _, t := range todos {
				for _, r := range t.Packet.HeldBy {
					if r != t.Request && !slices.Contains(l.holds[r], t.Request) {
						l.holds[r] = append(l.holds[r], t.Request)
					}
				}
			}
		}
	}
	for _, h := range l.holds[request] {
		o, err := loadObject(ctx, cwm, h)
		if err != nil {
			continue
		}
		if inc, rel = releaseOfWork(ctx, cwm, o); inc != "" || rel != "" {
			return inc, rel
		}
	}
	return "", ""
}

// dependentReleases are the releases of the open increments that depend on
// inc, directly or through another increment, in any stream.
func (l *labelling) dependentReleases(ctx context.Context, cwm cwmapi.CWMService, inc string) []string {
	if got, ok := l.dependents[inc]; ok {
		return got
	}
	out := []string{}
	seen := map[string]bool{inc: true}
	queue := []string{inc}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		from, err := incoming(ctx, cwm, at, api.EdgeDependsOn)
		if err != nil {
			continue
		}
		for _, d := range from {
			if seen[d] {
				continue
			}
			seen[d] = true
			o, _, err := loadAny(ctx, cwm, d)
			if err != nil || o.Type != api.IncrementType {
				continue
			}
			if waitEnded(o) {
				continue
			}
			if r := edgeTo(o, api.EdgeShipsIn); r != "" && !slices.Contains(out, r) {
				out = append(out, r)
			}
			queue = append(queue, d)
		}
	}
	l.dependents[inc] = out
	return out
}

// holdOf is what an increment imposes on the work it holds: its own
// priority, the hold a person put on it, and - while an increment it depends
// on is neither done nor dropped - the wait of its build.
func (l *labelling) holdOf(ctx context.Context, cwm cwmapi.CWMService, inc string) incHold {
	if h, ok := l.incs[inc]; ok {
		return h
	}
	var h incHold
	if held := heldIncrement(ctx, inc); held != nil {
		h.held = "its increment " + inc + " is on hold"
		if held.Reason != "" {
			h.held += ": " + held.Reason
		}
	}
	if o, _, err := loadAny(ctx, cwm, inc); err == nil {
		if pf, ok := planOf(o); ok {
			h.priority = pf.Priority
		}
		// An increment that has ended holds its work on nothing more
		//
		deps := edgesTo(o, api.EdgeDependsOn)
		if isDoneIncrement(o) {
			deps = nil
		}
		for _, d := range deps {
			dep, _, err := loadAny(ctx, cwm, d)
			if err != nil || dep.Type != api.IncrementType {
				continue
			}
			if !waitEnded(dep) {
				h.dependency = "its increment " + inc + " waits on " + d
				break
			}
		}
	}
	l.incs[inc] = h
	return h
}

// higherPriority is the more urgent of two priorities; an unset one yields.
func higherPriority(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	case priorityRank(b) < priorityRank(a):
		return b
	}
	return a
}

// releaseOfWork is the increment and the release a request's work belongs to.
func releaseOfWork(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) (inc, rel string) {
	return releaseOfWorkFrom(ctx, cwm, o, 0)
}

// releaseOfWorkFrom is releaseOfWork, depth requests away from the one asked:
// an increment wins over a bare release - the want a request serves is held
// by its increment even when the request itself still targets a release (one
// rescoped from a release's acceptance to an increment); and a follow-up or a
// split part that reaches neither is its origin's work (a request an agent
// files during an increment's run).
func releaseOfWorkFrom(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, depth int) (inc, rel string) {
	held := "" // a held increment holding a want it serves
	at := func(id string) {
		t, _, err := loadAny(ctx, cwm, id)
		if err != nil {
			return
		}
		switch t.Type {
		case api.IncrementType:
			inc = t.ID
			rel = edgeTo(t, api.EdgeShipsIn)
		case api.ReleaseType:
			rel = t.ID
		}
	}
	if o.Type == api.IncrementType {
		// An increment's own request (its proposal): its work is the increment's.
		at(o.ID)
	}
	if t := takerOf(ctx, cwm, o); t != "" && inc == "" && rel == "" {
		at(t)
	}
	if inc == "" {
		// The want it serves or is filed on, then that want's parents: a
		// feature under a goal of the increment is its work (plan-work files
		// it). Searched even past the request's own release target: what an
		// increment holds is the increment's work.
		direct := rel
		rel = ""
		// Every want it serves, each with its parents: the first held by an
		// increment wins; with none, the first that reaches a release. A
		// closed scope (an increment done or dropped, a release released)
		// never labels it; a held increment does only when the request's own
		// origin reaches no open scope.
		firstRel := ""
		for _, w := range servedWants(ctx, cwm, o) {
			rel = ""
			for hops := 0; w != "" && inc == "" && rel == "" && hops < 8; hops++ {
				wo, err := loadObject(ctx, cwm, w)
				if err != nil {
					break
				}
				w = ""
				if t := takerOf(ctx, cwm, wo); t != "" {
					switch takerScope(ctx, cwm, t) {
					case scopeClosed:
					case scopeHeld:
						if held == "" {
							held = t
						}
						continue // the want is the held increment's: not its parents'
					default:
						at(t)
					}
				}
				if wo.Parent != "" && carriesWant(ctx, cwm, wo.Parent) {
					w = wo.Parent
				}
			}
			if inc != "" {
				break
			}
			if firstRel == "" {
				firstRel = rel
			}
		}
		if inc == "" {
			rel = firstRel
		}
		if inc == "" && direct != "" {
			rel = direct // no increment holds what it serves: its own release
		}
	}
	if inc == "" && rel == "" && depth < 8 {
		// A follow-up or a part reaching neither: the work of what filed it.
		for _, kind := range []string{api.EdgeFollowsUp, api.EdgeSplitFrom} {
			from := edgeTo(o, kind)
			if from == "" {
				continue
			}
			if fo, err := loadObject(ctx, cwm, from); err == nil {
				if inc, rel = releaseOfWorkFrom(ctx, cwm, fo, depth+1); inc != "" || rel != "" {
					break
				}
			}
		}
	}
	if inc == "" && rel == "" && held != "" {
		// Its origin reaches no open scope: the held increment's work, held.
		at(held)
	}
	return inc, rel
}

// takerScope is the scope a taker gives the work it holds: open, held (an increment a
// person put on hold) or closed (an increment done or dropped, one shipped in
// a released release, a release released).
const (
	scopeOpen = iota
	scopeHeld
	scopeClosed
)

func takerScope(ctx context.Context, cwm cwmapi.CWMService, taker string) int {
	t, _, err := loadAny(ctx, cwm, taker)
	if err != nil || !liveTaker(t) {
		return scopeClosed
	}
	if t.Type != api.IncrementType {
		return scopeOpen
	}
	if r := edgeTo(t, api.EdgeShipsIn); r != "" {
		if ro, _, err := loadAny(ctx, cwm, r); err == nil && releaseState(ro) == api.ReleaseReleased {
			return scopeClosed
		}
	}
	if heldIncrement(ctx, taker) != nil {
		return scopeHeld
	}
	return scopeOpen
}

// rankIn is a request's rank in its release's network: 1 on the critical
// path, else 2 + its slack; 0 (unset) when it is no node of it.
func (l *labelling) rankIn(ctx context.Context, rel, request string) int {
	n, ok := l.networks[rel]
	if !ok {
		var err error
		if n, err = NewPlanning().Network(ctx, api.GetInput{ID: rel}); err != nil {
			l.networks[rel] = api.NetworkOutput{}
			return 0
		}
		l.networks[rel] = n
	}
	i := slices.IndexFunc(n.Nodes, func(x api.NetNode) bool { return x.ID == request })
	if i < 0 {
		return 0
	}
	if n.Nodes[i].Critical {
		return 1
	}
	return 2 + int(math.Floor(n.Nodes[i].Slack))
}

// servedWants is every want a request's work is for: the object itself when
// it carries a want, else each one it serves, then each one it is about - in
// that order (servedOf is the first of them).
func servedWants(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) []string {
	if carriesWant(ctx, cwm, o.ID) {
		return []string{o.ID}
	}
	var out []string
	for _, kind := range []string{api.EdgeServes, api.EdgeAbout} {
		for _, e := range o.Edges {
			if e.Kind == kind && !slices.Contains(out, e.To) && carriesWant(ctx, cwm, e.To) {
				out = append(out, e.To)
			}
		}
	}
	return out
}
