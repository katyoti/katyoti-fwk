// SPDX-License-Identifier: Apache-2.0

// Package events implements CWM's `events.*` slot — emit on
// every mutating substrate write + a thin Subscribe wrapper that
// returns a typed `<-chan api.EventEnvelope`.
//
// v1 sits on top of the in-process `internal/core/eventbus`
// EventBus. NATS production wiring lands in a follow-up alongside
// the KTSP server-stream adapter; the subject format already
// matches the NATS hierarchy so the producer side ports cleanly.
//
// Emit set: object.<token>.created / .updated / .deleted on every object
// write, object.<token>.moved / .split / .transitioned / .commented /
// .transition-refused for the operations that report themselves, and
// object.<token>.event for an event raised on an object (EventRaise), and
// project.caught-up for a catch-up of what moved outside the verbs.
//
// Deferred to v2:
//   - shared-state.updated / .completed
//   - spawn.completed (needs budget service hook)
//   - health.score.changed (needs threshold cache)
//   - KTSP server-stream bridge
//   - NATS wildcard patterns in topic filters (in-process bus
//     uses exact + prefix-`.>` match for v1)
package events

import (
	stdctx "context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Service is the CWM events sub-service. Constructed by core.New
// with the workspace's scope identifiers and a callable bus
// fetcher.
type Service struct {
	tenant   string
	universe string
	// workspace is the workspace slot: the project's token (api.WorkspaceToken),
	// set again when the project's id is minted after the service opened
	// (SetWorkspace) - a string, read on every publish.
	workspace atomic.Value
	bus       eventbus.EventBus
	// mb is the cross-sidecar NATS bus. cwm-v2-providers US-15
	// (2026-06-08): when non-nil, every Publish* fires on NATS in
	// addition to the in-process EventBus so EventService subscribers
	// (`internal/extensions/events/`) receive lifecycle events. The
	// in-process publish stays for existing same-process consumers.
	mb mbapi.MessageBus
	// own are the observers of THIS service's events (ObserveOwn): handed
	// each event this service emits, never another store's.
	ownMu sync.RWMutex
	own   []*ownIntake
}

// ownIntake is one observer's intake of its service's own events.
type ownIntake struct {
	take func(topic string, payload any)
}

// New constructs an events Service. If bus is nil, in-process publish
// calls become no-ops + every Subscribe returns an empty
// (already-closed) channel. If mb is nil, NATS publish is skipped —
// in-process publish still works. The substrate must run cleanly
// in test contexts where neither bus is wired.
func New(tenantID, universeID, workspaceID string, bus eventbus.EventBus, mb mbapi.MessageBus) *Service {
	slog.Info("cwm events: publisher scope",
		"tenant", tenantID, "universe", universeID, "workspace", workspaceID,
		"natsBus", mb != nil, "inProcBus", bus != nil)
	s := &Service{
		tenant:   tenantID,
		universe: universeID,
		bus:      bus,
		mb:       mb,
	}
	s.workspace.Store(workspaceID)
	return s
}

// Workspace is the workspace slot every subject this service publishes carries.
func (s *Service) Workspace() string {
	if s == nil {
		return ""
	}
	ws, _ := s.workspace.Load().(string)
	return ws
}

// SetWorkspace changes the workspace slot of the subjects published from now
// on: a project whose id is minted while the service is open (ProjectInit)
// publishes under that id at once, not under the slot it opened with.
func (s *Service) SetWorkspace(workspaceID string) {
	if s == nil || workspaceID == "" {
		return
	}
	if s.Workspace() != workspaceID {
		slog.Info("cwm events: publisher workspace", "tenant", s.tenant, "universe", s.universe, "workspace", workspaceID)
	}
	s.workspace.Store(workspaceID)
}

// publishToNATS is the cross-sidecar leg of every PublishX. Best-
// effort fire-and-forget — encode payload to JSON, hit the bus,
// swallow errors (callers can't undo the in-process publish that
// already succeeded).
func (s *Service) publishToNATS(subject string, payload any) {
	if s == nil || s.mb == nil {
		// No bus is the in-process case (a test, a program with no hub):
		// Debug, so it does not bury what a test run prints
		//
		slog.Debug("cwm events: NATS publish skipped (no bus)", "subject", subject)
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := s.mb.Publish(stdctx.Background(), subject, body); err != nil {
		slog.Warn("cwm events: NATS publish failed", "subject", subject, "error", err)
		return
	}
	slog.Info("cwm events: NATS published", "subject", subject)
}

// --- Publish helpers --------------------------------------------------------
//
// One helper per event class. Each constructs the subject via the
// public api.*Subject builder (which validates tokens) and
// publishes the typed payload. Callers don't touch the bus
// directly.

// recoverPublish makes event emission BEST-EFFORT: a malformed data id (a
// non-kebab itemID/nodeID — e.g. an uppercase or `#`-bearing id that slipped
// through item creation) makes the subject builders' `cwmMustToken` panic. An
// event publish must NEVER crash the mutating task that triggered it, so every
// Publish* method defers this guard — the bad event is dropped (logged) and the
// write proceeds. The malformed id itself is a separate bug to fix at the source.
func (s *Service) recoverPublish() {
	if r := recover(); r != nil {
		recoveredPanics.Add(1)
		slog.Warn("cwm: dropped event publish (invalid subject token)", "panic", r)
	}
}

// recoveredPanics counts every panic this package recovered — in a publish or
// in one of its handlers. Tests read it to prove a path never panics.
var recoveredPanics atomic.Int64

// emit sends one built event on both legs. The in-process leg runs under its
// own recover, so a handler that panics there never costs the NATS leg.
func (s *Service) emit(subject string, payload any) {
	if s.bus != nil {
		s.ownMu.RLock()
		own := s.own
		s.ownMu.RUnlock()
		for _, o := range own {
			guardHandler(o.take)(subject, payload)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					recoveredPanics.Add(1)
					slog.Warn("cwm: an in-process event handler panicked", "subject", subject, "panic", r)
				}
			}()
			_ = s.bus.Publish(subject, payload)
		}()
	}
	s.publishToNATS(subject, payload)
}

