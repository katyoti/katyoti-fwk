// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// Subscribe binds an Endpoint's slots to MessageBus subjects so a remote
// ktsp.Sender (the messagebus.Sender from sender.go) reaches them via
// Request/Publish on the canonical named-slot scheme:
//
//	ktw.<tenant>.universe.<u>.slot.<provider>.<slottype>.<instance>.<method>
//
// `provider` is `selfProvider` (typically the binary's `AppConfig.AppName`).
// `instance` is now ANY name — one subscription per slot type covers
// every instance registered on the endpoint via `AddSlot`.
// The dispatch handler parses the instance token off the subject and
// forwards to `Endpoint.HandleInstance(slot, instance, method, …)`,
// which returns a typed `ErrSlotNotFound(slot(instance))` if no
// handler is bound.
//
// Queue group is `slot:<provider>.<slottype>` (instance-agnostic) so
// true replicas of the same provider load-balance correctly across all
// instances they host. Different sidecars publishing the same slot
// type stay isolated by the `<provider>` token already present in the
// subject.
//
// Delivery: the transport hands a subscription's messages to one
// callback at a time, so a slot served inline answers one request at a
// time — and a handler that calls back into its own slot (CWM evaluating
// a guard whose function, served by a sidecar, reads the object back
// from CWM) waits for itself until the request times out. Each slot's
// requests are therefore served CONCURRENTLY by a bounded pool: up to
// DefaultMaxConcurrent at once (WithMaxConcurrent changes the default,
// the slot tag `max-concurrent` sets one slot's bound). When the pool is
// full the transport callback waits for a free worker, so excess
// requests queue in the transport's pending buffer, in arrival order. A
// slot whose handlers need its requests one at a time, in arrival order,
// says so with the slot tag `ordered: "true"` (ktsp.TagSlotOrdered) and
// is served inline, as before. Notifications (Sender.Notify: no reply
// expected) are served inline on every slot, so they keep their order.
//
// The returned SubscriptionGroup tracks every per-slot subscription
// and fans Drain / Unsubscribe across them.
func Subscribe(
	ctx context.Context,
	bus mbapi.MessageBus,
	tenantID, universeID, selfProvider string,
	endpoint *ktsp.Endpoint,
	opts ...SubscribeOption,
) (*SubscriptionGroup, error) {
	o := subscribeOptions{maxConcurrent: DefaultMaxConcurrent}
	for _, opt := range opts {
		opt(&o)
	}
	if selfProvider == "" {
		return nil, fmt.Errorf("messagebus.Subscribe: selfProvider is required (typically the binary's AppConfig.AppName)")
	}
	info := endpoint.Info()
	g := &SubscriptionGroup{}
	// The calls this subscription serves, by id, and the subject their
	// cancels come on: every replica listens, no queue group (cancel.go).
	calls := newCallRegistry()
	replay := newReplayCache() // the answers given, by request id (replay.go)
	cancelSub, err := bus.Subscribe(ctx, subjects.SlotCancelSubject(tenantID, universeID, selfProvider), calls.cancelHandler())
	if err != nil {
		return nil, fmt.Errorf("messagebus: subscribe the cancels of %s: %w", selfProvider, err)
	}
	g.cancelSub = cancelSub
	for _, slotInfo := range info.Slots {
		slottype := slotInfo.Name
		// One subscription per slot type, wildcarding BOTH the instance
		// and the method tokens. The dispatch handler reads the instance
		// off the subject and forwards to Endpoint.HandleInstance so a
		// single subscription serves every (instance, method) pair on
		// this slot type — including instances added at runtime via
		// AddSlot AFTER Subscribe returns.
		pattern := slotMethodPattern(tenantID, universeID, selfProvider, slottype)
		group := slotQueueGroup(selfProvider, slottype)

		handler := serveWith(deliveryOf(slotInfo.Tags, o.maxConcurrent),
			dispatchHandler(ctx, endpoint, selfProvider, calls, replay, bus.Publish))
		sub, err := bus.QueueSubscribe(ctx, pattern, group, handler)
		if err != nil {
			_ = g.Unsubscribe()
			return nil, fmt.Errorf("messagebus: subscribe slot %s/%s: %w",
				selfProvider, slottype, err)
		}
		g.add(slottype, sub)
	}
	return g, nil
}

