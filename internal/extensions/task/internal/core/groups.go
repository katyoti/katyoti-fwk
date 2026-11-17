// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// groupScheduler limits, per task group, how many runs RUN at once. A run
// takes a slot when it starts or is woken (acquire) and gives it back when its
// machine waits or ends (release / done); waiting holds none. A run with no
// free slot queues: higher priority first, then submit order. Groups do not
// see each other.
type groupScheduler struct {
	mu      sync.Mutex
	cfg     taskapi.TaskGroupsConfig
	groups  map[string]*groupState
	runs    map[string]*slotRun
	seq     uint64
	total   int                                                  // runs holding a slot, all groups together (the universe's ceiling)
	onState func(runID string, st taskapi.RunStatus, why string) // the run registry's status follows (why: a queued run's reason)
	// aging: a queued run's priority rises by one per aging period waited (0 = none).
	aging time.Duration
}

type groupState struct {
	name       string
	max        int
	queueLimit int
	on         bool
	running    int
	queue      []*slotWaiter
}

type slotRun struct {
	group    string
	priority int
	holding  bool
	waiting  bool          // gave its slot back: its machine waits
	gone     chan struct{} // closed when the run ends: a waiter for it stops
}

type slotWaiter struct {
	runID    string
	priority int
	seq      uint64
	since    time.Time // when it started to queue: its priority ages from then
	ready    chan struct{}
}

func newGroupScheduler(cfg taskapi.TaskGroupsConfig) *groupScheduler {
	aging := time.Duration(cfg.PriorityAgingS) * time.Second
	if cfg.PriorityAgingS == 0 {
		aging = taskapi.DefaultPriorityAgingS * time.Second
	} else if cfg.PriorityAgingS < 0 {
		aging = 0
	}
	return &groupScheduler{cfg: cfg, groups: map[string]*groupState{}, runs: map[string]*slotRun{}, aging: aging}
}

// defaultGroup is the group of a run that names none and has no parent.
func (g *groupScheduler) defaultGroup() string {
	if g.cfg.DefaultGroup != "" {
		return g.cfg.DefaultGroup
	}
	return taskapi.DefaultGroup
}

// group returns a group's state, made from its declaration (or the defaults)
// on first use. g.mu held.
func (g *groupScheduler) group(name string) *groupState {
	if st, ok := g.groups[name]; ok {
		return st
	}
	decl := g.cfg.Groups[name]
	st := &groupState{name: name, max: decl.MaxRunning, queueLimit: decl.QueueLimit, on: decl.On == nil || *decl.On}
	if st.max <= 0 {
		st.max = taskapi.DefaultMaxRunning
	}
	g.groups[name] = st
	return st
}

// resolve is the group a run named runs in: a group TaskGroups.yaml declares
// (or the default group); any other name runs in the default group — a typo or
// an unknown name does not create a group of slots of its own.
func (g *groupScheduler) resolve(name string) string {
	def := g.defaultGroup()
	if name == "" || name == def {
		return def
	}
	if _, ok := g.cfg.Groups[name]; ok {
		return name
	}
	return def
}

// ceiling is how many runs of all groups may run at once.
func (g *groupScheduler) ceiling() int {
	if g.cfg.MaxRunning > 0 {
		return g.cfg.MaxRunning
	}
	return taskapi.DefaultUniverseMaxRunning
}

// groupOf is the group a run was admitted in; "" when unknown.
func (g *groupScheduler) groupOf(runID string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r, ok := g.runs[runID]; ok {
		return r.group
	}
	return ""
}

// admit registers a run in its group, refusing it when it would have to queue
// and the group's queue is full. It takes no slot: acquire does.
func (g *groupScheduler) admit(runID, group string, priority int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.group(group)
	free := st.on && st.running < st.max && len(st.queue) == 0
	if !free && st.queueLimit > 0 && len(st.queue) >= st.queueLimit {
		return &taskapi.GroupFullError{Group: group, Running: st.running, Queued: len(st.queue), MaxRunning: st.max, QueueLimit: st.queueLimit}
	}
	g.runs[runID] = &slotRun{group: group, priority: priority, gone: make(chan struct{})}
	return nil
}

// admitChild registers a child of a run already admitted: it queues for a
// slot, never refused by the queue's limit.
func (g *groupScheduler) admitChild(runID, group string, priority int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.runs[runID] = &slotRun{group: group, priority: priority, gone: make(chan struct{})}
	return nil
}

