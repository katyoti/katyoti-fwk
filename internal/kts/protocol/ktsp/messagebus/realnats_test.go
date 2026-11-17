// SPDX-License-Identifier: Apache-2.0

package messagebus

// Integration tests for KTSP-over-NATS — exercise messagebus.Subscribe
// + messagebus.Sender against a real embedded nats-server, not the
// in-memory teststub. These prove the wire-level shape (subject
// pattern, queue group, header propagation, request/reply) matches what
// the production NATS adapter actually produces.
//
// The teststub tests in `sender_test.go` cover logical correctness; this
// file covers wire correctness. A failure here without a corresponding
// teststub failure usually means the teststub diverges from real NATS.

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

func startEmbeddedHub(t *testing.T) *natsembedded.EmbeddedHandle {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "nats-jetstream")
	opts := natsembedded.EmbeddedOptions(dir, true /*quiet*/)
	h, err := natsembedded.StartEmbedded(opts)
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	t.Cleanup(h.Stop)
	return h
}

// TestRealNATS_RoundTrip — full KTSP request/response round-trip
// through embedded NATS using messagebus.Subscribe (sidecar side) and
// messagebus.Sender (hub-side caller).
func TestRealNATS_RoundTrip(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)

	ep := newEndpointWithHello(t)
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	in := map[string]any{"msg": "hi", "n": 42}
	out, err := sender.Call(ctx, testProvider+":/hello/echo", in)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["msg"] != "hi" || int(got["n"].(float64)) != 42 {
		t.Errorf("round-trip: got %+v want %+v", got, in)
	}
}

// ctxSentinelKey is a private context key used by the base-ctx
// propagation regression test below.
type ctxSentinelKey struct{}

