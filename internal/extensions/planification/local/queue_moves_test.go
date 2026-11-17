// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// « À faire » is built from the scheduler's to-dos too, whose moves carry no
// CWM event: a to-do put or archived after the page's read (the scheduler's
// plan.changed) makes the next click compute the queue again, so the click
// shows the new item and no longer the archived one.
func TestAToDosMoveMakesTheNextClickComputeAgain(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	views := views(t, ctx)
	page := func(in api.PersonQueueInput) string {
		t.Helper()
		in.Language = "en"
		def, err := views.PersonQueuePage(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def)
		return string(raw)
	}
	click := api.PersonQueueInput{Action: api.ActionValidate}
	const title = "Validate the moved export"

	page(api.PersonQueueInput{}) // the page's read, kept for its clicks
	put, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: schedapi.Todo{
		Key: "ht-moved", Module: "billing", Target: "billing", Title: title, For: schedapi.ForPerson, Start: "manual", Priority: "P1",
		State: schedapi.StateReady, Owner: inboxapi.Owner, Work: map[string]any{"task": inboxapi.HumanTaskStrategy,
			inboxapi.WorkAsk: map[string]any{"kind": inboxapi.KindApprove, "title": title, "question": "The export", "asked_by": "agent:reviewer"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page(click), title) {
		t.Fatal("a click after a to-do was put was answered from the queue read before it")
	}

	page(api.PersonQueueInput{})
	if _, err := schedOf(t, ctx).Archive(ctx, schedapi.ArchiveInput{ID: put.Todo.ID}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page(click), title) {
		t.Fatal("a click after a to-do was archived still shows it")
	}

}
