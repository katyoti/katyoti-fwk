// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// EventsSubscribe: an object write delivers its envelope to a subscriber whose
// pattern matches; once cancelled, the channel closes and no further write
// reaches it.
func TestEventsSubscribeDeliversAWriteUntilCancelled(t *testing.T) {
	svc, bus := eventsFixture(t)
	ctx := stdctx.Background()
	before := bus.TotalSubscribers()
	ch, cancel, err := svc.EventsSubscribe(ctx, api.EventsSubscribeInput{TopicPattern: "ktw.t.universe.u.workspace.w.>"})
	if err != nil {
		t.Fatal(err)
	}
	o := treeObject("billing.spec", "")
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	want := api.ObjectCreatedSubject("t", "u", "w", api.SubjectToken(o.ID))
	select {
	case env := <-ch:
		if env.Topic != want {
			t.Errorf("topic %q, want %q", env.Topic, want)
		}
		var p events.ObjectPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.ObjectID != o.ID || p.Module != "billing" || p.Action != "created" {
			t.Errorf("payload %s (%v), want billing.spec created", env.Payload, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the create delivered nothing")
	}

	cancel()
	cancel() // idempotent
	if n := bus.TotalSubscribers(); n != before {
		t.Errorf("subscribers after cancel = %d, want %d (released)", n, before)
	}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	select {
	case env, ok := <-ch:
		if ok {
			t.Errorf("delivered after cancel: %s", env.Topic)
		}
	case <-time.After(time.Second):
		t.Error("the channel did not close on cancel")
	}
}

// A pattern naming another object's subject receives nothing of this write.
func TestEventsSubscribeFiltersByPattern(t *testing.T) {
	svc, _ := eventsFixture(t)
	ctx := stdctx.Background()
	other := api.ObjectCreatedSubject("t", "u", "w", api.SubjectToken("billing.other"))
	ch, cancel, err := svc.EventsSubscribe(ctx, api.EventsSubscribeInput{TopicPattern: other})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: treeObject("billing.spec", "")}); err != nil {
		t.Fatal(err)
	}
	select {
	case env := <-ch:
		t.Errorf("unexpected delivery %s", env.Topic)
	case <-time.After(100 * time.Millisecond):
	}
}