// DefaultMaxConcurrent is how many requests one slot serves at once
// unless WithMaxConcurrent or the slot's `max-concurrent` tag says
// otherwise. Slot requests mostly wait on I/O (a file, another slot, an
// LLM), so the bound is about memory and fairness, not CPU.
const DefaultMaxConcurrent = 64

// SubscribeOption configures Subscribe.
type SubscribeOption func(*subscribeOptions)

type subscribeOptions struct {
	maxConcurrent int
}

// WithMaxConcurrent sets how many requests each slot serves at once when
// its tags do not say (n < 1 keeps the default). One serves every slot
// inline, one request at a time.
func WithMaxConcurrent(n int) SubscribeOption {
	return func(o *subscribeOptions) {
		if n >= 1 {
			o.maxConcurrent = n
		}
	}
}

// deliveryOf is how many requests a slot serves at once: one when it is
// tagged ordered, its `max-concurrent` tag when valid, else def.
func deliveryOf(tags ktsp.SlotTags, def int) int {
	if tags[ktsp.TagSlotOrdered] == "true" {
		return 1
	}
	if v, err := strconv.Atoi(tags[ktsp.TagSlotMaxConcurrent]); err == nil && v >= 1 {
		return v
	}
	return def
}

// serveWith wraps a dispatch handler in a bounded worker pool of size n.
// n == 1 serves inline, on the transport's callback: one request at a
// time, in arrival order. Otherwise the callback takes a worker slot
// (waiting while all n are busy, which leaves later requests queued in
// the transport, in order) and serves the request on its own goroutine.
// A notification (no reply subject: nobody waits for it, so nothing can
// wait on it either) is always served inline, so a slot's notifications
// keep the order they were sent in.
func serveWith(n int, h mbapi.Handler) mbapi.Handler {
	if n <= 1 {
		return h
	}
	workers := make(chan struct{}, n)
	return func(ctx context.Context, msg mbapi.Message) {
		if msg.Reply() == "" {
			h(ctx, msg)
			return
		}
		workers <- struct{}{}
		go func() {
			defer func() { <-workers }()
			h(ctx, msg)
		}()
	}
}

// slotMethodPattern returns the wildcard subject covering every
// (instance, method) pair on a (provider, slottype):
//
//	ktw.<tenant>.universe.<u>.slot.<provider>.<slottype>.*.>
//
// `<instance>` is a `*` wildcard and the method a `>` one — one
// subscription serves every named instance of the slot type and every
// method, of one segment or several (`a/b/c` travels as `a.b.c`). The
// dispatch handler parses the actual instance off each inbound
// subject and routes to `Endpoint.HandleInstance`.
//
// Built explicitly (not via SlotSubject) so the trailing `*`s don't
// fail token validation. The fixed prefix is validated implicitly via
// a sentinel SlotSubject call so a bad provider/slottype panics with
// the same message it would on a real publish.
func slotMethodPattern(tenantID, universeID, provider, slottype string) string {
	_ = subjects.SlotSubject(tenantID, universeID, provider, slottype, "default", "ping")
	return strings.Join([]string{
		"ktw", tenantID, "universe", universeID, "slot", provider, slottype, "*", ">",
	}, ".")
}

// slotQueueGroup is `slot:<provider>.<slottype>` — instance-agnostic
// so that true replicas of the same provider load-balance across every
// instance they host. Kept in this file (rather than reusing
// subjects.SlotQueueGroup which includes instance) because the queue
// group's semantics are NATS-layer routing, not subject construction.
func slotQueueGroup(provider, slottype string) string {
	return "slot:" + provider + "." + slottype
}

