// SPDX-License-Identifier: Apache-2.0

package events

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// THE NATS BRIDGE. Every Publish* also goes out on the cross-sidecar bus, on
// the subject built from the service's tenant / universe / workspace and the
// object's SubjectToken (never its id), with the JSON of the payload. A
// failing bus or no bus at all costs the in-process publish nothing.

// recordingBus is a MessageBus that records Publish; any other method panics
// (the bridge must not call them).
type recordingBus struct {
	mbapi.MessageBus
	mu   sync.Mutex
	pubs []published
	err  error
}

type published struct {
	subject string
	body    []byte
}

func (b *recordingBus) Publish(_ stdctx.Context, subject string, payload []byte, _ ...mbapi.PubOpt) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pubs = append(b.pubs, published{subject, append([]byte(nil), payload...)})
	return b.err
}

func (b *recordingBus) all() []published {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]published(nil), b.pubs...)
}

func TestPublishObject_BridgesToNATSOnTheTokenSubject(t *testing.T) {
	mb := &recordingBus{}
	svc := New("acme", "prod", "ws-1", eventbus.NewMemoryEventBus(), mb)
	const id = "billing.Spec-v2" // an authored id: `.` is the NATS separator
	svc.PublishObject("created", id, "billing", "doc", "billing", "spec", "", api.EventMeta{})
	svc.PublishObject("deleted", id, "billing", "doc", "billing", "spec", "", api.EventMeta{})
	svc.PublishObject("bogus", id, "billing", "doc", "", "", "", api.EventMeta{}) // an unknown action publishes nothing

	tok := api.SubjectToken(id)
	want := []string{
		api.ObjectCreatedSubject("acme", "prod", "ws-1", tok),
		api.ObjectDeletedSubject("acme", "prod", "ws-1", tok),
	}
	got := mb.all()
	if len(got) != len(want) {
		t.Fatalf("NATS publishes = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, p := range got {
		if p.subject != want[i] {
			t.Errorf("publish %d on %q, want %q", i, p.subject, want[i])
		}
		var payload ObjectPayload
		if err := json.Unmarshal(p.body, &payload); err != nil || payload.ObjectID != id || payload.Module != "billing" {
			t.Errorf("publish %d body %s (%v), want the payload naming %s", i, p.body, err, id)
		}
	}
	if got[0].subject != "ktw.acme.universe.prod.workspace.ws-1.object."+tok+".created" {
		t.Errorf("subject %q is not scoped tenant/universe/workspace + token", got[0].subject)
	}
}

func TestPublishTransitioned_BridgesToNATS(t *testing.T) {
	mb := &recordingBus{}
	svc := New("acme", "prod", "ws-1", nil, mb) // no in-process bus: NATS still gets it
	svc.PublishObjectTransitioned(ObjectTransitionedPayload{ObjectID: "m.g", Module: "m", From: "a", To: "b"})
	got := mb.all()
	if len(got) != 1 || got[0].subject != api.ObjectTransitionedSubject("acme", "prod", "ws-1", api.SubjectToken("m.g")) {
		t.Fatalf("publishes %+v", got)
	}
	var p ObjectTransitionedPayload
	if err := json.Unmarshal(got[0].body, &p); err != nil || p.From != "a" || p.To != "b" {
		t.Errorf("body %s (%v)", got[0].body, err)
	}
}

// A failing NATS bus does not stop the in-process delivery.
func TestPublish_NATSFailureKeepsInProcessDelivery(t *testing.T) {
	mb := &recordingBus{err: errors.New("connection closed")}
	svc := New("acme", "prod", "ws-1", eventbus.NewMemoryEventBus(), mb)
	ch, cancel, _ := svc.Subscribe(stdctx.Background(), api.EventsSubscribeInput{})
	defer cancel()
	svc.PublishObject("updated", "m.x", "m", "doc", "", "", "", api.EventMeta{})
	select {
	case env := <-ch:
		if env.Topic != api.ObjectUpdatedSubject("acme", "prod", "ws-1", api.SubjectToken("m.x")) {
			t.Errorf("topic %q", env.Topic)
		}
	case <-time.After(time.Second):
		t.Error("in-process delivery lost when NATS failed")
	}
	if len(mb.all()) != 1 {
		t.Errorf("NATS attempted %d times, want 1", len(mb.all()))
	}
}

// No NATS bus, or a nil service: publishing does not panic.
func TestPublish_NoNATSBusDoesNotPanic(t *testing.T) {
	svc := New("acme", "prod", "ws-1", eventbus.NewMemoryEventBus(), nil)
	svc.PublishObject("created", "m.x", "m", "doc", "", "", "", api.EventMeta{})
	svc.publishToNATS("ktw.acme.x", map[string]any{"a": 1})
	var none *Service
	none.publishToNATS("ktw.acme.x", nil)
	none.PublishObject("created", "m.x", "m", "doc", "", "", "", api.EventMeta{})
}