// guardHandler runs a handler this package registers under a recover, so it
// can never stop the bus's loop over the other handlers of a publish.
func guardHandler(h func(string, any)) func(string, any) {
	return func(topic string, payload any) {
		defer func() {
			if r := recover(); r != nil {
				recoveredPanics.Add(1)
				slog.Warn("cwm: an event subscriber panicked", "subject", topic, "panic", r)
			}
		}()
		h(topic, payload)
	}
}

// PublishObject emits an object write — `object.<token>.<action>` — with the
// object's id and module in the payload, so a subscriber (the journal) knows
// WHICH object without reversing the token. action: created | updated | deleted.
//
// operation marks a write an operation's own event reports (api.OperationMove,
// api.OperationSplit); meta is what the event says about the write itself
// (its id, time, revision and cause).
func (s *Service) PublishObject(action, objectID, module, typ, parent, slug, operation string, meta api.EventMeta) {
	if s == nil {
		return
	}
	defer s.recoverPublish()
	token := api.SubjectToken(objectID)
	var subject string
	switch action {
	case "created":
		subject = api.ObjectCreatedSubject(s.tenant, s.universe, s.Workspace(), token)
	case "updated":
		subject = api.ObjectUpdatedSubject(s.tenant, s.universe, s.Workspace(), token)
	case "deleted":
		subject = api.ObjectDeletedSubject(s.tenant, s.universe, s.Workspace(), token)
	default:
		return
	}
	payload := ObjectPayload{EventMeta: meta, ObjectID: objectID, Module: module, Type: typ, Parent: parent, Slug: slug, Action: action, Operation: operation}
	s.emit(subject, payload)
}

// PublishObjectEvent emits `object.<token>.event`, one per EventRaise: an
// event of a declared type raised on an object, which it does not change.
func (s *Service) PublishObjectEvent(p api.ObjectRaisedEventPayload) {
	if s == nil {
		return
	}
	defer s.recoverPublish()
	subject := api.ObjectEventRaisedSubject(s.tenant, s.universe, s.Workspace(), api.SubjectToken(p.ObjectID))
	s.emit(subject, p)
}

// PublishObjectTransitioned emits `object.<token>.transitioned`.
func (s *Service) PublishObjectTransitioned(p ObjectTransitionedPayload) {
	if s == nil {
		return
	}
	defer s.recoverPublish()
	subject := api.ObjectTransitionedSubject(s.tenant, s.universe, s.Workspace(), api.SubjectToken(p.ObjectID))
	s.emit(subject, p)
}

// PublishObjectTransitionRefused emits `object.<token>.transition-refused`,
// one per move its guard refused.
func (s *Service) PublishObjectTransitionRefused(p ObjectTransitionRefusedPayload) {
	if s == nil {
		return
	}
	defer s.recoverPublish()
	subject := api.ObjectTransitionRefusedSubject(s.tenant, s.universe, s.Workspace(), api.SubjectToken(p.ObjectID))
	s.emit(subject, p)
}

// PublishObjectMoved emits the ONE `object.<token>.moved` event of a move.
func (s *Service) PublishObjectMoved(p ObjectMovedPayload) {
	if s == nil {
		return
	}
	defer s.recoverPublish()
	subject := api.ObjectMovedSubject(s.tenant, s.universe, s.Workspace(), api.SubjectToken(p.ObjectID))
	s.emit(subject, p)
}

