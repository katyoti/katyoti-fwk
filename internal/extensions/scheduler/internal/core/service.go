// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// Service implements api.TodoService over its store port: the store the
// application chose (ServiceIn, OpenSchedulerStore), and the record of the
// pulls it was asked (the application's, PullRecord).
type Service struct {
	store Store
	pulls *PullRecord
	// planned, when a test sets it, is the module references the queue reads
	// and the planning space of each (plannedModules otherwise).
	planned func(ctx context.Context) ([]plannedModule, error)
	// repoOf, when a test sets it, is the repository a module reference
	// resolves to (repositoryOf otherwise).
	repoOf func(ctx context.Context, module string) (string, error)
}

// plannedModule is a module the queue reads: its reference (bare, or
// qualified by its project) and its planning space, where its mode is kept.
type plannedModule struct {
	ref, space string
}

// plannedModules is every module whose to-dos the queue reads, each with its
// planning space: the user modules the CWM lists, and the requests module.
func plannedModules(ctx context.Context) ([]plannedModule, error) {
	mods, err := modules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]plannedModule, 0, len(mods))
	for _, m := range mods {
		space, err := planningSpace(ctx, m)
		if err != nil {
			return nil, err
		}
		out = append(out, plannedModule{ref: m, space: space})
	}
	return out, nil
}

// plannedIn is the modules the queue reads: module alone when named - a bare
// name several served projects hold is refused, naming their references
// (oneModule).
func (s *Service) plannedIn(ctx context.Context, module string) ([]plannedModule, error) {
	if s.planned != nil {
		all, err := s.planned(ctx)
		if err != nil || module == "" {
			return all, err
		}
		for _, p := range all {
			if sameModule(p.ref, module) {
				return []plannedModule{p}, nil
			}
		}
		return nil, fmt.Errorf("%w: module %s", cwmapi.ErrNotFound, module)
	}
	if module == "" {
		return plannedModules(ctx)
	}
	if err := oneModule(ctx, module); err != nil {
		return nil, err
	}
	space, err := planningSpace(ctx, module)
	if err != nil {
		return nil, err
	}
	return []plannedModule{{ref: module, space: space}}, nil
}

var _ api.TodoService = (*Service)(nil)

// New returns the scheduler over the file store, whatever the universe holds:
// a test's or a tool's own; an application's verbs run on ServiceIn.
func New() *Service { return &Service{store: newFileStore(), pulls: newPullRecord()} }

// NewOver returns the scheduler over a store: the SQLite store
// (OpenSQLiteStore), with a pull record of its own.
func NewOver(st Store) *Service { return &Service{store: st, pulls: newPullRecord()} }

var priorities = []string{"P0", "P1", "P2", "P3"}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: todos: %s", cwmapi.ErrInvalidInput, fmt.Sprintf(format, args...))
}

func checkActor(by cwmapi.Actor) error {
	if by.ID == "" || (by.Kind != "person" && by.Kind != "agent") {
		return invalid("by needs an id and a kind (person or agent), got %+v", by)
	}
	return nil
}

// --- reading -----------------------------------------------------------------

// scheduled is every to-do still scheduled (or done), with the mode of each
// module, keyed by its project and its module (modeKey): two projects'
// modules of one name keep their own mode.
func (s *Service) scheduled(ctx context.Context, module string, done bool) ([]api.Todo, map[string]string, error) {
	planned, err := s.plannedIn(ctx, module)
	if err != nil {
		return nil, nil, err
	}
	modes := map[string]string{}
	var out []api.Todo
	for _, p := range planned {
		st, err := settings(p.space)
		if err != nil {
			return nil, nil, err
		}
		got, err := s.store.List(ctx, p.ref, done)
		if err != nil {
			return nil, nil, err
		}
		for _, t := range got {
			modes[modeKey(t.Repository, t.Module)] = st.Scheduling
		}
		out = append(out, got...)
	}
	return out, modes, nil
}

// Get reads one to-do, scheduled or done.
func (s *Service) Get(ctx context.Context, in api.GetInput) (api.TodoOutput, error) {
	t, err := s.store.Get(ctx, in.ID)
	if err != nil {
		return api.TodoOutput{}, err
	}
	return api.TodoOutput{Todo: t}, nil
}

// List lists to-dos.
func (s *Service) List(ctx context.Context, in api.ListInput) (api.TodosOutput, error) {
	since, err := sinceOf(in.Since)
	if err != nil {
		return api.TodosOutput{}, err
	}
	all, _, err := s.scheduled(ctx, in.Module, in.Done)
	if err != nil {
		return api.TodosOutput{}, err
	}
	out := api.TodosOutput{Todos: []api.Todo{}}
	for _, t := range all {
		if kept(t, in, since) {
			out.Todos = append(out.Todos, t)
		}
	}
	sort.SliceStable(out.Todos, func(i, j int) bool { return before(out.Todos[i], out.Todos[j], "") })
	return out, nil
}

// --- the queue ---------------------------------------------------------------

