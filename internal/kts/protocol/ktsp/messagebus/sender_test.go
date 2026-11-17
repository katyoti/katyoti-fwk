// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	teststub "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

const (
	testTenant   = "default"
	testUniverse = "test-u"
	testProvider = "test-binary" // selfProvider for the Subscribe side
)

// newEndpointWithHello builds a minimal Endpoint with one slot ("hello")
// exposing 3 methods that exercise: success, RPCError, and opaque-error
// paths through the wire envelope.
func newEndpointWithHello(t *testing.T) *ktsp.Endpoint {
	t.Helper()
	ep := ktsp.NewEndpoint("test-container", "test", "1.0.0")

	slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "test slot", nil)
	slot.Add(ktsp.RawMethod("echo", ktsp.MethodOpts{Desc: "echo params verbatim"},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			if len(params) == 0 {
				return json.RawMessage(`null`), nil
			}
			return params, nil
		}))
	slot.Add(ktsp.RawMethod("fail", ktsp.MethodOpts{Desc: "always returns ErrToolNotFound"},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return nil, ktsp.ErrToolNotFound("hammer")
		}))
	slot.Add(ktsp.RawMethod("raw-error", ktsp.MethodOpts{Desc: "opaque error"},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("opaque non-rpc failure")
		}))
	slot.Add(ktsp.RawMethod("cause", ktsp.MethodOpts{Desc: "the cause the handler's ctx carries"},
		func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			c, ok := cause.CauseFrom(ctx)
			return json.Marshal(map[string]any{"ok": ok, "cause": c})
		}))
	ep.AddSlot(slot)
	return ep
}

// TestACallCarriesItsCause: a call made under a cause carries it in the
// ktw.meta.cause header — the fixed scope header untouched — and the
// dispatched handler's context holds it; a call made under none carries none.
func TestACallCarriesItsCause(t *testing.T) {
	bus := teststub.New()
	ep := newEndpointWithHello(t)
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()
	sender := NewSender(bus, testTenant, testUniverse)
	who := cause.Cause{Kind: cause.KindAgent, ID: "run-9", Depth: 1}
	caused := cause.WithCause(context.Background(), who)

	h := sender.headerFor(caused)
	if got := h[HeaderCause]; len(got) != 1 || got[0] != `{"kind":"agent","id":"run-9","depth":1}` {
		t.Fatalf("header %s = %v, want the cause as JSON", HeaderCause, got)
	}
	if h[HeaderTenantID][0] != testTenant || len(sender.header) != 2 || sender.header[HeaderCause] != nil {
		t.Fatalf("the per-call header lost the scope or the fixed header was written to: %v / %v", h, sender.header)
	}

	type seen struct {
		OK    bool        `json:"ok"`
		Cause cause.Cause `json:"cause"`
	}
	call := func(ctx context.Context) seen {
		t.Helper()
		out, err := sender.Call(ctx, testProvider+":/hello/cause", nil)
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		var s seen
		if err := json.Unmarshal(out, &s); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return s
	}
	if got := call(caused); !got.OK || got.Cause != who {
		t.Fatalf("the handler saw %+v, want %+v", got, who)
	}
	if got := call(context.Background()); got.OK || !got.Cause.IsZero() {
		t.Fatalf("an uncaused call reached the handler with %+v", got)
	}
}

func TestSendReceiveEcho(t *testing.T) {
	bus := teststub.New()
	ep := newEndpointWithHello(t)

	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	in := map[string]any{"msg": "hi", "n": 42}
	out, err := sender.Call(ctx, testProvider+":/hello/echo", in)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode echo result: %v", err)
	}
	if got["msg"] != "hi" || int(got["n"].(float64)) != 42 {
		t.Errorf("echo round-trip failed: got %+v want %+v", got, in)
	}
}