// PublishObjectSplit emits the ONE `object.<token>.split` event of a split.
func (s *Service) PublishObjectSplit(p ObjectSplitPayload) {
	if s == nil {
		return
	}
	defer s.recoverPublish()
	subject := api.ObjectSplitSubject(s.tenant, s.universe, s.Workspace(), api.SubjectToken(p.ObjectID))
	s.emit(subject, p)
}

// PublishObjectArchived emits the ONE `object.<token>.archived` event of an
// archive.
func (s *Service) PublishObjectArchived(p ObjectArchivedPayload) {
	if s == nil {
		return
	}
	defer s.recoverPublish()
	subject := api.ObjectArchivedSubject(s.tenant, s.universe, s.Workspace(), api.SubjectToken(p.ObjectID))
	s.emit(subject, p)
}

// PublishCaughtUp emits the ONE `project.caught-up` event of a catch-up that
// found objects moved outside the verbs.
func (s *Service) PublishCaughtUp(p ObjectsCaughtUpPayload) {
	if s == nil {
		return
	}
	defer s.recoverPublish()
	s.emit(api.ProjectCaughtUpSubject(s.tenant, s.universe, s.Workspace()), p)
}

// --- Subscribe --------------------------------------------------------------

// Subscribe wraps eventbus subscription with a typed channel +
// pattern-aware matching:
//
//   - Exact pattern: "ktw.t.universe.u.workspace.W.object.<token>.created"
//     receives only that subject.
//   - Prefix pattern ending in ".>" (NATS-style):
//     "ktw.t.universe.u.workspace.W.object.>" matches every object
//     subject under the workspace.
//
// Other NATS wildcards (`*` mid-pattern) are NOT supported in v1
// — the substrate uses the in-process EventBus which doesn't
// implement them. When the NATS bridge ships in v2 the wildcard
// surface widens automatically.
//
// Returns the typed receive channel + a cancel function. Callers
// MUST call cancel() to release the subscription; otherwise the
// in-process bus retains the handler until process exit.
func (s *Service) Subscribe(_ stdctx.Context, in api.EventsSubscribeInput) (<-chan api.EventEnvelope, func(), error) {
	if s == nil || s.bus == nil {
		ch := make(chan api.EventEnvelope)
		close(ch)
		return ch, func() {}, nil
	}
	ch := make(chan api.EventEnvelope, 64) // small in-process buffer
	pattern := in.TopicPattern
	// mu + closed: the bus calls handlers from a snapshot of its list, so a
	// publish already under way can run this handler AFTER cancel. The send
	// and the close happen under the same lock, and a send after the close is
	// dropped — the channel is never closed while a send can run.
	var (
		mu     sync.Mutex
		closed bool
	)

	handler := func(topic string, payload any) {
		if !subjectMatches(pattern, topic) {
			return
		}
		env := api.EventEnvelope{Topic: topic, Payload: payloadToRaw(payload)}
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		// Best-effort delivery. Drop if the consumer is slow —
		// matches the existing KTSP-streaming backpressure
		// pattern. The send never blocks, so holding mu is brief.
		select {
		case ch <- env:
		default:
		}
	}

	// Subscribe to "any" rather than a single topic so the pattern
	// match is consistent regardless of the underlying bus's
	// exact-topic semantics.
	unsub := s.bus.SubscribeAny(guardHandler(handler))
	cancel := func() {
		unsub()
		mu.Lock()
		defer mu.Unlock()
		if !closed {
			closed = true
			close(ch)
		}
	}
	return ch, cancel, nil
}

// subjectMatches implements the v1 pattern semantics: exact OR
// prefix-`.>`. Empty pattern matches everything.
func subjectMatches(pattern, subject string) bool {
	if pattern == "" {
		return true
	}
	if strings.HasSuffix(pattern, ".>") {
		prefix := strings.TrimSuffix(pattern, ".>")
		return strings.HasPrefix(subject, prefix+".") || subject == prefix
	}
	return pattern == subject
}

// Observe delivers every event matching pattern to fn, IN ORDER and WITHOUT
// LOSS — unlike Subscribe, whose small buffer drops under load. The bus hands
// each event to an unbounded queue synchronously; fn runs on the observer's
// own goroutine, so it may call CWM back (the write that published has
// returned by then). The returned stop discards what is still queued and
// RETURNS ONCE THE GOROUTINE HAS ENDED — a handler already running has
// returned — so nothing the observer does lands after it: a workspace closed
// is not written into. The wait is bounded (observerStopWait) so a handler
// that itself stops its observer cannot hang the close; that is logged.
func (s *Service) Observe(pattern string, fn func(api.EventEnvelope)) (stop func()) {
	return s.ObserveDrainable(pattern, fn).Stop
}

