// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// countingInbox counts the requests filed.
type countingInbox struct {
	*fakeInbox
	mu    sync.Mutex
	filed int
}

func (c *countingInbox) File(ctx context.Context, in inboxapi.FileInput) (inboxapi.FileOutput, error) {
	c.mu.Lock()
	c.filed++
	c.mu.Unlock()
	return c.fakeInbox.File(ctx, in)
}

// In a loop that waits as a machine, the ask does not block: it files the
// request and awaits its update. Called again it files nothing: still pending
// → it awaits again; answered → the answer. No suspension, no task service.
func TestInboxAskAwaitsInAMachineLoop(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	inbox := &countingInbox{fakeInbox: &fakeInbox{status: inboxapi.StatusPending}}
	application.PutService[inboxapi.InboxService](app, "default", inbox)
	ctx := agentapi.WithMachineWaits(application.WithApplication(context.Background(), app))

	_, err := (&inboxChannel{}).Ask(ctx, AskInput{Prompt: "Valider le plan ?"})
	var aw *agentapi.ErrAwait
	if !errors.As(err, &aw) {
		t.Fatalf("first ask: %v, want an await", err)
	}
	if aw.EventURI != inboxapi.UpdatedEventURI("req-1") || aw.Resume[requestKey] != "req-1" {
		t.Errorf("await = %+v, want the request's update event and its id", aw)
	}

	again := agentapi.WithAwaitResume(ctx, aw.Resume)
	if _, err := (&inboxChannel{}).Ask(again, AskInput{Prompt: "Valider le plan ?"}); !errors.As(err, &aw) {
		t.Fatalf("still pending: %v, want an await again", err)
	}

	inbox.mu.Lock()
	inbox.fakeInbox.status, inbox.fakeInbox.note = "approved", "oui, go"
	inbox.mu.Unlock()
	out, err := (&inboxChannel{}).Ask(again, AskInput{Prompt: "Valider le plan ?"})
	if err != nil || out.Answer != "oui, go" {
		t.Fatalf("answered: %+v, %v; want the decision note", out, err)
	}
	if inbox.filed != 1 {
		t.Errorf("filed %d requests, want one", inbox.filed)
	}
}

// A request answered without a note answers with its status; a dismissed one
// says it was dismissed — the model never reads "dismissed" as an answer.
func TestAnAnswerIsTheNoteElseTheStatus(t *testing.T) {
	if _, ok := answerOf(inboxapi.Entry{Status: inboxapi.StatusPending}); ok {
		t.Error("a pending request is not an answer")
	}
	if out, ok := answerOf(inboxapi.Entry{Status: "approved"}); !ok || out.Answer != "approved" {
		t.Errorf("no note: %+v, want the status", out)
	}
	if out, ok := answerOf(inboxapi.Entry{Status: inboxapi.StatusDismissed}); !ok || !out.Dismissed || out.Answer == "dismissed" {
		t.Errorf("dismissed: %+v, want it marked dismissed, not an answer", out)
	}
	if out, _ := answerOf(inboxapi.Entry{Status: "approved", DecisionNote: "yes"}); out.Answer != "yes" {
		t.Errorf("with a note: %+v, want the note", out)
	}
}