func TestSendPropagatesRPCError(t *testing.T) {
	bus := teststub.New()
	ep := newEndpointWithHello(t)
	group, _ := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	_, err := sender.Call(context.Background(), testProvider+":/hello/fail", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var rpc *ktsp.RPCError
	if !errors.As(err, &rpc) {
		t.Fatalf("expected *ktsp.RPCError, got %T: %v", err, err)
	}
	if rpc.Code != ktsp.CodeToolNotFound {
		t.Errorf("Code = %d, want %d", rpc.Code, ktsp.CodeToolNotFound)
	}
}

func TestSendWrapsOpaqueError(t *testing.T) {
	bus := teststub.New()
	ep := newEndpointWithHello(t)
	group, _ := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	_, err := sender.Call(context.Background(), testProvider+":/hello/raw-error", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var rpc *ktsp.RPCError
	if !errors.As(err, &rpc) {
		t.Fatalf("expected *ktsp.RPCError wrapping, got %T: %v", err, err)
	}
	if rpc.Code != ktsp.CodeInternalError {
		t.Errorf("Code = %d, want CodeInternalError", rpc.Code)
	}
}

func TestSendNoResponderReturnsSlotNotFound(t *testing.T) {
	bus := teststub.New()
	// No Subscribe — nothing is listening.

	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := sender.Call(ctx, testProvider+":/hello/echo", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var rpc *ktsp.RPCError
	if !errors.As(err, &rpc) {
		t.Fatalf("expected *ktsp.RPCError, got %T: %v", err, err)
	}
	if rpc.Code != ktsp.CodeSlotNotFound {
		t.Errorf("Code = %d, want CodeSlotNotFound; msg=%q", rpc.Code, rpc.Message)
	}
}

func TestNotifyDoesNotWaitForReply(t *testing.T) {
	bus := teststub.New()
	ep := newEndpointWithHello(t)
	group, _ := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	if err := sender.Notify(context.Background(), testProvider+":/hello/echo", map[string]any{"x": 1}); err != nil {
		t.Errorf("Notify returned error: %v", err)
	}
	// No assertion on reply — Notify is fire-and-forget.
}

// TestParsePath legacy form was removed alongside the old parsePath
// helper. The replacement URI parser is exercised in path_test.go
// (TestParsePath_HappyPath, TestParsePath_ErrorCases,
// TestParsePath_EmptyProviderWithoutCtx, TestPath_Subject).

func TestSubscriptionGroup_TracksSlots(t *testing.T) {
	bus := teststub.New()
	ep := newEndpointWithHello(t)
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	// Endpoint auto-registers a built-in `ktsp` slot, so we expect both.
	slots := group.Slots()
	have := map[string]bool{}
	for _, s := range slots {
		have[s] = true
	}
	if !have["hello"] {
		t.Errorf("Slots() = %v missing 'hello'", slots)
	}
	if !have["ktsp"] {
		t.Errorf("Slots() = %v missing built-in 'ktsp'", slots)
	}
}

func TestHeadersAttachedOnSend(t *testing.T) {
	bus := teststub.New()

	// Custom subscription that captures the headers received on the
	// inbound request — bypasses the manifestbus.Subscribe wrapper so
	// we can inspect raw msg.Header().
	// Wire form: slot.<provider>.<slottype>.<instance>.<method>
	subject := "ktw." + testTenant + ".universe." + testUniverse +
		".slot." + testProvider + ".hello.default.echo"
	gotTenant := make(chan string, 1)
	gotUniverse := make(chan string, 1)
	sub, err := bus.QueueSubscribe(context.Background(), subject,
		"slot:"+testProvider+".hello.default",
		func(_ context.Context, msg mbapi.Message) {
			t, u := HeadersFromMessage(msg)
			select {
			case gotTenant <- t:
			default:
			}
			select {
			case gotUniverse <- u:
			default:
			}
			env, _ := encodeResponse(json.RawMessage(`null`), nil)
			_ = msg.Respond(env)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if _, err := sender.Call(ctx, testProvider+":/hello/echo", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case got := <-gotTenant:
		if got != testTenant {
			t.Errorf("tenant header = %q, want %q", got, testTenant)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive tenant header within deadline")
	}
	select {
	case got := <-gotUniverse:
		if got != testUniverse {
			t.Errorf("universe header = %q, want %q", got, testUniverse)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive universe header within deadline")
	}
}

func TestHeadersAttachedOnNotify(t *testing.T) {
	bus := teststub.New()

	subject := "ktw." + testTenant + ".universe." + testUniverse +
		".slot." + testProvider + ".hello.default.notify-me"
	gotTenant := make(chan string, 1)
	sub, err := bus.Subscribe(context.Background(), subject,
		func(_ context.Context, msg mbapi.Message) {
			t, _ := HeadersFromMessage(msg)
			select {
			case gotTenant <- t:
			default:
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	if err := sender.Notify(context.Background(), testProvider+":/hello/notify-me", nil); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case got := <-gotTenant:
		if got != testTenant {
			t.Errorf("tenant header = %q, want %q", got, testTenant)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Notify did not deliver tenant header")
	}
}

func TestScopeContext_HandlerSeesCallerScope(t *testing.T) {
	// End-to-end: Sender attaches meta.tenantId/universeId headers,
	// Subscribe's dispatchHandler reads them and injects into the
	// handler's ctx. The handler reads them back via context helpers.
	bus := teststub.New()

	ep := ktsp.NewEndpoint("test-container", "test", "1.0.0")
	slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "test slot", nil)
	gotTenant := make(chan string, 1)
	gotUniverse := make(chan string, 1)
	slot.Add(ktsp.RawMethod("ping", ktsp.MethodOpts{Desc: "echo scope via ctx"},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			select {
			case gotTenant <- TenantFromContext(ctx):
			default:
			}
			select {
			case gotUniverse <- UniverseFromContext(ctx):
			default:
			}
			return json.RawMessage(`"pong"`), nil
		}))
	ep.AddSlot(slot)

	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if _, err := sender.Call(ctx, testProvider+":/hello/ping", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case got := <-gotTenant:
		if got != testTenant {
			t.Errorf("handler ctx tenant = %q, want %q", got, testTenant)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler didn't receive tenant within deadline")
	}
	select {
	case got := <-gotUniverse:
		if got != testUniverse {
			t.Errorf("handler ctx universe = %q, want %q", got, testUniverse)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler didn't receive universe within deadline")
	}
}

func TestScopeContext_MissingHeadersLeaveContextUnchanged(t *testing.T) {
	// Bare ctx — no scope bound.
	ctx := context.Background()
	if got := TenantFromContext(ctx); got != "" {
		t.Errorf("bare ctx tenant = %q, want empty", got)
	}
	if got := UniverseFromContext(ctx); got != "" {
		t.Errorf("bare ctx universe = %q, want empty", got)
	}

	// WithTenant("") is a no-op (don't pollute ctx with empty values).
	ctx2 := WithTenant(ctx, "")
	if got := TenantFromContext(ctx2); got != "" {
		t.Errorf("WithTenant(empty): tenant = %q, want empty", got)
	}
	// WithTenant("real") attaches.
	ctx3 := WithTenant(ctx, "tenant-x")
	if got := TenantFromContext(ctx3); got != "tenant-x" {
		t.Errorf("WithTenant: tenant = %q, want tenant-x", got)
	}
}

func TestHeadersFromMessage_MissingHeaderReturnsEmpty(t *testing.T) {
	// Build a Message with no headers and verify graceful degradation.
	bus := teststub.New()
	subject := "test.no-headers"
	got := make(chan string, 1)
	sub, _ := bus.Subscribe(context.Background(), subject,
		func(_ context.Context, msg mbapi.Message) {
			tID, _ := HeadersFromMessage(msg)
			got <- tID
		})
	defer sub.Unsubscribe()
	_ = bus.Publish(context.Background(), subject, []byte("hi"))

	select {
	case t1 := <-got:
		if t1 != "" {
			t.Errorf("expected empty tenant, got %q", t1)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no message delivered")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	bus := teststub.New()
	ep := newEndpointWithHello(t)
	group, _ := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)

	sender := NewSender(bus, testTenant, testUniverse)
	// Sanity: works before unsubscribe.
	if _, err := sender.Call(context.Background(), testProvider+":/hello/echo", nil); err != nil {
		t.Fatalf("pre-unsubscribe Send failed: %v", err)
	}

	if err := group.Unsubscribe(); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := sender.Call(ctx, testProvider+":/hello/echo", nil)
	if err == nil {
		t.Error("expected error after unsubscribe, got nil")
	}
}

// TestAMethodOfSeveralSegmentsIsReachable: a method whose name has several
// segments (`a/b/c`, as `tasks/dossier/refine-scope`) travels as one subject
// token per segment; the slot's subscription covers it and the dispatch
// gives the handler its whole name back, so the call is answered.
// A one-segment method on the same slot is still answered.
func TestAMethodOfSeveralSegmentsIsReachable(t *testing.T) {
	bus := teststub.New()
	ep := newEndpointWithHello(t)
	slot := ktsp.NewSlot("deep", ktsp.DefaultInstance, "multi-segment methods", nil)
	slot.Add(ktsp.RawMethod("a/b/c", ktsp.MethodOpts{Desc: "three segments"},
		func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`"abc"`), nil
		}))
	slot.Add(ktsp.RawMethod("dossier/refine-scope", ktsp.MethodOpts{Desc: "two segments"},
		func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`"refined"`), nil
		}))
	ep.AddSlot(slot)
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()
	sender := NewSender(bus, testTenant, testUniverse)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for path, want := range map[string]string{
		testProvider + ":deep/a/b/c":               `"abc"`,
		testProvider + ":Deep/Dossier/RefineScope": `"refined"`,
		testProvider + ":hello/echo":               `"one"`,
	} {
		var params any
		if want == `"one"` {
			params = "one"
		}
		out, err := sender.Call(ctx, path, params)
		if err != nil {
			t.Fatalf("Call(%s): %v", path, err)
		}
		if string(out) != want {
			t.Errorf("Call(%s) = %s, want %s", path, out, want)
		}
	}
}