// Observer is one running observer (ObserveDrainable). Drain lets it finish
// what it already received; Stop ends it, discarding what is still queued.
type Observer struct {
	pattern  string
	unsub    func()
	mu       sync.Mutex
	cond     *sync.Cond
	queue    []api.EventEnvelope
	draining bool // takes no new event; the goroutine ends once the queue is empty
	closed   bool // takes no new event and handles nothing more
	done     chan struct{}
	intake   sync.Once
}

// ObserveDrainable is Observe returning the observer itself, so a shutdown
// can DRAIN it — handle what was already published — before stopping it.
func (s *Service) ObserveDrainable(pattern string, fn func(api.EventEnvelope)) *Observer {
	return s.observe(pattern, fn, func(take func(string, any)) func() {
		return s.bus.SubscribeAny(guardHandler(take))
	})
}

// ObserveOwn is ObserveDrainable over THIS service's events only: the events
// it emits, never what another service publishes on the same in-process bus.
// Several stores served in one process share that bus; an
// observer attached to one store hears that store's writes, each once. Like
// ObserveDrainable it hears nothing when the service has no in-process bus.
func (s *Service) ObserveOwn(pattern string, fn func(api.EventEnvelope)) *Observer {
	return s.observe(pattern, fn, func(take func(string, any)) func() {
		in := &ownIntake{take: take}
		s.ownMu.Lock()
		s.own = append(append([]*ownIntake(nil), s.own...), in)
		s.ownMu.Unlock()
		return func() {
			s.ownMu.Lock()
			defer s.ownMu.Unlock()
			kept := make([]*ownIntake, 0, len(s.own))
			for _, o := range s.own {
				if o != in {
					kept = append(kept, o)
				}
			}
			s.own = kept
		}
	})
}

// observe runs one observer: subscribe registers its intake and answers how
// to unregister it.
func (s *Service) observe(pattern string, fn func(api.EventEnvelope), subscribe func(take func(string, any)) func()) *Observer {
	o := &Observer{pattern: pattern, done: make(chan struct{})}
	o.cond = sync.NewCond(&o.mu)
	if s == nil || s.bus == nil {
		o.unsub = func() {}
		close(o.done)
		return o
	}
	o.unsub = subscribe(func(topic string, payload any) {
		if !subjectMatches(pattern, topic) {
			return
		}
		o.mu.Lock()
		if !o.closed && !o.draining {
			o.queue = append(o.queue, api.EventEnvelope{Topic: topic, Timestamp: time.Now().UTC(), Payload: payloadToRaw(payload)})
			o.cond.Signal()
		}
		o.mu.Unlock()
	})
	go func() {
		defer close(o.done)
		for {
			o.mu.Lock()
			for len(o.queue) == 0 && !o.closed && !o.draining {
				o.cond.Wait()
			}
			if o.closed || len(o.queue) == 0 { // closed, or drained dry
				o.mu.Unlock()
				return
			}
			ev := o.queue[0]
			o.queue = o.queue[1:]
			o.mu.Unlock()
			fn(ev)
		}
	}()
	return o
}

// EndIntake makes the observer take no new event; what it already received
// is still handled (unless Stop discards it). Idempotent.
func (o *Observer) EndIntake() {
	o.intake.Do(func() {
		o.unsub()
		o.mu.Lock()
		o.draining = true
		o.cond.Broadcast()
		o.mu.Unlock()
	})
}

// Drain ends the intake and returns once every event already received has
// been handled — the observer has ended — or when ctx ends, or after
// observerStopWait (logged). The service the handlers write into is still
// open: this is what lets a one-shot command's own write be recorded before
// the process closes its services.
func (o *Observer) Drain(ctx stdctx.Context) {
	o.EndIntake()
	o.wait(ctx, "drain")
}

// Stop ends the intake, discards what is still queued and returns once the
// observer has ended (a running handler has returned), bounded by
// observerStopWait (logged). Nothing the observer does lands after it.
func (o *Observer) Stop() {
	o.EndIntake()
	o.mu.Lock()
	o.closed = true
	o.queue = nil
	o.cond.Broadcast()
	o.mu.Unlock()
	o.wait(stdctx.Background(), "stop")
}

func (o *Observer) wait(ctx stdctx.Context, phase string) {
	t := time.NewTimer(observerStopWait)
	defer t.Stop()
	select {
	case <-o.done:
	case <-ctx.Done():
	case <-t.C:
		slog.Warn("cwm: an event observer did not end within the stop wait — its handler is still running",
			"pattern", o.pattern, "phase", phase, "wait", observerStopWait)
	}
}

// observerStopWait bounds how long stopping an observer waits for its running
// handler to return.
const observerStopWait = 30 * time.Second