// TestRealNATS_BaseCtxThreadedToHandler — the ctx passed to Subscribe
// (which in production carries the in-process Application via
// WithApplication) MUST reach the dispatched slot handler. The transport
// hands each message a fresh per-message ctx that does NOT carry the
// Application; dispatchHandler derives the handler ctx from the Subscribe
// base ctx instead (mirroring StdioServe's Phase E §5b pass-through).
//
// Regression for the "resources/list over the bus returns []" bug: the
// served ResourceService.List walks ContributionProvidersPoint via
// application.UseExtensions[…](ctx), which silently returns nil when the
// Application is absent from ctx — collapsing the manifest to empty.
func TestRealNATS_BaseCtxThreadedToHandler(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)

	ep := ktsp.NewEndpoint("test", "", "")
	slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("sees-base-ctx", ktsp.MethodOpts{},
		func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			v, _ := ctx.Value(ctxSentinelKey{}).(string)
			return json.Marshal(v)
		}))
	ep.AddSlot(slot)

	// Subscribe with a base ctx carrying a sentinel value — stands in for
	// WithApplication(ctx, app) in production.
	baseCtx := context.WithValue(context.Background(), ctxSentinelKey{}, "from-base-ctx")
	group, err := Subscribe(baseCtx, bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := sender.Call(ctx, testProvider+":/hello/sees-base-ctx", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var got string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != "from-base-ctx" {
		t.Errorf("handler ctx lost the Subscribe base ctx value: got %q, want %q "+
			"(the Application would be invisible to UseExtensions/UseService)", got, "from-base-ctx")
	}
}

// TestRealNATS_RPCErrorPropagates — sidecar-side RPCError survives
// the wire envelope and resurfaces as the same *ktsp.RPCError on the
// caller side.
func TestRealNATS_RPCErrorPropagates(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)

	ep := newEndpointWithHello(t)
	group, _ := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := sender.Call(ctx, testProvider+":/hello/fail", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var rpc *ktsp.RPCError
	if !errors.As(err, &rpc) {
		t.Fatalf("expected *ktsp.RPCError, got %T: %v", err, err)
	}
	if rpc.Code != ktsp.CodeToolNotFound {
		t.Errorf("Code = %d, want CodeToolNotFound", rpc.Code)
	}
}

// TestRealNATS_NoResponderMapsToSlotNotFound — when nothing is
// subscribed for a given subject, NATS returns ErrNoResponders;
// messagebus.Sender maps that to ktsp.ErrSlotNotFound so callers see
// the same shape they'd see from a local Endpoint.Handle miss.
func TestRealNATS_NoResponderMapsToSlotNotFound(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)

	// No Subscribe — nothing is listening on the slot subject.
	sender := NewSender(bus, testTenant, testUniverse)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := sender.Call(ctx, testProvider+":/ghost/method", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var rpc *ktsp.RPCError
	if !errors.As(err, &rpc) {
		t.Fatalf("expected *ktsp.RPCError, got %T: %v", err, err)
	}
	if rpc.Code != ktsp.CodeSlotNotFound {
		t.Errorf("Code = %d, want CodeSlotNotFound", rpc.Code)
	}
}

// TestRealNATS_QueueGroupLoadBalances — two replica subscriptions on
// the same slot's queue group should split incoming requests roughly
// 50/50 (NATS round-robins within a queue group). This is THE
// multi-instance routing primitive the analysis §3.4 calls out.
func TestRealNATS_QueueGroupLoadBalances(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)

	// Two Endpoints with the same slot/method, both subscribed under
	// the same queue group via Subscribe (which uses SlotQueueGroup
	// internally). NATS round-robins.
	mu := sync.Mutex{}
	var aCount, bCount atomic.Int32

	makeEndpoint := func(counter *atomic.Int32) *ktsp.Endpoint {
		ep := ktsp.NewEndpoint("test", "", "")
		slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "", nil)
		slot.Add(ktsp.RawMethod("ping", ktsp.MethodOpts{},
			func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
				counter.Add(1)
				mu.Lock()
				defer mu.Unlock()
				return json.RawMessage(`"pong"`), nil
			}))
		ep.AddSlot(slot)
		return ep
	}

	groupA, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, makeEndpoint(&aCount))
	if err != nil {
		t.Fatal(err)
	}
	defer groupA.Unsubscribe()
	groupB, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, makeEndpoint(&bCount))
	if err != nil {
		t.Fatal(err)
	}
	defer groupB.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	const N = 20
	for i := 0; i < N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_, err := sender.Call(ctx, testProvider+":/hello/ping", nil)
		cancel()
		if err != nil {
			t.Fatalf("Send #%d: %v", i, err)
		}
	}

	a, b := int(aCount.Load()), int(bCount.Load())
	if a+b != N {
		t.Errorf("total = %d, want %d", a+b, N)
	}
	// Round-robin should give each handler at least one call across N=20.
	if a == 0 || b == 0 {
		t.Errorf("queue group did NOT load-balance: a=%d b=%d", a, b)
	}
}

// TestRealNATS_NotifyFireAndForget — Notify uses Publish (no reply
// subject), so it returns immediately and the handler still runs.
func TestRealNATS_NotifyFireAndForget(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)

	got := make(chan struct{}, 1)
	ep := ktsp.NewEndpoint("test", "", "")
	slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("notify-me", ktsp.MethodOpts{},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			select {
			case got <- struct{}{}:
			default:
			}
			return nil, nil
		}))
	ep.AddSlot(slot)

	group, _ := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	if err := sender.Notify(context.Background(), testProvider+":/hello/notify-me", nil); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not run within 2s of Notify")
	}
}