// acquire takes a slot of the run's group, queueing until one is free. It
// returns ctx's error, or nil at once for a run the scheduler does not know or
// that already holds its slot.
func (g *groupScheduler) acquire(ctx context.Context, runID string) error {
	g.mu.Lock()
	r, ok := g.runs[runID]
	if !ok || r.holding {
		g.mu.Unlock()
		return nil
	}
	st := g.group(r.group)
	g.seq++
	w := &slotWaiter{runID: runID, priority: r.priority, seq: g.seq, since: time.Now(), ready: make(chan struct{})}
	st.queue = append(st.queue, w)
	g.dispatchAll()
	queued := !r.holding
	why := g.whyQueued(runID)
	gone := r.gone
	g.mu.Unlock()
	if queued {
		g.notify(runID, taskapi.RunStatusQueued, why)
	}

	select {
	case <-w.ready:
		g.notify(runID, taskapi.RunStatusRunning, "")
		return nil
	case <-ctx.Done():
		return g.gaveUp(runID, w, ctx.Err())
	case <-gone:
		return g.gaveUp(runID, w, context.Canceled)
	}
}

// gaveUp takes a waiter that stops queueing out of its queue and returns why
// it stopped: cause, with why it still had no slot then — a run stopped while
// queued says what kept it waiting (its group off or full, the universe's
// ceiling), never only that it was stopped.
func (g *groupScheduler) gaveUp(runID string, w *slotWaiter, cause error) error {
	g.mu.Lock()
	why := g.whyQueued(runID)
	g.mu.Unlock()
	g.abandon(runID, w)
	if why == "" { // its slot came meanwhile: abandon gave it back
		return cause
	}
	return &queuedError{Cause: cause, Why: why}
}

// queuedError is the end of a wait for a slot that never came: Cause (the
// run's stop, its time out) and Why it had no slot.
type queuedError struct {
	Cause error
	Why   string
}

func (e *queuedError) Error() string { return e.Cause.Error() + " (queued: " + e.Why + ")" }
func (e *queuedError) Unwrap() error { return e.Cause }

// queuedReason is whyQueued now, for a reader outside the scheduler.
func (g *groupScheduler) queuedReason(runID string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.whyQueued(runID)
}

// whyQueued says why a queued run has no slot: its group off, its group
// full, the universe's ceiling reached, else the runs before it in its
// group's queue. "" for a run that holds its slot or is unknown. g.mu held.
func (g *groupScheduler) whyQueued(runID string) string {
	r, ok := g.runs[runID]
	if !ok || r.holding {
		return ""
	}
	st := g.group(r.group)
	switch {
	case !st.on:
		return fmt.Sprintf("its group %q is off (TaskGroups.yaml on: false, or paused live)", st.name)
	case st.running >= st.max:
		return fmt.Sprintf("its group %q is full: %d of %d running", st.name, st.running, st.max)
	case g.total >= g.ceiling():
		return fmt.Sprintf("the universe's ceiling is reached: %d runs of all groups running, at most %d", g.total, g.ceiling())
	default:
		return fmt.Sprintf("it waits its turn in group %q (%d queued)", st.name, len(st.queue))
	}
}

// abandon takes a waiter that gave up out of its queue — or, when its slot was
// granted meanwhile, gives the slot back.
func (g *groupScheduler) abandon(runID string, w *slotWaiter) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r, ok := g.runs[runID]
	if !ok {
		return
	}
	st := g.group(r.group)
	for i, q := range st.queue {
		if q == w {
			st.queue = append(st.queue[:i], st.queue[i+1:]...)
			return
		}
	}
	if r.holding {
		r.holding = false
		st.running--
		g.total--
		g.dispatchAll()
	}
}

// release gives the run's slot back: its machine waits.
func (g *groupScheduler) release(runID string) {
	g.mu.Lock()
	r, ok := g.runs[runID]
	if !ok || !r.holding {
		g.mu.Unlock()
		return
	}
	r.holding = false
	r.waiting = true
	st := g.group(r.group)
	st.running--
	g.total--
	g.dispatchAll()
	g.mu.Unlock()
	g.notify(runID, taskapi.RunStatusWaiting, "")
}

// admitRestored registers a run restored after a restart without the queue
// limit check — a full queue does not refuse a run that was already admitted
// before the crash. waiting: it waits again and holds no slot; else it
// queues for a slot (acquire), a run started again.
func (g *groupScheduler) admitRestored(runID, group string, priority int, waiting bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.group(group)
	g.runs[runID] = &slotRun{group: group, priority: priority, waiting: waiting, gone: make(chan struct{})}
}