// held reports a claimed to-do whose lease has not passed.
func held(t api.Todo) bool {
	if t.State != api.StateClaimed || t.Claim == nil {
		return false
	}
	until, err := time.Parse(time.RFC3339Nano, t.Claim.Until)
	return err != nil || time.Now().UTC().Before(until)
}

// reporterClaim is the claim of a to-do reported done whose reporter's lease
// has not passed, nil otherwise: a failed or blocked report gave it up.
func reporterClaim(t api.Todo) *api.Claim {
	if t.State != api.StateReported || t.Claim == nil || t.Result == nil || t.Result.Status != api.StatusSuccess {
		return nil
	}
	until, err := time.Parse(time.RFC3339Nano, t.Claim.Until)
	if err != nil || !time.Now().UTC().Before(until) {
		return nil
	}
	c := *t.Claim
	return &c
}

// pullable reports a to-do a worker may take: ready (and past its retry's
// not_before), or claimed and its lease passed.
func pullable(t api.Todo) bool {
	return (t.State == api.StateReady && later(t) == "") || (t.State == api.StateClaimed && !held(t))
}

// later says until when a ready to-do waits before it is retried; "" when it
// does not.
func later(t api.Todo) string {
	if t.NotBefore == "" {
		return ""
	}
	at, err := time.Parse(time.RFC3339Nano, t.NotBefore)
	if err != nil || !time.Now().UTC().Before(at) {
		return ""
	}
	return "retried after " + t.NotBefore + " (attempt " + strconv.Itoa(t.Attempts+1) + " of " + strconv.Itoa(t.MaxAttempts) + ")"
}

// maxBackoff caps the wait before a retry.
const maxBackoff = time.Hour

// retryAt is when a to-do failed n times is retried: its backoff doubled at
// each attempt, at most maxBackoff.
func retryAt(t api.Todo) string {
	d := time.Duration(t.BackoffSeconds) * time.Second
	for i := 1; i < t.Attempts && d < maxBackoff; i++ {
		d *= 2
	}
	d = min(d, maxBackoff)
	if d <= 0 {
		return ""
	}
	return time.Now().UTC().Add(d).Format(time.RFC3339Nano)
}

func rank(p string) int {
	if i := slices.Index(priorities, p); i >= 0 {
		return i
	}
	return len(priorities)
}

// order is a to-do's rank as the queue sorts it: a set rank (above 0) in
// its order, an unset one after every set one.
func order(t api.Todo) int {
	if t.Rank > 0 {
		return t.Rank
	}
	return math.MaxInt
}

// before is the queue order: priority, then the owner's rank, then the
// preferred module (a tie only), then age (since), then the request, then
// position.
func before(a, b api.Todo, prefer string) bool {
	if ra, rb := rank(a.Priority), rank(b.Priority); ra != rb {
		return ra < rb
	}
	if oa, ob := order(a), order(b); oa != ob {
		return oa < ob
	}
	if pa, pb := a.Module == prefer, b.Module == prefer; prefer != "" && pa != pb {
		return pa
	}
	if a.Since != b.Since {
		return a.Since < b.Since
	}
	if a.Request != b.Request {
		return a.Request < b.Request
	}
	if a.Position != b.Position {
		return a.Position < b.Position
	}
	return a.ID < b.ID
}

// matches reports a to-do the filter keeps.
func matches(t api.Todo, f api.Filter) bool {
	switch {
	case f.Module != "" && t.Module != f.Module,
		f.Project != "" && t.Repository != f.Project,
		f.Request != "" && t.Request != f.Request,
		f.Group != "" && t.Group != f.Group,
		f.Skill != "" && t.Skill != f.Skill,
		f.Priority != "" && rank(t.Priority) > rank(f.Priority),
		f.For != "" && t.For != f.For:
		return false
	}
	for _, l := range f.Labels {
		if !slices.Contains(t.Labels, l) {
			return false
		}
	}
	return true
}

// queue orders the pullable to-dos the filter keeps against the modules the
// held ones use — every held one, whatever the filter: a lock taken outside
// the filter still blocks. A takeable to-do reserves its modules against the
// ones after it, so two workers never get two writers of one module; a pull
// of one given to-do (only) counts no reservation - only what is held stops
// it. prefer is a module that wins a tie (Pull).
func (s *Service) queue(ctx context.Context, f api.Filter, auto bool, prefer, only string) ([]api.QueueEntry, error) {
	q, _, err := s.queueIn(ctx, f, auto, prefer, only)
	return q, err
}

// queueState is what a queue read beside its entries: each module's mode and
// the held to-dos (the takers' rules read them).
type queueState struct {
	modes map[string]string
	held  []api.Todo
}

