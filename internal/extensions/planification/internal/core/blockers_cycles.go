// SPDX-License-Identifier: Apache-2.0

package core

import (
	"slices"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE WAITS THAT ARE NOT EDGES, SWEPT WHOLE - a wait
// that is no depends-on edge (a blocked report's held object, a step after a
// step of another request, a dependency or review wait settle wrote, a GUI
// batch) is refused at no write: a cycle of them can be stored. The one reader
// follows each wait from the work it starts at, so it meets such a cycle along
// one walk - named from wherever the walk began, and stopped at the scope's
// edge when it leaves the scope. Here every open request's waits form one
// graph; its strongly connected components (Tarjan) are the cycles, each named
// once - its members in a canonical order from its root, the smallest id - and
// every piece of work they hold, inside the scope or not, says which cycle
// holds it. A cycle is never broken by code: the person breaks one of its
// waits.

// waitCycles are the cycles of stored waits: by member, the cycle it is in.
type waitCycles struct {
	of    map[string]*storedCycle
	graph map[string][]string
}

// storedCycle is one cycle: its members sorted (the root first), and a path
// from the root back to it through its waits.
type storedCycle struct {
	members []string
	path    []string
}

// root is the cycle's smallest member: where its one incident is raised.
func (c *storedCycle) root() string { return c.members[0] }

// reason names the cycle: its path back to its root, then the members the
// path does not pass through.
func (c *storedCycle) reason() string {
	s := waitCycle + strings.Join(c.path, " → ")
	var more []string
	for _, m := range c.members {
		if !slices.Contains(c.path, m) {
			more = append(more, m)
		}
	}
	if len(more) > 0 {
		s += " (its waits also hold " + strings.Join(more, ", ") + ")"
	}
	return s
}

// next is the member a member's wait goes to along the cycle's path; any
// member it waits on when it is off the path.
func (c *waitCycles) next(cy *storedCycle, m string) string {
	if i := slices.Index(cy.path, m); i >= 0 && i+1 < len(cy.path) {
		return cy.path[i+1]
	}
	for _, d := range c.graph[m] {
		if c.of[d] == cy {
			return d
		}
	}
	return ""
}

// waitsOf is what one open to-do waits on outside its own request: the
// objects of its wait.
func (r *holdReader) waitsOf(t api.WorkTodo) []string {
	var out []string
	if dep, _ := waitTarget(t); dep != "" {
		out = append(out, dep)
	}
	if batch, ok := strings.CutPrefix(t.Waits, guiBatchWaits); ok {
		b, _, _ := strings.Cut(batch, ",")
		out = append(out, strings.TrimSpace(b))
	}
	if t.State == schedapi.StateWaiting && strings.HasPrefix(t.Waits, "after ") {
		for _, a := range t.After {
			if p, ok := r.byID[a]; ok && p.Request != t.Request {
				out = append(out, p.Request)
			}
		}
	}
	return out
}

// cycles reads the graph of every open request's waits on other open
// requests and finds its cycles.
func (r *holdReader) cycles() *waitCycles {
	g := map[string][]string{}
	nodes := make([]string, 0, len(r.open))
	for req, todos := range r.open {
		nodes = append(nodes, req)
		for _, t := range todos {
			for _, d := range r.waitsOf(t) {
				if _, open := r.open[d]; open && !slices.Contains(g[req], d) {
					g[req] = append(g[req], d)
				}
			}
		}
	}
	sort.Strings(nodes)
	for k := range g {
		sort.Strings(g[k])
	}
	out := &waitCycles{of: map[string]*storedCycle{}, graph: g}
	for _, scc := range tarjan(nodes, g) {
		if len(scc) == 1 && !slices.Contains(g[scc[0]], scc[0]) {
			continue
		}
		sort.Strings(scc)
		cy := &storedCycle{members: scc}
		cy.path = cyclePath(scc, g)
		for _, m := range scc {
			out.of[m] = cy
		}
	}
	return out
}

// cyclePath is the shortest path from a component's root back to it through
// its waits (breadth first, in id order).
func cyclePath(scc []string, g map[string][]string) []string {
	root := scc[0]
	in := map[string]bool{}
	for _, m := range scc {
		in[m] = true
	}
	prev := map[string]string{}
	queue := []string{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, d := range g[n] {
			if !in[d] {
				continue
			}
			if d == root {
				path := []string{root}
				for c := n; c != root; c = prev[c] {
					path = append(path, c)
				}
				slices.Reverse(path[1:])
				return append(path, root)
			}
			if _, seen := prev[d]; !seen {
				prev[d] = n
				queue = append(queue, d)
			}
		}
	}
	return append(slices.Clone(scc), root)
}

// tarjan is the strongly connected components of a graph, iterative.
func tarjan(nodes []string, g map[string][]string) [][]string {
	index, low := map[string]int{}, map[string]int{}
	on := map[string]bool{}
	var stack []string
	var out [][]string
	next := 0
	type frame struct {
		n string
		i int
	}
	for _, start := range nodes {
		if _, seen := index[start]; seen {
			continue
		}
		calls := []frame{{n: start}}
		index[start], low[start] = next, next
		next++
		stack = append(stack, start)
		on[start] = true
		for len(calls) > 0 {
			f := &calls[len(calls)-1]
			if f.i < len(g[f.n]) {
				d := g[f.n][f.i]
				f.i++
				if _, seen := index[d]; !seen {
					index[d], low[d] = next, next
					next++
					stack = append(stack, d)
					on[d] = true
					calls = append(calls, frame{n: d})
				} else if on[d] && index[d] < low[f.n] {
					low[f.n] = index[d]
				}
				continue
			}
			n := f.n
			calls = calls[:len(calls)-1]
			if len(calls) > 0 {
				if p := calls[len(calls)-1].n; low[n] < low[p] {
					low[p] = low[n]
				}
			}
			if low[n] == index[n] {
				var scc []string
				for {
					m := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					on[m] = false
					scc = append(scc, m)
					if m == n {
						break
					}
				}
				out = append(out, scc)
			}
		}
	}
	return out
}

// nameCycles puts the cycles on the scope's blockers: a member of a cycle is
// an incident naming the cycle whole, on the member its wait goes to, moved
// by the person - the walk stopped at the scope's edge or not; a piece of
// work held by a cycle without being in it names the cycle it reaches.
func (r *holdReader) nameCycles(blockers []api.Blocker) {
	c := r.cycles()
	if len(c.of) == 0 {
		return
	}
	for i := range blockers {
		b := &blockers[i]
		if cy, ok := c.of[b.Work]; ok && b.Todo != "" {
			if on := r.waitInto(b.Todo, cy, c); on != "" {
				b.Kind, b.Cause, b.Reason, b.On, b.Chain, b.Cycle = api.BlockerIncident, api.CauseCycle, cy.reason(), on, nil, cy.root()
				b.Actor, b.ActorKind = "the person: break one of its waits", api.ActorPerson
				continue
			}
		}
		if !strings.Contains(b.Reason, waitCycle) {
			continue
		}
		if cy := c.reached(b.Work); cy != nil {
			b.Cycle = cy.root()
			why, _, _ := strings.Cut(b.Reason, waitCycle)
			why = strings.TrimSuffix(strings.TrimSuffix(why, " - at its root: "), " - ")
			// Held by the cycle, not in it: planned again or taken out.
			b.Kind, b.Cause, b.Reason = api.BlockerIncident, "", orDefault(why, "it waits on "+b.On)+" - held by "+cy.reason()
			b.Actor, b.ActorKind = "the person: break one of the waits of the cycle at "+cy.root(), api.ActorPerson
			if len(b.Chain) == 0 {
				b.Chain = []string{cy.root()}
			}
		}
	}
}

// waitInto is the member of its cycle a member's to-do waits on - the next
// one along the cycle's path first; "" when that to-do's wait leaves it.
func (r *holdReader) waitInto(todo string, cy *storedCycle, c *waitCycles) string {
	t, ok := r.byID[todo]
	if !ok {
		return ""
	}
	waits := r.waitsOf(t)
	if n := c.next(cy, t.Request); slices.Contains(waits, n) {
		return n
	}
	for _, d := range waits {
		if c.of[d] == cy {
			return d
		}
	}
	return ""
}

// reached is the first cycle a piece of work reaches through its waits.
func (c *waitCycles) reached(from string) *storedCycle {
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, d := range c.graph[n] {
			if cy, ok := c.of[d]; ok {
				return cy
			}
			if !seen[d] {
				seen[d] = true
				queue = append(queue, d)
			}
		}
	}
	return nil
}