// TestRealNATS_NamedInstanceDispatch — proves the substrate now
// honours `<provider>:<slot>(<instance>)/<method>` wire addressing.
// Two instances of one slot type with different bound handlers; each
// caller's request must reach the matching instance, end-to-end over
// embedded NATS.
//
// This is the dispatch-layer test for the
// `bus-server-side-instance-dispatch` work — formerly the limitation
// that blocked CRUDService per-connection slots, LLMProfiles per-
// profile slots, and the wider "named instance is a first-class wire
// citizen" story.
func TestRealNATS_NamedInstanceDispatch(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)

	ep := ktsp.NewEndpoint("test-container", "test", "1.0.0")

	// Two instances of the same `echo-svc` slot type, each bound to a
	// distinct handler closure that echoes the instance's identifier.
	// The slot itself owns its instance — Phase A rename.
	makeSlot := func(instance, identifier string) *ktsp.Slot {
		s := ktsp.NewSlot("echo-svc", instance, "echo svc", nil)
		id := identifier // capture
		s.Add(ktsp.RawMethod("which", ktsp.MethodOpts{},
			func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
				return json.Marshal(id)
			}))
		return s
	}
	ep.AddSlot(makeSlot("alice", "alice-impl"))
	ep.AddSlot(makeSlot("bob", "bob-impl"))
	ep.AddSlot(makeSlot(ktsp.DefaultInstance, "default-impl"))

	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)

	cases := []struct {
		path string
		want string
	}{
		// Named instances reach their own handler.
		{testProvider + ":echo-svc(alice)/which", "alice-impl"},
		{testProvider + ":echo-svc(bob)/which", "bob-impl"},
		// Default instance both with and without explicit `(default)`.
		{testProvider + ":echo-svc/which", "default-impl"},
		{testProvider + ":echo-svc(default)/which", "default-impl"},
	}
	for _, tc := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		raw, err := sender.Call(ctx, tc.path, nil)
		cancel()
		if err != nil {
			t.Fatalf("Send %q: %v", tc.path, err)
		}
		var got string
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decode %q: %v", tc.path, err)
		}
		if got != tc.want {
			t.Errorf("Send %q → %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestRealNATS_NamedInstanceMissing — calling an unknown instance on
// an existing slot type returns a typed CodeSlotNotFound (with the
// `slot(instance)` form in the message) — not a NoResponders timeout.
// This is the failure mode CRUD tools rely on for "wrong connection
// name" diagnostics.
func TestRealNATS_NamedInstanceMissing(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)

	ep := ktsp.NewEndpoint("test-container", "test", "1.0.0")
	s := ktsp.NewSlot("echo-svc", "alice", "echo svc", nil)
	s.Add(ktsp.RawMethod("which", ktsp.MethodOpts{},
		func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.Marshal("alice-impl")
		}))
	ep.AddSlot(s)

	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = sender.Call(ctx, testProvider+":echo-svc(carol)/which", nil)
	if err == nil {
		t.Fatal("expected error for unknown instance, got nil")
	}
	var rpc *ktsp.RPCError
	if !errors.As(err, &rpc) {
		t.Fatalf("expected *ktsp.RPCError, got %T: %v", err, err)
	}
	if rpc.Code != ktsp.CodeSlotNotFound {
		t.Errorf("Code = %d, want CodeSlotNotFound (%d)", rpc.Code, ktsp.CodeSlotNotFound)
	}
}

// TestRealNATS_MultiUniverseCoHosted — Slice 1 of the multi-container hub
// (hub-multi-container-authority): ONE process (one NATS connection / one `bus`)
// co-hosts TWO universes' containers with DISTINCT (universe, provider) tuples —
// the `admin` universe (control plane, provider `katyoti-control`) and a work
// universe (provider `universe-<slug>`). Proves the reframed model
// ((modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)): a single hub process serves N universes by
// calling Subscribe once per universe, each independently addressable + hard-
// scoped over NATS, with cross-universe isolation.
func TestRealNATS_MultiUniverseCoHosted(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client) // ONE bus / one connection == one hub process

	// A trivial container whose `hello/which` echoes a marker.
	makeUniverseContainer := func(marker string) *ktsp.Endpoint {
		ep := ktsp.NewEndpoint("container-"+marker, marker, "1.0.0")
		slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "", nil)
		m := marker // capture
		slot.Add(ktsp.RawMethod("which", ktsp.MethodOpts{},
			func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
				return json.Marshal(m)
			}))
		ep.AddSlot(slot)
		return ep
	}

	const (
		adminUniverse = "admin"
		adminProvider = "katyoti-control"
		workUniverse  = "universe-acme"
		workProvider  = "universe-acme"
	)

	// Both containers subscribed from the SAME bus (one process), distinct
	// (universe, provider) — this is the multi-container hosting: Subscribe called
	// once per universe.
	gAdmin, err := Subscribe(context.Background(), bus, testTenant, adminUniverse, adminProvider, makeUniverseContainer("admin-control"))
	if err != nil {
		t.Fatalf("Subscribe(admin): %v", err)
	}
	defer gAdmin.Unsubscribe()
	gWork, err := Subscribe(context.Background(), bus, testTenant, workUniverse, workProvider, makeUniverseContainer("acme-universe"))
	if err != nil {
		t.Fatalf("Subscribe(work): %v", err)
	}
	defer gWork.Unsubscribe()

	// A client scoped to the admin universe reaches ONLY the admin container.
	adminSender := NewSender(bus, testTenant, adminUniverse)
	// A client scoped to the work universe reaches ONLY the work container.
	workSender := NewSender(bus, testTenant, workUniverse)

	callWhich := func(s *Sender, provider string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		raw, err := s.Call(ctx, provider+":/hello/which", nil)
		if err != nil {
			return "", err
		}
		var got string
		return got, json.Unmarshal(raw, &got)
	}

	// 1. Each universe's client reaches its own container.
	if got, err := callWhich(adminSender, adminProvider); err != nil || got != "admin-control" {
		t.Fatalf("admin universe: got %q err %v, want \"admin-control\"", got, err)
	}
	if got, err := callWhich(workSender, workProvider); err != nil || got != "acme-universe" {
		t.Fatalf("work universe: got %q err %v, want \"acme-universe\"", got, err)
	}

	// 2. Cross-universe isolation is HARD: the admin-scoped client cannot reach the
	// work universe's provider (different universe segment in the subject) — the
	// subject simply has no responder.
	if _, err := callWhich(adminSender, workProvider); err == nil {
		t.Fatalf("cross-universe isolation breached: admin client reached the work provider")
	}
}

