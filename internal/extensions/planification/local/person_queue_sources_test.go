// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// fakeMemorySource is a source contributed to « À faire » the way
// agent-memory's quarantined memory is: one question, answered by its own
// controls (two buttons named by element id).
func fakeMemorySource() inboxapi.WaitingSource {
	return inboxapi.WaitingSource{
		Name: "fake-memory",
		Items: func(context.Context) ([]inboxapi.WaitingItem, error) {
			return []inboxapi.WaitingItem{{Key: "fake-memory:old-notes", Kind: inboxapi.KindChoose,
				Title: "Agent memory written before the wall: keep it or delete it", Module: "billing",
				Since: "2026-11-05T10:00:00Z", Ref: "old-notes", AnswerableBy: "person"}}, nil
		},
		Controls: func(_ context.Context, it inboxapi.WaitingItem, _ string) []render.Node {
			return []render.Node{{"kind": "button", "id": "fake-memory-assign-" + it.Ref}, {"kind": "button", "id": "fake-memory-delete-" + it.Ref}}
		},
	}
}

// EVERY CONTRIBUTED SOURCE IS DRAWN IN « À FAIRE »:
// a question another source contributes to the inbox's one list - counted by
// the rail's badge - is a card of the page the rail opens, carrying its
// source's controls, as the inbox's own questions are.
func TestAContributedSourcesQuestionIsACardWithItsControls(t *testing.T) {
	t.Parallel()
	ctx, _, _ := worldExtended(t, nil, nil, func(init *application.ApplicationInit) {
		init.AddContribute("fake-memory", ".waiting", func(app *application.Application) error {
			return application.Contribute(app, fakeMemorySource())
		})
	})
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	q, err := views.PersonQueue(ctx, api.PersonQueueInput{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	var card *api.PersonItem
	for i := range q.Items {
		if q.Items[i].Key == "fake-memory:old-notes" {
			card = &q.Items[i]
		}
	}
	if card == nil {
		keys := make([]string, 0, len(q.Items))
		for _, it := range q.Items {
			keys = append(keys, it.Key)
		}
		t.Fatalf("the contributed source's question is no card of « À faire »: items %v (unread %q)", keys, q.Unread)
	}
	ids := map[string]bool{}
	for _, n := range card.Answer {
		if id, _ := n["id"].(string); id != "" {
			ids[id] = true
		}
	}
	if !ids["fake-memory-assign-old-notes"] || !ids["fake-memory-delete-old-notes"] {
		t.Errorf("the card's answer = %v; want its source's two controls", card.Answer)
	}
	if card.Action != api.ActionDecide {
		t.Errorf("the card's action = %q; want %q (a choice)", card.Action, api.ActionDecide)
	}
}