// queueIn is queue, with what it read.
func (s *Service) queueIn(ctx context.Context, f api.Filter, auto bool, prefer, only string) ([]api.QueueEntry, queueState, error) {
	all, modes, err := s.scheduled(ctx, "", false)
	if err != nil {
		return nil, queueState{}, err
	}
	st := queueState{modes: modes}
	l := &locks{}
	var candidates, retried []api.Todo
	for _, t := range all {
		switch {
		case held(t):
			l.holdFor(t)
			st.held = append(st.held, t)
		case t.State == api.StateReady && later(t) != "" && modeOf(modes, t) != api.ModeOff && matches(t, f):
			retried = append(retried, t)
		case !pullable(t), modeOf(modes, t) == api.ModeOff, !matches(t, f):
		case auto && (modeOf(modes, t) != api.ModeAuto || t.Start != api.StartAuto || t.For != api.ForAgent):
		default:
			candidates = append(candidates, t)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return before(candidates[i], candidates[j], prefer) })
	out := make([]api.QueueEntry, 0, len(candidates))
	for _, t := range candidates {
		if why, w := l.waitIn(t.Repository, t.Reads, t.Writes); why != "" {
			w.Todo = t.ID
			out = append(out, api.QueueEntry{Todo: t, Waits: why, Wait: w})
			continue
		}
		if only == "" {
			l.reserveFor(t)
		}
		out = append(out, api.QueueEntry{Todo: t, Takeable: true})
	}
	for _, t := range retried {
		out = append(out, api.QueueEntry{Todo: t, Waits: later(t), Wait: &api.QueueWait{Todo: t.ID, Cause: api.WaitBackoff, Until: t.NotBefore}})
	}
	// Who alone may take each entry, whoever looks.
	ms := newMemberships(ctx)
	for i := range out {
		o, err := ms.only(out[i].Todo)
		if err != nil {
			return nil, queueState{}, err
		}
		out[i].Only = o
	}
	return out, st, nil
}

// Queue answers the ready to-dos in the order they are pulled, each takeable
// one with the recorded pulls that would take it.
func (s *Service) Queue(ctx context.Context, f api.Filter) (api.QueueOutput, error) {
	f, err := s.scopeOf(ctx, f)
	if err != nil {
		return api.QueueOutput{}, err
	}
	q, st, err := s.queueIn(ctx, f, false, "", "")
	if err != nil {
		return api.QueueOutput{}, err
	}
	since, err := s.takers(ctx, q, st)
	if err != nil {
		return api.QueueOutput{}, err
	}
	return api.QueueOutput{Queue: q, TakersSince: since}, nil
}

// --- the worker's verbs --------------------------------------------------------

// waitEvery is how often a pull that waits looks again: a read of the to-do
// files, no model token.
var waitEvery = 2 * time.Second

// Pull takes the next ready to-do for the filter, or a given one. With a
// wait, it looks again until it takes one, the filter's scope holds nothing
// an agent can move (finished), or the wait passes (timeout).
func (s *Service) Pull(ctx context.Context, in api.PullInput) (out api.PullOutput, err error) {
	if in.Filter, err = s.scopeOf(ctx, in.Filter); err != nil {
		return api.PullOutput{}, err
	}
	// Recorded outside the store's hold: a pull by id names its to-do, never
	// recorded; a pull that waits is open until it answers.
	if in.ID == "" && checkActor(in.By) == nil && s.pulls != nil {
		id := s.pulls.open(in.By, callerSession(ctx), in.Filter, in.Auto)
		defer func() { s.pulls.end(id, outcomeOf(out, err)) }()
	}
	if in.Wait == "" {
		out, _, err := s.pull(ctx, in)
		return out, err
	}
	d, err := time.ParseDuration(in.Wait)
	if err != nil || d <= 0 || d > api.MaxWait {
		return api.PullOutput{}, invalid("wait %q: a duration above 0 and at most %s (30m)", in.Wait, api.MaxWait)
	}
	deadline := time.Now().Add(d)
	for {
		out, holds, err := s.pull(ctx, in)
		if err != nil || out.Todo != nil || holds {
			return out, err
		}
		if done, err := s.finished(ctx, in); err != nil {
			return api.PullOutput{}, err
		} else if done {
			out.Ended = api.EndedFinished
			return out, nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			out.Ended = api.EndedTimeout
			return out, nil
		}
		select {
		case <-ctx.Done():
			out.Ended = api.EndedTimeout
			return out, nil
		case <-time.After(min(waitEvery, left)):
		}
	}
}

// finished reports a scope with nothing an agent can move: no agent to-do the
// pull's filter keeps is ready, held or reported (a waiting one behind a
// person's step does not count), in a module not switched off.
func (s *Service) finished(ctx context.Context, in api.PullInput) (bool, error) {
	f := in.Filter
	if in.ID != "" {
		got, err := s.Get(ctx, api.GetInput{ID: in.ID})
		if err != nil {
			return false, err
		}
		f = api.Filter{Module: got.Todo.Module, Project: got.Todo.Repository, Request: got.Todo.Request}
	}
	f.For = api.ForAgent
	all, modes, err := s.scheduled(ctx, "", false)
	if err != nil {
		return false, err
	}
	ms := newMemberships(ctx)
	for _, t := range all {
		if modeOf(modes, t) == api.ModeOff || !matches(t, f) {
			continue
		}
		switch t.State {
		case api.StateReady:
			// A ready to-do this worker may never take - its holding list's
			// members', or given to another - is not left for it.
			if in.By.ID != "" {
				if why, err := ms.mayTake(t, in.By); err != nil {
					return false, err
				} else if why != "" {
					continue
				}
			}
			return false, nil
		case api.StateClaimed, api.StateReported:
			return false, nil
		}
	}
	// A collect still running for a to-do of the scope may release its next
	// step: its owner archives the step first and releases what follows last
	// - a person's step included, whose answer releases an
	// agent's.
	f.For = ""
	running := false
	collecting.Range(func(_, v any) bool {
		if t, ok := v.(api.Todo); ok && matches(t, f) {
			running = true
		}
		return !running
	})
	return !running, nil
}

