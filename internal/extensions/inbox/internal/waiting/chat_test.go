// SPDX-License-Identifier: Apache-2.0

package waiting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/internal/humantask"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// TestTheChatsCardIsDrawnByÀFairesFrame:
// a chat's card for a question is drawn by « À faire »'s one frame
// (api.HumanTaskCard), given the question as the waiting list reads it - its
// key, title and what it is about - with the nodes that answer it in « À
// faire », reporting the same to-do; it arrives resolved. Answered, the frame
// gets no answer nodes but who answered and what - a relayed answer naming who
// relayed it and the proof. Without a frame, the question's title alone.
func TestTheChatsCardIsDrawnByÀFairesFrame(t *testing.T) {
	ctx := context.Background()
	td := humantask.TodoOf(api.HumanTask{Kind: api.KindApprove, Title: "Validate the plan", About: "ui.F-061", Chat: "run-chat"}, "run-ht", time.Now())
	td.ID = "ui.TD-1"

	was := api.HumanTaskCard
	defer func() { api.HumanTaskCard = was }()
	api.HumanTaskCard = nil
	if bare := ChatCard(ctx, td); bare["id"] != "inbox:ui.TD-1" || !strings.Contains(asJSON(bare), "Validate the plan") {
		t.Errorf("with no frame: %s; want the question's title alone", asJSON(bare))
	}

	var got api.WaitingItem
	var gotAnswered string
	api.HumanTaskCard = func(_ context.Context, it api.WaitingItem, answered, _ string) render.Node {
		got, gotAnswered = it, answered
		return render.Node{"kind": "card", "id": it.Key, "children": []any{it.Answer}}
	}
	inChat := ChatCard(ctx, td)
	if got.Key != "inbox:ui.TD-1" || got.Title != "Validate the plan" || got.About != "ui.F-061" || got.Module != "ui" || gotAnswered != "" {
		t.Fatalf("the frame got %+v (answered %q); want the waiting question", got, gotAnswered)
	}
	s := asJSON(inChat)
	if !strings.Contains(s, todosReportURI) || !strings.Contains(s, `"ui.TD-1"`) || !strings.Contains(s, `"id":"approve:ui.TD-1"`) {
		t.Errorf("the chat's card does not answer through « À faire »'s nodes, reporting its to-do: %s", s)
	}
	if strings.Contains(s, `"ui":`) {
		t.Errorf("the chat's card carries an unresolved answer form: %s", s)
	}

	td.State = schedapi.StateReported
	td.Result = &schedapi.Result{Status: schedapi.StatusSuccess, Verdict: "approved", Summary: "ok",
		By: cwmapi.Actor{ID: "jdoe", Kind: "person"}, Relayed: &schedapi.Relayed{By: "claude-1", Confirmed: "prompt"}}
	ChatCard(ctx, td)
	if len(got.Answer) != 0 || !strings.Contains(gotAnswered, "answered by jdoe · approved") || !strings.Contains(gotAnswered, "relayed by claude-1, confirmed by prompt") {
		t.Errorf("answered, the frame got answer %v and %q; want no answer nodes, who answered and who relayed it", got.Answer, gotAnswered)
	}
}

// TestTheRailsInboxIsNamedByTheBackend: the rail's Inbox opens
// what the inbox names - « À faire »'s page and count, standing in for
// planification's former « À faire » panel.
func TestTheRailsInboxIsNamedByTheBackend(t *testing.T) {
	on, err := New().Inbox(context.Background(), api.InboxInput{})
	if err != nil || on.Page != "ktw-agents:views/person-queue-page" || on.Count != "ktw-agents:views/person-queue-count" || len(on.Replaces) != 1 || on.Replaces[0] != "person-queue" {
		t.Fatalf("%+v %v; want planification's « À faire » page and count reused (ops.RQ-383), replacing person-queue on the strip", on, err)
	}
}