// dispatchHandler returns the MessageBus Handler that routes one
// incoming request into Endpoint.Handle and writes the response
// envelope back via msg.Respond.
//
// Scope propagation: reads `meta.tenantId` + `meta.universeId` headers
// from the inbound message (set by every Sender.Send/Notify) and injects
// them into the handler's ctx via WithScope. Slot handlers can then read
// the originator scope via TenantFromContext / UniverseFromContext —
// useful for auth gates, audit logs, and trace correlation. Missing
// headers leave the ctx unchanged (local handlers see "" and fall back
// to their own configured scope).
//
// Misrouting check: the subject's `<provider>` token must equal
// `selfProvider`. NATS' queue-group semantics prevent cross-provider
// delivery at the wire level, but the check is a defensive sanity net
// — a misconfiguration or future cluster-mode routing change would be
// caught here rather than silently answering as the wrong sidecar.
// baseCtx is the ctx passed to Subscribe — it carries the in-process
// Application (WithApplication) so dispatched service handlers can resolve
// services, config, and EM extension points (e.g. ResourceService.List
// walking ContributionProvidersPoint). The transport hands each message a
// fresh per-message ctx that does NOT carry the Application; we derive
// handlerCtx from baseCtx instead, mirroring StdioServe's Phase E §5b
// pass-through. The per-message ctx is intentionally dropped — over
// NATS request/reply the client enforces the deadline, and the
// subscription's lifecycle is already bound to baseCtx.
//
// A request carrying a call id (HeaderCallID) is served under a context its
// caller's cancel ends (calls, cancel.go).
//
// A request carrying a request id (HeaderRequestID) it already answered is
// answered again from the replay cache, its method not run again; one whose
// first run is in flight waits for that run's answer (replay.go).
//
// A request lent a panel handle (HeaderPanel) is served with that handle
// resumed at the caller's sequence and its Reporter installed (panel.Lend):
// the method's progress.Emit feeds the caller's panel, published with
// publish, and the answer gives the last sequence back.
func dispatchHandler(baseCtx context.Context, endpoint *ktsp.Endpoint, selfProvider string, calls *callRegistry, replay *replayCache, publish func(context.Context, string, []byte, ...mbapi.PubOpt) error) mbapi.Handler {
	return func(_ context.Context, msg mbapi.Message) {
		// Parse the addressing off the subject:
		//   ...slot.<provider>.<slottype>.<instance>.<method>[.<segment>…]
		provider, slottype, instance, method, err := parseSlotSubject(msg.Subject())
		if err != nil {
			respondError(msg, ktsp.ErrInvalidRequest(err.Error()))
			return
		}
		if provider != selfProvider {
			respondError(msg, ktsp.ErrInvalidRequest(fmt.Sprintf(
				"messagebus: subject provider %q does not match self %q (misrouting?)",
				provider, selfProvider)))
			return
		}
		// Inject originator scope from wire headers into the handler ctx,
		// chaining off baseCtx so the Application stays visible.
		tenantID, universeID := HeadersFromMessage(msg)
		// Who calls, from the key that signed the call: an
		// unsigned call, a disagreeing tenant header or a person's cause
		// no person's client attested is refused here (auth.go).
		authCtx, tenantID, authErr := authorize(baseCtx, msg, tenantID)
		if authErr != nil {
			respondError(msg, authErr)
			return
		}
		handlerCtx := WithScope(authCtx, tenantID, universeID)
		// The caller's cause, so the writes this call makes say why - and
		// only the caller's: a call that carries none is caused by no one,
		// never by the cause the serving process's own context carries (a
		// person's, unattested, that a keyed bus refuses on every call the
		// method makes in turn -).
		if c, ok := CauseFromMessage(msg); ok {
			handlerCtx = cause.WithCause(handlerCtx, c)
		} else {
			handlerCtx = cause.Without(handlerCtx)
		}
		// The caller's panel, lent to the method for its reports.
		var lent *panel.Handle
		if ref, ok := PanelFromMessage(msg); ok && publish != nil {
			if h, err := panel.Resume(ref, func(ctx context.Context, subject string, payload []byte) error {
				return publish(ctx, subject, payload)
			}); err == nil {
				lent = h
				handlerCtx = panel.Lend(handlerCtx, h)
			}
		}
		// A request sent again: answered once, from its first run.
		var first *replayEntry
		if id := RequestIDFromMessage(msg); id != "" && replay != nil && msg.Reply() != "" {
			e, owner := replay.begin(tenantID + "/" + universeID + "/" + id)
			if !owner {
				select {
				case <-e.done:
				case <-baseCtx.Done():
					respondError(msg, ktsp.ErrInternalError("the request's first run did not answer before the subscription ended"))
					return
				}
				if e.body == nil {
					respondError(msg, ktsp.ErrInternalError("the request's first run gave no answer"))
					return
				}
				if err := msg.Respond(e.body); err != nil {
					respondError(msg, ktsp.ErrInternalError(fmt.Sprintf("%s: the replayed reply could not be sent: %v", method, err)))
				}
				return
			}
			first = e
			defer func() {
				if first != nil { // it ended without an answer: who waits is told so
					replay.finish(first, nil)
				}
			}()
		}
		// The caller can cancel this call while it runs.
		if id := CallIDFromMessage(msg); id != "" && calls != nil && msg.Reply() != "" {
			var done func()
			handlerCtx, done = calls.begin(handlerCtx, id)
			defer done()
		}

		// Dispatch to the local endpoint by (slot, instance) — the
		// instance was parsed off the subject above. parseSlotSubject
		// guarantees instance is non-empty, so Handle's
		// instance-required guard isn't tripped here.
		result, dispErr := endpoint.Handle(handlerCtx, slottype, instance, method, msg.Data())
		if msg.Reply() == "" {
			// Notification (Publish, no Request) — don't respond.
			return
		}
		// The handler's error, with the message key it carries (Data.msg).
		rpcErr := ktsp.FromError(dispErr)
		var panelSeq int64
		if lent != nil {
			panelSeq = lent.Seq()
		}
		body, err := encodeResponseSeq(result, rpcErr, panelSeq)
		if err != nil {
			body, _ = encodeResponse(nil, ktsp.ErrInternalError("encode failure"))
		}
		if first != nil {
			replay.finish(first, body)
			first = nil
		}
		if err := msg.Respond(body); err != nil {
			// A reply the bus cannot carry (larger than its maximum payload)
			// is never delivered: the caller would wait for its whole timeout
			// with nothing to say why. Answer the failure instead.
			respondError(msg, ktsp.ErrInternalError(fmt.Sprintf(
				"%s: the reply (%d bytes) could not be sent: %v — narrow the request", method, len(body), err)))
		}
	}
}