// ended is closed when the run is done (or at once for a run the scheduler
// does not know): what waits for the run stops with it.
func (g *groupScheduler) ended(runID string) <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r, ok := g.runs[runID]; ok {
		return r.gone
	}
	c := make(chan struct{})
	close(c)
	return c
}

// done gives the run's slot back for good and forgets it.
func (g *groupScheduler) done(runID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r, ok := g.runs[runID]
	if !ok {
		return
	}
	delete(g.runs, runID)
	close(r.gone)
	st := g.group(r.group)
	if r.holding {
		st.running--
		g.total--
	}
	for i, q := range st.queue {
		if q.runID == runID {
			st.queue = append(st.queue[:i], st.queue[i+1:]...)
			break
		}
	}
	g.dispatchAll()
}

// dispatchAll grants free slots to the best waiters of every group, within
// each group's limit and the universe's ceiling. The default group (a person's
// runs) is served first, then the groups by priority. Within a group: by
// priority — aged by the time waited — then submit order. g.mu held.
func (g *groupScheduler) dispatchAll() {
	def := g.defaultGroup()
	states := make([]*groupState, 0, len(g.groups))
	for _, st := range g.groups {
		states = append(states, st)
	}
	sort.SliceStable(states, func(i, j int) bool {
		if (states[i].name == def) != (states[j].name == def) {
			return states[i].name == def
		}
		pi, pj := g.cfg.Groups[states[i].name].Priority, g.cfg.Groups[states[j].name].Priority
		if pi != pj {
			return pi > pj
		}
		return states[i].name < states[j].name
	})
	now := time.Now()
	for _, st := range states {
		if !st.on || len(st.queue) == 0 {
			continue
		}
		sort.SliceStable(st.queue, func(i, j int) bool {
			pi, pj := g.aged(st.queue[i], now), g.aged(st.queue[j], now)
			if pi != pj {
				return pi > pj
			}
			return st.queue[i].seq < st.queue[j].seq
		})
		for st.running < st.max && g.total < g.ceiling() && len(st.queue) > 0 {
			w := st.queue[0]
			st.queue = st.queue[1:]
			r, ok := g.runs[w.runID]
			if !ok {
				continue
			}
			r.holding = true
			r.waiting = false
			st.running++
			g.total++
			close(w.ready)
		}
	}
}

// aged is a waiter's priority raised by the time it has queued.
func (g *groupScheduler) aged(w *slotWaiter, now time.Time) int {
	if g.aging <= 0 || w.since.IsZero() {
		return w.priority
	}
	return w.priority + int(now.Sub(w.since)/g.aging)
}

// load is a group's running and queued counts (tests; the API in te4).
func (g *groupScheduler) load(group string) (running, queued int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.group(group)
	return st.running, len(st.queue)
}

func (g *groupScheduler) notify(runID string, st taskapi.RunStatus, why string) {
	if g.onState != nil {
		g.onState(runID, st, why)
	}
}

// loads is every group's load: the declared groups and any a run used.
func (g *groupScheduler) loads() []taskapi.GroupLoad {
	g.mu.Lock()
	defer g.mu.Unlock()
	for name := range g.cfg.Groups {
		g.group(name)
	}
	g.group(g.defaultGroup())
	waiting := map[string]int{}
	for _, r := range g.runs {
		if r.waiting {
			waiting[r.group]++
		}
	}
	out := make([]taskapi.GroupLoad, 0, len(g.groups))
	for name, st := range g.groups {
		out = append(out, taskapi.GroupLoad{
			Name: name, Running: st.running, Waiting: waiting[name], Queued: len(st.queue),
			MaxRunning: st.max, QueueLimit: st.queueLimit, Priority: g.cfg.Groups[name].Priority, On: st.on,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// set changes a group's limits live; a raised limit or a resumed group starts
// queued runs at once.
func (g *groupScheduler) set(in taskapi.SetGroupRequest) taskapi.GroupLoad {
	g.mu.Lock()
	st := g.group(in.Group)
	if in.MaxRunning != nil && *in.MaxRunning > 0 {
		st.max = *in.MaxRunning
	}
	if in.QueueLimit != nil && *in.QueueLimit >= 0 {
		st.queueLimit = *in.QueueLimit
	}
	if in.On != nil {
		st.on = *in.On
	}
	g.dispatchAll()
	g.mu.Unlock()
	for _, l := range g.loads() {
		if l.Name == in.Group {
			return l
		}
	}
	return taskapi.GroupLoad{Name: in.Group}
}