// pull is one look: the to-do taken, or why none was; holds says the agent
// already holds one (a wait does not wait then).
func (s *Service) pull(ctx context.Context, in api.PullInput) (api.PullOutput, bool, error) {
	out, err := s.pullOnce(ctx, in)
	return out.PullOutput, out.holds, err
}

// pullAnswer is one look's answer, and whether the agent already holds a
// to-do.
type pullAnswer struct {
	api.PullOutput
	holds bool
	named map[string]api.Todo // the waiting to-dos, for their owners' namers
}

// pullOnce is one look, in one hold of the store (Atomically): the held
// to-dos it reads, the module locks it checks and the claim it takes are one
// transaction - two pulls in different queues never both take a third module.
func (s *Service) pullOnce(ctx context.Context, in api.PullInput) (pullAnswer, error) {
	if err := checkActor(in.By); err != nil {
		return pullAnswer{}, err
	}
	var out pullAnswer
	err := s.store.Atomically(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.pullIn(ctx, in)
		return err
	})
	if err != nil {
		return out, err
	}
	// Named after the store's hold: a namer never runs inside it.
	nameWaits(ctx, &out.PullOutput, out.named)
	return out, nil
}

// nameWaits prefixes each waiting line with the label its to-do's owner's
// wait namer answers; the to-do of an owner with no namer keeps the
// scheduler's words. todos are the waiting to-dos, by id.
func nameWaits(ctx context.Context, out *api.PullOutput, todos map[string]api.Todo) {
	namers := application.UseExtensions[api.WaitNamer](ctx)
	if len(namers) == 0 {
		return
	}
	for i, w := range out.Waits {
		t, ok := todos[w.Todo]
		if !ok || i >= len(out.Waiting) {
			continue
		}
		for _, n := range namers {
			if n.Owner != t.Owner || n.Name == nil {
				continue
			}
			if label := n.Name(ctx, t, w); label != "" {
				out.Waiting[i] = t.ID + ": " + label + " - " + strings.TrimPrefix(out.Waiting[i], t.ID+": ")
			}
			break
		}
	}
}

// afterQueue, when a test sets it, runs between a pull's queue and its claim.
var afterQueue func()

// pullIn is pullOnce's look, in the store's hold.
func (s *Service) pullIn(ctx context.Context, in api.PullInput) (pullAnswer, error) {
	f := in.Filter
	if f.For == "" {
		f.For = api.ForAgent
		if in.By.Kind == "person" {
			f.For = ""
		}
	}
	// A worker asking by id for a to-do it holds - one a refused report left
	// with it - is answered it, still held.
	if in.ID != "" {
		if got, err := s.Get(ctx, api.GetInput{ID: in.ID}); err == nil && held(got.Todo) && same(got.Todo.Claim.By, in.By) && otherSession(got.Todo, callerSession(ctx)) == "" {
			t := got.Todo
			return pullAnswer{PullOutput: api.PullOutput{Todo: &t}}, nil
		}
	}
	// An agent holds one to-do at a time (OD-006): the internal worker, which
	// runs several, and people, who answer theirs in any order, are not bound.
	if in.By.Kind == "agent" && in.By.ID != api.Worker.ID {
		if h, err := s.holding(ctx, in.By); err != nil {
			return pullAnswer{}, err
		} else if h != nil {
			if other := otherSession(*h, callerSession(ctx)); other != "" {
				return pullAnswer{PullOutput: api.PullOutput{Nothing: in.By.ID + " holds " + h.ID + " from another session (" + other + "): that session reports or releases it - give this session its own actor name"}, holds: true}, nil
			}
			return pullAnswer{PullOutput: api.PullOutput{Nothing: in.By.ID + " holds " + h.ID + ": report it or release it first"}, holds: true}, nil
		}
	}
	if in.ID != "" {
		got, err := s.Get(ctx, api.GetInput{ID: in.ID})
		if err != nil {
			return pullAnswer{}, err
		}
		f = api.Filter{Module: got.Todo.Module, Project: got.Todo.Repository, Request: got.Todo.Request}
	}
	q, err := s.queue(ctx, f, in.Auto, in.Prefer, in.ID)
	if err != nil {
		return pullAnswer{}, err
	}
	if afterQueue != nil {
		afterQueue()
	}
	var out api.PullOutput
	named := map[string]api.Todo{}
	wait := func(t api.Todo, why string, w api.QueueWait) {
		w.Todo = t.ID
		out.Waiting = append(out.Waiting, t.ID+": "+why)
		out.Waits = append(out.Waits, w)
		named[t.ID] = t
	}
	ms := newMemberships(ctx)
	for _, e := range q {
		if in.ID != "" && e.Todo.ID != in.ID {
			continue
		}
		if !e.Takeable {
			w := api.QueueWait{}
			if e.Wait != nil {
				w = *e.Wait
			}
			wait(e.Todo, e.Waits, w)
			continue
		}
		// A holding list's to-do is its members', one given to a worker that
		// worker's.
		if why, w, err := ms.mayWait(e.Todo, in.By); err != nil {
			return pullAnswer{}, err
		} else if why != "" {
			wait(e.Todo, why, w)
			continue
		}
		if e.Todo.For == api.ForPerson && in.By.Kind != "person" {
			wait(e.Todo, "a person's to-do", api.QueueWait{Cause: api.WaitPerson})
			continue
		}
		t, err := s.claim(ctx, e.Todo.ID, in.By, callerSession(ctx), in.LeaseSeconds)
		if errors.Is(err, errTaken) {
			continue // another worker was first
		}
		if err != nil {
			return pullAnswer{}, err
		}
		out.Todo = &t
		return pullAnswer{PullOutput: out, named: named}, nil
	}
	switch {
	case in.ID != "" && len(out.Waiting) == 0:
		out.Nothing = in.ID + " is not ready"
	case len(out.Waiting) > 0:
		out.Nothing = "every ready to-do waits"
	default:
		out.Nothing = "nothing ready"
	}
	return pullAnswer{PullOutput: out, named: named}, nil
}