// TestRealNATS_AReplyTooLargeFailsFast: a reply larger than the bus carries
// is answered with an error naming its size, at once — never left for the
// caller to wait out its timeout (an unfiltered whole-corpus listing hung
// for minutes). A reply the server's maximum allows still arrives whole.
func TestRealNATS_AReplyTooLargeFailsFast(t *testing.T) {
	opts := natsembedded.EmbeddedOptions(filepath.Join(t.TempDir(), "nats-jetstream"), true)
	opts.MaxPayload = 64 << 10
	h, err := natsembedded.StartEmbedded(opts)
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	t.Cleanup(h.Stop)
	bus := natsxport.New(h.Client)

	ep := withPadSlot(newEndpointWithHello(t))
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()
	sender := NewSender(bus, testTenant, testUniverse)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	_, err = sender.Call(ctx, testProvider+":/pad/make", map[string]any{"n": 128 << 10})
	if err == nil {
		t.Fatal("a reply over the maximum payload was accepted")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the caller waited %s for a reply that could not be sent", took)
	}
	if !strings.Contains(err.Error(), "could not be sent") {
		t.Fatalf("the error does not say the reply could not be sent: %v", err)
	}

	if _, err := sender.Call(ctx, testProvider+":/pad/make", map[string]any{"n": 32 << 10}); err != nil {
		t.Fatalf("a reply under the maximum failed: %v", err)
	}
}

// TestRealNATS_TheHubCarriesMultiMegabyteReplies: the embedded server's
// maximum is raised from NATS's 1 MiB default, so a reply of a few MiB
// (a whole corpus listed unfiltered) arrives.
func TestRealNATS_TheHubCarriesMultiMegabyteReplies(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)
	ep := withPadSlot(newEndpointWithHello(t))
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()
	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := sender.Call(ctx, testProvider+":/pad/make", map[string]any{"n": 3 << 20})
	if err != nil {
		t.Fatalf("a 3 MiB reply failed: %v", err)
	}
	if len(out) < 3<<20 {
		t.Fatalf("the reply was cut: %d bytes", len(out))
	}
}

// withPadSlot adds pad/make: a reply of n bytes for a request of a few.
func withPadSlot(ep *ktsp.Endpoint) *ktsp.Endpoint {
	slot := ktsp.NewSlot("pad", ktsp.DefaultInstance, "replies of a chosen size", nil)
	slot.Add(ktsp.RawMethod("make", ktsp.MethodOpts{Desc: "a reply of n bytes"},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var in struct{ N int }
			_ = json.Unmarshal(params, &in)
			return json.Marshal(strings.Repeat("x", in.N))
		}))
	ep.AddSlot(slot)
	return ep
}
