// SPDX-License-Identifier: Apache-2.0

package commslocal

import (
	"context"
	"testing"
	"time"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// newTestService + a real in-memory engine Channel — the parent-side wake path
// (Await) needs only the Channel, no TaskService/SuspensionManager. This is the
// exact logic that regressed (a parent woke on the wrong correlation / never),
// so it is pinned here without an LLM or a bootstrap.
func newTestServiceWithChannel() (*service, *engine.Channel, context.Context) {
	bus := engine.NewMemoryEventBus()
	ch := engine.NewChannel(bus)
	ctx := engine.WithChannel(context.Background(), ch)
	return &service{broker: newAskBroker()}, ch, ctx
}

func publishChildEvent(ch *engine.Channel, topic string, evt interface {
	CorrelationRef() *engine.Correlation
}) {
	_ = ch.Bus().Publish(topic, evt)
}

// `asked` wakes via the BROKER (a Post), NOT the engine Channel — this is the
// fix for the delegated agent child whose bubble runs with a nil Channel so its
// `child.suspended` never emits. The broker is hit on every Post regardless.
func TestAwait_WakesOnBrokerPost(t *testing.T) {
	svc, _, ctx := newTestServiceWithChannel()
	const parent = "parent-1"

	done := make(chan commsapi.Event, 1)
	go func() {
		ev, _ := svc.Await(ctx, commsapi.AwaitRequest{ParentRunID: parent, TimeoutS: 5})
		done <- ev
	}()

	// Let the broker subscription register before posting.
	time.Sleep(20 * time.Millisecond)
	svc.broker.Post(parent, agentapi.ChildQuestion{ID: "q1", ChildTaskID: "child-9", Question: "color?"})

	select {
	case ev := <-done:
		if ev.Kind != commsapi.EventAsked {
			t.Fatalf("want %q, got %q", commsapi.EventAsked, ev.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Await did not wake on the broker Post")
	}
}

// A question posted BEFORE Await blocked is caught by the outstanding pre-check
// (no lost wakeup, no timeout).
func TestAwait_OutstandingPreCheck(t *testing.T) {
	svc, _, ctx := newTestServiceWithChannel()
	const parent = "p"
	svc.broker.Post(parent, agentapi.ChildQuestion{ID: "q1", ChildTaskID: "c", Question: "x?"})

	ev, _ := svc.Await(ctx, commsapi.AwaitRequest{ParentRunID: parent, TimeoutS: 5})
	if ev.Kind != commsapi.EventAsked {
		t.Fatalf("want asked (outstanding pre-check), got %q", ev.Kind)
	}
}

// Outstanding reflects only UNANSWERED questions — it clears on Take (answer),
// so a re-await after answering does not falsely return `asked` (the
// double-answer bug).
func TestBroker_OutstandingClearsOnTake(t *testing.T) {
	b := newAskBroker()
	b.Post("p", agentapi.ChildQuestion{ID: "q1", ChildTaskID: "c", Question: "x?"})
	if !b.HasOutstanding("p") {
		t.Fatal("should be outstanding after Post")
	}
	if _, ok := b.Take("q1"); !ok {
		t.Fatal("Take failed")
	}
	if b.HasOutstanding("p") {
		t.Fatal("should NOT be outstanding after Take (answered) — else the parent double-answers")
	}
}

func TestAwait_IgnoresOtherParentsChildren(t *testing.T) {
	svc, ch, ctx := newTestServiceWithChannel()

	done := make(chan commsapi.Event, 1)
	go func() {
		ev, _ := svc.Await(ctx, commsapi.AwaitRequest{ParentRunID: "parent-A", TimeoutS: 1})
		done <- ev
	}()

	time.Sleep(20 * time.Millisecond)
	// A DIFFERENT parent's child — must NOT wake parent-A.
	evt := &engine.ChildSuspendedEvent{RunID: "child-x", ParentRunID: "parent-B"}
	evt.Correlation = engine.Correlation{RunID: "child-x", ParentRunID: "parent-B"}
	publishChildEvent(ch, engine.ChildSuspendedTopic("child-x"), evt)

	ev := <-done
	if ev.Kind != commsapi.EventTimeout {
		t.Fatalf("expected timeout (no matching child), got %q", ev.Kind)
	}
}

func TestAwait_DoneAndFailed(t *testing.T) {
	cases := []struct {
		name  string
		topic func(string) string
		want  string
	}{
		{"done", engine.ChildDoneTopic, commsapi.EventCompleted},
		{"failed", engine.ChildFailedTopic, commsapi.EventFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, ch, ctx := newTestServiceWithChannel()
			const parent = "p"
			done := make(chan commsapi.Event, 1)
			go func() {
				ev, _ := svc.Await(ctx, commsapi.AwaitRequest{ParentRunID: parent, TimeoutS: 5})
				done <- ev
			}()
			time.Sleep(20 * time.Millisecond)
			evt := &engine.ChildDoneEvent{RunID: "c", Output: "ok"}
			evt.Correlation = engine.Correlation{RunID: "c", ParentRunID: parent}
			publishChildEvent(ch, tc.topic("c"), evt)
			select {
			case ev := <-done:
				if ev.Kind != tc.want {
					t.Fatalf("want %q, got %q", tc.want, ev.Kind)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("Await did not wake on child.%s", tc.name)
			}
		})
	}
}

func TestBroker_PostDrainTake(t *testing.T) {
	b := newAskBroker()
	b.Post("parent", agentapi.ChildQuestion{ID: "q1", ChildTaskID: "c1", Question: "color?"})
	b.Post("parent", agentapi.ChildQuestion{ID: "q2", ChildTaskID: "c1", Question: "number?"})
	b.Post("other", agentapi.ChildQuestion{ID: "q3", ChildTaskID: "c2", Question: "x?"})

	got := b.Drain("parent")
	if len(got) != 2 {
		t.Fatalf("want 2 drained for parent, got %d", len(got))
	}
	// Drain does not remove byID — the parent can still Take to answer.
	if q, ok := b.Take("q1"); !ok || q.ChildTaskID != "c1" {
		t.Fatalf("Take(q1) failed: %+v ok=%v", q, ok)
	}
	// Second Take of the same id fails.
	if _, ok := b.Take("q1"); ok {
		t.Fatal("Take(q1) twice should fail")
	}
	// Drain cleared the parent queue.
	if rest := b.Drain("parent"); len(rest) != 0 {
		t.Fatalf("re-Drain should be empty, got %d", len(rest))
	}
}

func TestBroker_IgnoresEmpty(t *testing.T) {
	b := newAskBroker()
	b.Post("", agentapi.ChildQuestion{ID: "q"}) // no parent
	b.Post("p", agentapi.ChildQuestion{ID: ""}) // no id
	if len(b.Drain("")) != 0 || len(b.Drain("p")) != 0 {
		t.Fatal("empty parent or empty id must not enqueue")
	}
}

func TestChildTopicAction(t *testing.T) {
	cases := map[string]string{
		"child.c1.suspended": commsapi.EventAsked,
		"child.c1.done":      commsapi.EventCompleted,
		"child.c1.failed":    commsapi.EventFailed,
		"child.c1.started":   "",
		"to.x.message":       "",
		"":                   "",
	}
	for topic, want := range cases {
		if got := childTopicAction(topic); got != want {
			t.Errorf("childTopicAction(%q) = %q, want %q", topic, got, want)
		}
	}
}

func TestChildEventMatchesParent(t *testing.T) {
	evt := &engine.ChildSuspendedEvent{}
	evt.Correlation = engine.Correlation{ParentRunID: "P"}
	if !childEventMatchesParent(evt, "P") {
		t.Fatal("should match same parent")
	}
	if childEventMatchesParent(evt, "Q") {
		t.Fatal("should not match different parent")
	}
	if childEventMatchesParent("not-correlated", "P") {
		t.Fatal("non-correlated payload should not match")
	}
}