var errTaken = errors.New("taken")

// holding is the to-do a worker holds now (its lease not passed), nil when none.
func (s *Service) holding(ctx context.Context, by cwmapi.Actor) (*api.Todo, error) {
	all, _, err := s.scheduled(ctx, "", false)
	if err != nil {
		return nil, err
	}
	for _, t := range all {
		if held(t) && t.Claim.By.ID == by.ID && t.Claim.By.Kind == by.Kind {
			return &t, nil
		}
	}
	return nil, nil
}

// callerSession is the agent's session a call comes from (its cause), "" when
// none is known: a person, the internal worker, an older command.
func callerSession(ctx context.Context) string {
	if c, ok := cause.CauseFrom(ctx); ok && c.Kind == cause.KindAgent {
		return c.Session
	}
	return ""
}

// otherSession is the session holding t when it is not session - both known
// and different - else "": two sessions acting as one actor are told apart
// ; an unknown session on either side goes on as before.
func otherSession(t api.Todo, session string) string {
	if t.Claim == nil || t.Claim.Session == "" || session == "" || t.Claim.Session == session {
		return ""
	}
	return t.Claim.Session
}

// claim holds a to-do for a worker, from its session if known, if it is
// still pullable.
func (s *Service) claim(ctx context.Context, id string, by cwmapi.Actor, session string, leaseSeconds int) (api.Todo, error) {
	return s.store.Claim(ctx, id, func(t *api.Todo) error {
		if !pullable(*t) {
			return errTaken
		}
		t.State = api.StateClaimed
		t.Claim = &api.Claim{By: by, At: now(), Until: lease(leaseSeconds), Session: session}
		t.Result, t.Held = nil, nil
		return nil
	})
}

// Renew extends the lease of the to-do its worker holds.
func (s *Service) Renew(ctx context.Context, in api.RenewInput) (api.TodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.TodoOutput{}, err
	}
	var out api.Todo
	err := s.change(ctx, in.ID, func(t *api.Todo) error {
		if err := holder(*t, in.By, callerSession(ctx), "Renew"); err != nil {
			return err
		}
		t.Claim.Until = lease(in.LeaseSeconds)
		if in.Run != "" {
			t.Claim.Run, t.Claim.Session = in.Run, in.Session
		}
		out = *t
		return nil
	})
	return api.TodoOutput{Todo: out}, err
}

// Release gives a pulled to-do back.
func (s *Service) Release(ctx context.Context, in api.ReleaseInput) (api.TodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.TodoOutput{}, err
	}
	var out api.Todo
	err := s.change(ctx, in.ID, func(t *api.Todo) error {
		if err := lastHolder(*t, in.By, callerSession(ctx), "Release"); err != nil && in.By.Kind != "person" {
			return err
		}
		if t.State != api.StateClaimed {
			return invalid("Release: %s is %s, not pulled", t.ID, t.State)
		}
		t.State, t.Claim, t.Note = api.StateReady, nil, in.Note
		// Given to the worker handing it back: its holding list's again.
		if t.Assignee != nil && same(*t.Assignee, in.By) {
			t.Assignee = nil
		}
		out = *t
		return nil
	})
	if err == nil {
		wake(ctx)
	}
	return api.TodoOutput{Todo: out}, err
}

