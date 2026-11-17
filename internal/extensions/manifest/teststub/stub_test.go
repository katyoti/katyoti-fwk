// SPDX-License-Identifier: Apache-2.0

package teststub

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Mirrors the smoke suite of internal/kts/transport/nats/embedded_test.go,
// but runs entirely in-memory. Any test that does not need TTL or network
// failure simulation should use this stub.

func TestStub_PubSub_RoundTrip(t *testing.T) {
	bus := New()
	got := make(chan []byte, 1)
	sub, err := bus.Subscribe(context.Background(), "test.pubsub", func(_ context.Context, m mbapi.Message) {
		got <- m.Data()
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Drain()

	if err := bus.Publish(context.Background(), "test.pubsub", []byte("hello")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case data := <-got:
		if string(data) != "hello" {
			t.Errorf("got %q", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestStub_QueueGroup_LoadBalances(t *testing.T) {
	bus := New()
	const N = 20
	var a, b atomic.Int32

	subA, err := bus.QueueSubscribe(context.Background(), "test.q", "workers",
		func(_ context.Context, _ mbapi.Message) { a.Add(1) })
	if err != nil {
		t.Fatalf("QueueSubscribe a: %v", err)
	}
	defer subA.Drain()

	subB, err := bus.QueueSubscribe(context.Background(), "test.q", "workers",
		func(_ context.Context, _ mbapi.Message) { b.Add(1) })
	if err != nil {
		t.Fatalf("QueueSubscribe b: %v", err)
	}
	defer subB.Drain()

	for i := 0; i < N; i++ {
		if err := bus.Publish(context.Background(), "test.q", []byte("x")); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	total := a.Load() + b.Load()
	if total != N {
		t.Errorf("got %d total, want %d (a=%d b=%d)", total, N, a.Load(), b.Load())
	}
	if a.Load() == 0 || b.Load() == 0 {
		t.Errorf("queue group did not load-balance: a=%d b=%d", a.Load(), b.Load())
	}
}

func TestStub_RequestReply(t *testing.T) {
	bus := New()
	sub, err := bus.Subscribe(context.Background(), "test.rpc", func(_ context.Context, m mbapi.Message) {
		_ = m.Respond([]byte("pong"))
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Drain()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := bus.Request(ctx, "test.rpc", []byte("ping"))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if string(out) != "pong" {
		t.Errorf("got %q, want pong", out)
	}
}

func TestStub_RequestReply_NoResponders(t *testing.T) {
	bus := New()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := bus.Request(ctx, "no.responders", []byte("ping"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(mbapi.ErrNoResponders); !ok {
		t.Errorf("expected ErrNoResponders, got %T: %v", err, err)
	}
}

func TestStub_RequestReply_Timeout(t *testing.T) {
	bus := New()
	// Subscriber that never responds.
	sub, err := bus.Subscribe(context.Background(), "test.timeout", func(_ context.Context, _ mbapi.Message) {})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Drain()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = bus.Request(ctx, "test.timeout", []byte("ping"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(mbapi.ErrTimeout); !ok && err != context.DeadlineExceeded {
		t.Errorf("expected ErrTimeout or context.DeadlineExceeded, got %T: %v", err, err)
	}
}

func TestStub_Wildcards_Single(t *testing.T) {
	bus := New()
	got := make(chan string, 4)

	sub, err := bus.Subscribe(context.Background(), "ktw.*.universe.*.event.created",
		func(_ context.Context, m mbapi.Message) { got <- m.Subject() })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Drain()

	bus.Publish(context.Background(), "ktw.t1.universe.u1.event.created", nil)
	bus.Publish(context.Background(), "ktw.t1.universe.u2.event.created", nil)
	bus.Publish(context.Background(), "ktw.t1.universe.u1.event.deleted", nil) // shouldn't match
	time.Sleep(50 * time.Millisecond)

	if len(got) != 2 {
		t.Errorf("got %d matches, want 2", len(got))
	}
}

func TestStub_Wildcards_Multi(t *testing.T) {
	bus := New()
	got := make(chan string, 4)

	sub, err := bus.Subscribe(context.Background(), "ktw.t1.universe.u1.>",
		func(_ context.Context, m mbapi.Message) { got <- m.Subject() })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Drain()

	bus.Publish(context.Background(), "ktw.t1.universe.u1.event.created", nil)
	bus.Publish(context.Background(), "ktw.t1.universe.u1.session.s1.event.x", nil)
	bus.Publish(context.Background(), "ktw.t2.universe.u1.event.created", nil) // wrong tenant
	time.Sleep(50 * time.Millisecond)

	if len(got) != 2 {
		t.Errorf("got %d matches, want 2", len(got))
	}
}

func TestStub_KV_PutGet(t *testing.T) {
	bus := New()
	kv := bus.JetStream().KV("KTW_SESSION")
	ctx := context.Background()

	rev, err := kv.Put(ctx, "tenant.session-1", []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if rev != 1 {
		t.Errorf("revision = %d, want 1", rev)
	}

	entry, err := kv.Get(ctx, "tenant.session-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(entry.Value()) != `{"x":1}` {
		t.Errorf("value = %q", entry.Value())
	}
	if entry.Revision() != 1 {
		t.Errorf("revision = %d, want 1", entry.Revision())
	}

	rev2, err := kv.Put(ctx, "tenant.session-1", []byte(`{"x":2}`))
	if err != nil {
		t.Fatalf("Put 2: %v", err)
	}
	if rev2 != 2 {
		t.Errorf("revision = %d, want 2", rev2)
	}
}

func TestStub_KV_UpdateCAS(t *testing.T) {
	bus := New()
	kv := bus.JetStream().KV("KTW_ROOM")
	ctx := context.Background()

	rev, _ := kv.Put(ctx, "k", []byte("a"))

	// Wrong revision should fail.
	if _, err := kv.UpdateCAS(ctx, "k", []byte("b"), rev+99); err == nil {
		t.Errorf("UpdateCAS with wrong rev should fail")
	}

	// Right revision should succeed.
	rev2, err := kv.UpdateCAS(ctx, "k", []byte("c"), rev)
	if err != nil {
		t.Fatalf("UpdateCAS: %v", err)
	}
	if rev2 != rev+1 {
		t.Errorf("rev2 = %d, want %d", rev2, rev+1)
	}

	entry, _ := kv.Get(ctx, "k")
	if string(entry.Value()) != "c" {
		t.Errorf("value = %q", entry.Value())
	}
}

func TestStub_KV_Watch(t *testing.T) {
	bus := New()
	kv := bus.JetStream().KV("KTW_PRESENCE")
	ctx := context.Background()
	kv.Put(ctx, "a", []byte("v1"))

	w, err := kv.Watch(ctx, "a")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	// First update should be the existing value.
	select {
	case e := <-w.Updates():
		if string(e.Value()) != "v1" {
			t.Errorf("initial = %q", e.Value())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial value")
	}

	// Subsequent put should arrive.
	go kv.Put(ctx, "a", []byte("v2"))
	select {
	case e := <-w.Updates():
		if string(e.Value()) != "v2" {
			t.Errorf("v2 = %q", e.Value())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for update")
	}
}

func TestStub_Stream_PublishConsume(t *testing.T) {
	bus := New()
	js := bus.JetStream()
	stream := js.Stream("KTW_SESSION_JOURNAL")
	ctx := context.Background()

	stream.Publish(ctx, "ktw.t.universe.u.session.s1.event.created", []byte("e1"))
	stream.Publish(ctx, "ktw.t.universe.u.session.s2.event.created", []byte("e2"))
	stream.Publish(ctx, "ktw.t.universe.u.session.s1.event.completed", []byte("e3"))

	consumer, err := stream.PullConsumer(ctx, "test", mbapi.ConsumerConfig{
		FilterSubject: "ktw.t.universe.u.session.s1.event.>",
		DeliverPolicy: mbapi.DeliverFromStart,
	})
	if err != nil {
		t.Fatalf("PullConsumer: %v", err)
	}
	defer consumer.Close()

	fetchCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	msgs, err := consumer.Fetch(fetchCtx, 10)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if string(msgs[0].Data()) != "e1" || string(msgs[1].Data()) != "e3" {
		t.Errorf("payloads: %q, %q", msgs[0].Data(), msgs[1].Data())
	}
}

func TestStub_Drain(t *testing.T) {
	bus := New()
	bus.Subscribe(context.Background(), "x", func(_ context.Context, _ mbapi.Message) {})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Drain(ctx); err != nil {
		t.Errorf("Drain: %v", err)
	}
	// After drain, Publish should error.
	if err := bus.Publish(context.Background(), "x", nil); err == nil {
		t.Errorf("Publish after Drain should fail")
	}
}

// TestStub_SatisfiesInterface — compile-time + reflection-time check.
func TestStub_SatisfiesInterface(t *testing.T) {
	var _ mbapi.MessageBus = New()
}
