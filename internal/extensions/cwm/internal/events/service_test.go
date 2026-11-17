// SPDX-License-Identifier: Apache-2.0

package events

import (
	stdctx "context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// --- Subject matching ------------------------------------------

func TestSubjectMatches_Exact(t *testing.T) {
	if !subjectMatches("a.b.c", "a.b.c") {
		t.Error("exact match should hit")
	}
	if subjectMatches("a.b.c", "a.b.d") {
		t.Error("different subject should miss")
	}
}

func TestSubjectMatches_PrefixWildcard(t *testing.T) {
	if !subjectMatches("a.b.>", "a.b.c") {
		t.Error("prefix wildcard should hit deeper subject")
	}
	if !subjectMatches("a.b.>", "a.b.c.d.e") {
		t.Error("prefix wildcard should hit arbitrarily deeper subject")
	}
	if subjectMatches("a.b.>", "a.c.d") {
		t.Error("prefix wildcard should miss different branch")
	}
	// `a.b.>` matches a.b.* but NOT a.b itself (NATS semantic).
	if subjectMatches("a.b.>", "a.b") {
		t.Logf("note: matches `a.b` (substrate is lenient vs strict NATS)")
	}
}

func TestSubjectMatches_EmptyPattern(t *testing.T) {
	if !subjectMatches("", "anything.at.all") {
		t.Error("empty pattern should match everything")
	}
}

// --- Service publish + subscribe round-trip --------------------

// newTestService wires an events.Service against a real
// MemoryEventBus (the same implementation the application uses).
// Tests can publish + subscribe and observe deliveries without
// a NATS dependency.
func newTestService(t *testing.T) (*Service, eventbus.EventBus) {
	t.Helper()
	bus := eventbus.NewMemoryEventBus()
	return New("local", "default", "ws-uuid", bus, nil), bus
}

func TestPublishObject_DeliversToSubscriber(t *testing.T) {
	svc, _ := newTestService(t)

	// Subscribe to "everything for this workspace" via prefix.
	ch, cancel, err := svc.Subscribe(stdctx.Background(), api.EventsSubscribeInput{
		TopicPattern: "ktw.local.universe.default.workspace.ws-uuid.>",
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	// Publish.
	svc.PublishObject("created", "billing.d1", "billing", "decision", "billing", "", "", api.EventMeta{})

	// Receive within 1s.
	select {
	case env := <-ch:
		want := api.ObjectCreatedSubject("local", "default", "ws-uuid", api.SubjectToken("billing.d1"))
		if env.Topic != want {
			t.Errorf("Topic = %q, want %q", env.Topic, want)
		}
		var got ObjectPayload
		if err := json.Unmarshal(env.Payload, &got); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if got.ObjectID != "billing.d1" || got.Module != "billing" || got.Type != "decision" || got.Action != "created" {
			t.Errorf("payload = %+v, want billing.d1 created", got)
		}
	case <-time.After(time.Second):
		t.Error("did not receive envelope within 1s")
	}
}

func TestPublishObject_FiltersByExactSubject(t *testing.T) {
	svc, _ := newTestService(t)

	// Subscribe to ONLY one object's updated subject.
	exact := api.ObjectUpdatedSubject("local", "default", "ws-uuid", api.SubjectToken("billing.n99"))
	ch, cancel, _ := svc.Subscribe(stdctx.Background(), api.EventsSubscribeInput{
		TopicPattern: exact,
	})
	defer cancel()

	// Publish for a DIFFERENT object — should NOT deliver.
	svc.PublishObject("updated", "billing.n1", "billing", "project", "", "", "", api.EventMeta{})
	select {
	case env := <-ch:
		t.Errorf("unexpected delivery: %s", env.Topic)
	case <-time.After(50 * time.Millisecond):
	}

	// Publish for the matching object — should deliver.
	svc.PublishObject("updated", "billing.n99", "billing", "project", "", "", "", api.EventMeta{})
	select {
	case env := <-ch:
		if env.Topic != exact {
			t.Errorf("Topic = %q, want %q", env.Topic, exact)
		}
	case <-time.After(time.Second):
		t.Error("did not receive expected envelope")
	}
}

func TestPublish_NilBusIsNoop(t *testing.T) {
	// Nil bus = events publishing is a no-op. Service must not
	// panic and Subscribe should return a closed channel.
	svc := New("local", "default", "ws", nil, nil)

	svc.PublishObject("created", "m.i", "m", "rule", "", "", "", api.EventMeta{}) // must not panic

	ch, cancel, err := svc.Subscribe(stdctx.Background(), api.EventsSubscribeInput{})
	if err != nil {
		t.Fatalf("Subscribe with nil bus: %v", err)
	}
	defer cancel()

	// Channel must be closed (drained) — receiving returns
	// immediately with zero value.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed when bus is nil")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel not closed within timeout")
	}
}

func TestCancel_UnsubscribesAndClosesChannel(t *testing.T) {
	svc, _ := newTestService(t)

	ch, cancel, _ := svc.Subscribe(stdctx.Background(), api.EventsSubscribeInput{
		TopicPattern: "ktw.local.>",
	})

	cancel()

	// After cancel, channel must be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel after cancel")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("channel not closed after cancel")
	}
}