// Report hands a to-do's result back, then its owner collects it.
func (s *Service) Report(ctx context.Context, in api.ReportInput) (api.TodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.TodoOutput{}, err
	}
	if !slices.Contains([]string{api.StatusSuccess, api.StatusFail, api.StatusBlocked}, in.Status) {
		return api.TodoOutput{}, invalid("Report: status is success or fail or blocked, got %q", in.Status)
	}
	if strings.TrimSpace(in.Summary) == "" {
		return api.TodoOutput{}, invalid("Report: say what was done (summary)")
	}
	if in.Wait != nil {
		if in.Status != api.StatusBlocked {
			return api.TodoOutput{}, invalid("Report: a wait goes with a blocked report, not %s", in.Status)
		}
		if k := in.Wait.Kind; k != "" && !slices.Contains([]string{api.HeldObject, api.HeldPerson, api.HeldResource, api.HeldExternal}, k) {
			return api.TodoOutput{}, invalid("Report: a wait's kind is object or person or resource or external (or none: unclassified), got %q", k)
		}
	}
	// A person's report is the person's own: from an agent's session it is
	// refused, whatever by says — the session tells the person where to
	// answer (the Inbox, the request view) — unless it relays that person's
	// report, confirmed, with relaying on (iam's one rule); the result then
	// records the relay.
	var relayed *api.Relayed
	if in.By.Kind == "person" {
		r, err := person.AcceptRelayed(ctx, in.By.ID, "report:"+in.ID, "Report: "+in.ID+" is reported by the person themselves (the Inbox, the request view, or their own command)")
		if err != nil {
			return api.TodoOutput{}, invalid("%s", err.Error())
		}
		if r != nil {
			c, _ := cause.CauseFrom(ctx)
			relayed = &api.Relayed{By: c.ID, Confirmed: r.Confirmed, At: r.At}
		}
	}
	var out api.Todo
	session := callerSession(ctx)
	retried := false
	err := s.change(ctx, in.ID, func(t *api.Todo) error {
		switch {
		case t.State == api.StateClaimed && (in.By.Kind == "person" || lastHolder(*t, in.By, session, "Report") == nil):
		case t.State == api.StateReady && in.By.Kind == "person":
			// A person marks it done by hand.
		case t.State == api.StateClaimed:
			return lastHolder(*t, in.By, session, "Report")
		default:
			return invalid("Report: %s is %s — only a pulled to-do is reported (a person may report a ready one)", t.ID, t.State)
		}
		if t.For == api.ForPerson && in.By.Kind != "person" {
			return invalid("Report: %s is a person's to-do", t.ID)
		}
		r := in.Result
		r.At = now()
		r.Relayed = relayed
		// A summary is one line: a longer one keeps its first line, the rest
		// goes to its details (said in the details, nothing lost).
		r.Summary, r.Details = oneLine(r.Summary, r.Details)
		t.State, t.Result = api.StateReported, &r
		t.Held = nil
		if r.Status == api.StatusBlocked {
			t.Held = heldOn(in.Wait, r, in.By)
		}
		if r.Status == api.StatusFail {
			t.Attempts++
			// Attempts left: ready again after its backoff, its owner not
			// called — it collects the last result only.
			if t.Attempts < t.MaxAttempts {
				t.State, t.Claim, t.NotBefore = api.StateReady, nil, retryAt(*t)
				t.Note = fmt.Sprintf("attempt %d of %d failed: %s", t.Attempts, t.MaxAttempts, r.Summary)
				retried = true
			}
		}
		out = *t
		return nil
	})
	if err != nil {
		return api.TodoOutput{}, err
	}
	defer wake(ctx)
	if retried {
		return api.TodoOutput{Todo: out, Note: out.Note + "; retried after its backoff"}, nil
	}
	note, err := collect(ctx, out)
	if err != nil {
		return api.TodoOutput{Todo: out}, fmt.Errorf("Report: %s reported, but its owner did not collect it: %w", out.ID, err)
	}
	return api.TodoOutput{Todo: out, Note: note}, nil
}

// oneLine splits a summary written on several lines: its first line stays the
// summary, the rest opens the details (before any details given).
func oneLine(summary, details string) (string, string) {
	summary = strings.TrimSpace(summary)
	first, rest, multi := strings.Cut(summary, "\n")
	if !multi {
		return summary, details
	}
	rest = strings.TrimSpace(rest)
	if details = strings.TrimSpace(details); details != "" {
		rest += "\n\n" + details
	}
	return strings.TrimSpace(first), rest
}

// collecting holds the to-dos whose collect runs in this process (id -> the
// to-do): a tick never starts a second one beside it, and a wait whose scope
// holds one is not finished.
var collecting sync.Map