// parseSlotSubject extracts (provider, slottype, instance, method) from
// the subject. The expected shape is:
//
//	ktw.<t>.universe.<u>.slot.<provider>.<slottype>.<instance>.<method>[.<segment>…]  → 9+ parts
//
// A method of several segments travels one token per segment
// (subjects.SlotSubject turns its `/` into `.`): the tokens from the
// ninth on are joined back with `/`. Malformed subjects (fewer tokens)
// error cleanly so the dispatch handler can return a typed RPC error
// instead of crashing.
func parseSlotSubject(subject string) (provider, slottype, instance, method string, err error) {
	parts := strings.Split(subject, ".")
	if len(parts) < 9 {
		return "", "", "", "", fmt.Errorf("messagebus: malformed slot subject %q (want at least 9 tokens: ktw.<t>.universe.<u>.slot.<provider>.<slottype>.<instance>.<method>[.<segment>…])", subject)
	}
	if parts[0] != "ktw" || parts[2] != "universe" || parts[4] != "slot" {
		return "", "", "", "", fmt.Errorf("messagebus: malformed slot subject %q (fixed prefix tokens don't match scheme)", subject)
	}
	return parts[5], parts[6], parts[7], strings.Join(parts[8:], "/"), nil
}

func respondError(msg mbapi.Message, rpcErr *ktsp.RPCError) {
	if msg.Reply() == "" {
		return
	}
	body, _ := encodeResponse(nil, rpcErr)
	_ = msg.Respond(body)
}

// SubscriptionGroup tracks every per-slot subscription installed by
// Subscribe and fans Drain / Unsubscribe across them.
type SubscriptionGroup struct {
	mu        sync.Mutex
	subs      []namedSub
	cancelSub mbapi.Subscription // the provider's call cancels (cancel.go)
}

type namedSub struct {
	slot string
	sub  mbapi.Subscription
}

func (g *SubscriptionGroup) add(slot string, sub mbapi.Subscription) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.subs = append(g.subs, namedSub{slot: slot, sub: sub})
}

// Drain calls Drain on every subscription, returning the first error.
func (g *SubscriptionGroup) Drain() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	var firstErr error
	for _, ns := range g.subs {
		if err := ns.sub.Drain(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if g.cancelSub != nil {
		if err := g.cancelSub.Drain(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Unsubscribe calls Unsubscribe on every subscription, returning the first error.
func (g *SubscriptionGroup) Unsubscribe() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	var firstErr error
	for _, ns := range g.subs {
		if err := ns.sub.Unsubscribe(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if g.cancelSub != nil {
		if err := g.cancelSub.Unsubscribe(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Slots returns the list of slot names this group has subscribed to,
// in the order they were registered.
func (g *SubscriptionGroup) Slots() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.subs))
	for _, ns := range g.subs {
		out = append(out, ns.slot)
	}
	return out
}