// collect hands a reported to-do to its owner's collectors.
func collect(ctx context.Context, t api.Todo) (string, error) {
	if _, busy := collecting.LoadOrStore(t.ID, t); busy {
		return "its collect is running", nil
	}
	defer collecting.Delete(t.ID)
	var notes []string
	for _, c := range application.UseExtensions[api.Collector](ctx) {
		if c.Owner != t.Owner || c.Collect == nil {
			continue
		}
		if err := c.Collect(ctx, t); err != nil {
			return "", err
		}
		notes = append(notes, "collected by "+c.Owner)
	}
	if len(notes) == 0 {
		return "no collector for " + t.Owner + ": it stays reported", nil
	}
	return strings.Join(notes, "; "), nil
}

// holder refuses a worker that does not hold a to-do.
func holder(t api.Todo, by cwmapi.Actor, session, verb string) error {
	if err := lastHolder(t, by, session, verb); err != nil {
		return err
	}
	if !held(t) {
		return invalid("%s: %s — its lease passed; pull it again", verb, t.ID)
	}
	return nil
}

// lastHolder refuses a worker that is not the to-do's last holder. Its lease
// may have passed: while no one pulled it again (a pull replaces the claim),
// the work it finished is still its own to report or give back — a report and
// a release write only the scheduler's files, never the module another
// worker may hold meanwhile.
func lastHolder(t api.Todo, by cwmapi.Actor, session, verb string) error {
	if t.State != api.StateClaimed || t.Claim == nil {
		return invalid("%s: %s is %s, not pulled", verb, t.ID, t.State)
	}
	if t.Claim.By.ID != by.ID || t.Claim.By.Kind != by.Kind {
		return invalid("%s: %s is held by %s, not %s", verb, t.ID, t.Claim.By.ID, by.ID)
	}
	if other := otherSession(t, session); other != "" {
		return invalid("%s: %s is held by %s from another session (%s), not this one (%s) - give this session its own actor name", verb, t.ID, by.ID, other, session)
	}
	return nil
}

func lease(seconds int) string {
	if seconds <= 0 {
		seconds = api.DefaultLeaseSeconds
	}
	return time.Now().UTC().Add(time.Duration(seconds) * time.Second).Format(time.RFC3339Nano)
}

// --- the owner's verbs -----------------------------------------------------------

// Put writes a to-do as its owner gives it: new when it has no id. One a worker
// holds is not written over.
func (s *Service) Put(ctx context.Context, in api.PutInput) (api.TodoOutput, error) {
	t := in.Todo
	switch {
	case t.Module == "" || t.Owner == "" || t.Title == "":
		return api.TodoOutput{}, invalid("Put: a to-do has a module, an owner and a title")
	case t.For != api.ForAgent && t.For != api.ForPerson:
		return api.TodoOutput{}, invalid("Put: for is agent or person, got %q", t.For)
	case t.Start != api.StartAuto && t.Start != api.StartManual:
		return api.TodoOutput{}, invalid("Put: start is auto or manual, got %q", t.Start)
	case rank(t.Priority) == len(priorities):
		return api.TodoOutput{}, invalid("Put: priority is one of %v, got %q", priorities, t.Priority)
	case t.State != "" && t.State != api.StateWaiting && t.State != api.StateReady && t.State != api.StateReported:
		return api.TodoOutput{}, invalid("Put: its owner writes a to-do waiting or ready (or keeps it reported), got %q", t.State)
	case t.Rank < 0:
		return api.TodoOutput{}, invalid("Put: rank is 1 or more (0: unset), got %d", t.Rank)
	case slices.Contains(t.Labels, ""):
		return api.TodoOutput{}, invalid("Put: a label is not empty")
	}
	if t.State == "" {
		t.State = api.StateWaiting
	}
	if t.ID == "" {
		defer wake(ctx)
		out, err := s.store.Create(ctx, t)
		return api.TodoOutput{Todo: out}, err
	}
	if m, err := moduleOfID(t.ID); err != nil || m != bareModule(t.Module) {
		return api.TodoOutput{}, invalid("Put: %s is not a to-do of %s", t.ID, t.Module)
	}
	defer wake(ctx)
	put, err := s.store.Update(ctx, t.ID, func(cur *api.Todo) error {
		if held(*cur) {
			return invalid("Put: %s is held by %s", cur.ID, cur.Claim.By.ID)
		}
		// A report its owner refuses to collect sends the to-do back to the
		// queue: while its reporter's lease runs it stays the reporter's, who
		// reports it again - never another worker's to redo.
		if keep := reporterClaim(*cur); keep != nil && t.State == api.StateReady && t.Claim == nil {
			t.State, t.Claim = api.StateClaimed, keep
		}
		// Its project and its module are the store's key: kept as the row
		// holds them.
		t.Made, t.Module, t.Repository = cur.Made, cur.Module, cur.Repository
		*cur = t
		return nil
	})
	if err != nil {
		return api.TodoOutput{Todo: t}, err
	}
	return api.TodoOutput{Todo: put}, nil
}

// Archive moves a to-do to done.
func (s *Service) Archive(ctx context.Context, in api.ArchiveInput) (api.TodoOutput, error) {
	if _, err := moduleOfID(in.ID); err != nil {
		return api.TodoOutput{}, err
	}
	out, err := s.store.Archive(ctx, in.ID, func(t *api.Todo) error {
		if t.State == api.StateClaimed && held(*t) {
			return invalid("Archive: %s is held by %s", t.ID, t.Claim.By.ID)
		}
		t.Archived = now()
		if in.Note != "" {
			t.Note = in.Note
		}
		return nil
	})
	if err == nil {
		wake(ctx)
	}
	return api.TodoOutput{Todo: out}, err
}

// change rewrites one scheduled to-do in one hold of the store.
func (s *Service) change(ctx context.Context, id string, fn func(t *api.Todo) error) error {
	_, err := s.store.Update(ctx, id, fn)
	return err
}

// --- modes and keeping -------------------------------------------------------------

// Mode sets a module's scheduling mode.
func (s *Service) Mode(ctx context.Context, in api.ModeInput) (api.ModeOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ModeOutput{}, err
	}
	if !slices.Contains([]string{api.ModeManual, api.ModeAuto, api.ModeOff}, in.Mode) {
		return api.ModeOutput{}, invalid("Mode: manual or auto or off, got %q", in.Mode)
	}
	space, err := planningSpace(ctx, in.Module)
	if err != nil {
		return api.ModeOutput{}, err
	}
	st, err := settings(space)
	if err != nil {
		return api.ModeOutput{}, err
	}
	st.Scheduling = in.Mode
	if err := writeSettings(space, st); err != nil {
		return api.ModeOutput{}, err
	}
	wake(ctx)
	startOnAuto(ctx, in.Mode)
	return api.ModeOutput{Module: in.Module, Mode: in.Mode}, nil
}

// Modes answers each module's scheduling mode.
func (s *Service) Modes(ctx context.Context, _ api.ModesInput) (api.ModesOutput, error) {
	mods, err := modules(ctx)
	if err != nil {
		return api.ModesOutput{}, err
	}
	cfg, _ := application.UseConfig[api.SchedulerConfig](ctx)
	out := api.ModesOutput{Scheduler: cfg.Enabled(), Modules: []api.ModeOutput{}}
	for _, m := range mods {
		space, err := planningSpace(ctx, m)
		if err != nil {
			return api.ModesOutput{}, err
		}
		st, err := settings(space)
		if err != nil {
			return api.ModesOutput{}, err
		}
		out.Modules = append(out.Modules, api.ModeOutput{Module: m, Mode: st.Scheduling})
	}
	sort.Slice(out.Modules, func(i, j int) bool { return out.Modules[i].Module < out.Modules[j].Module })
	return out, nil
}

// Prune removes the done to-dos past their module's keeping time - of the
// module named alone (oneModule), of every served project's modules else. On the
// SQLite store a done to-do leaves only once an export holds it: the
// repositories of the to-dos to remove are exported first, and a to-do whose
// repository's export failed is kept for a later prune.
func (s *Service) Prune(ctx context.Context, in api.PruneInput) (api.PruneOutput, error) {
	if err := oneModule(ctx, in.Module); err != nil {
		return api.PruneOutput{}, err
	}
	mods, err := modules(ctx)
	if err != nil {
		return api.PruneOutput{}, err
	}
	var out api.PruneOutput
	var past []api.Todo
	for _, m := range mods {
		if in.Module != "" && !sameModule(m, in.Module) {
			continue
		}
		space, err := planningSpace(ctx, m)
		if err != nil {
			return out, err
		}
		st, err := settings(space)
		if err != nil {
			return out, err
		}
		done, err := s.store.List(ctx, m, true)
		if err != nil {
			return out, err
		}
		limit := time.Now().UTC().AddDate(0, 0, -st.DoneDays)
		for _, t := range done {
			at, err := time.Parse(time.RFC3339Nano, t.Archived)
			if err != nil || at.After(limit) {
				continue
			}
			past = append(past, t)
		}
	}
	kept := func(api.Todo) bool { return true }
	var exportErr error
	if d, ok := innerOf(s.store).(dumper); ok && len(past) > 0 {
		repos := map[string]bool{}
		for _, t := range past {
			repos[t.Repository] = true
		}
		var exported map[string]bool
		if exported, exportErr = exportBeforePrune(ctx, d, repos); exported == nil {
			return out, exportErr
		}
		kept = func(t api.Todo) bool { return exported[t.Repository] }
	}
	for _, t := range past {
		if !kept(t) {
			out.Kept++
			continue
		}
		if err := s.store.RemoveDone(ctx, t.ID); err != nil {
			return out, err
		}
		out.Removed++
	}
	return out, exportErr
}

// heldOn is the structured wait a blocked report writes on its to-do: the one
// it gave, or an unclassified one whose reason is its summary.
func heldOn(w *api.HeldOn, r api.Result, by cwmapi.Actor) *api.HeldOn {
	h := api.HeldOn{Reason: r.Summary}
	if w != nil {
		h.Kind, h.Target = w.Kind, w.Target
		if strings.TrimSpace(w.Reason) != "" {
			h.Reason = strings.TrimSpace(w.Reason)
		}
	}
	h.Since, h.By = r.At, by
	return &h
}
