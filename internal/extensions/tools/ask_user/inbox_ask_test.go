// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// fakeInbox files one request and answers Get the way the real service does:
// it reaches CWM through the application in its context, so a context
// without one fails.
type fakeInbox struct {
	inboxapi.InboxService
	mu     sync.Mutex
	status string
	note   string
	filed  inboxapi.FileInput
}

func (f *fakeInbox) File(_ context.Context, in inboxapi.FileInput) (inboxapi.FileOutput, error) {
	f.mu.Lock()
	f.filed = in
	f.mu.Unlock()
	return inboxapi.FileOutput{Request: inboxapi.Entry{ID: "req-1", Status: inboxapi.StatusPending}}, nil
}

func (f *fakeInbox) Get(ctx context.Context, in inboxapi.GetInput) (inboxapi.GetOutput, error) {
	if application.AppFromContext(ctx) == nil {
		return inboxapi.GetOutput{}, errors.New("no Application in context")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return inboxapi.GetOutput{Entry: inboxapi.Entry{ID: in.ID, Status: f.status, DecisionNote: f.note}}, nil
}

// fakeEvents keeps the handler; the test fires it with a bare context, as a
// NATS delivery does.
type fakeEvents struct {
	eventsapi.EventService
	mu      sync.Mutex
	handler eventsapi.Handler
}

type noSub struct{}

func (noSub) Unsubscribe() error { return nil }

func (f *fakeEvents) Subscribe(_ context.Context, _ string, _ string, h eventsapi.Handler) (eventsapi.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = h
	return noSub{}, nil
}

func (f *fakeEvents) fire() bool {
	f.mu.Lock()
	h := f.handler
	f.mu.Unlock()
	if h == nil {
		return false
	}
	h(context.Background(), json.RawMessage(`{}`))
	return true
}

type fakeSuspensions struct {
	taskapi.SuspensionManager
	mu sync.Mutex
	ch chan json.RawMessage
}

func (f *fakeSuspensions) Suspend(string, string) (<-chan json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = make(chan json.RawMessage, 1)
	return f.ch, nil
}

func (f *fakeSuspensions) Resume(_ string, payload json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch <- payload
	return nil
}

type fakeTasks struct {
	taskapi.TaskService
	sm *fakeSuspensions
}

func (f fakeTasks) Repeat(context.Context, taskapi.RepeatRequest) (taskapi.TaskRunResponse, error) {
	return taskapi.TaskRunResponse{}, nil
}
func (f fakeTasks) SuspensionManager() taskapi.SuspensionManager { return f.sm }

// The answer given in the inbox resumes the task — even though the event
// that announces it arrives on a context that carries no application.
func TestInboxChannel_AnswerResumesTheTask(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	inbox, events := &fakeInbox{status: inboxapi.StatusPending}, &fakeEvents{}
	application.PutService[inboxapi.InboxService](app, "default", inbox)
	application.PutService[eventsapi.EventService](app, "default", events)
	application.PutService[taskapi.TaskService](app, "default", fakeTasks{sm: &fakeSuspensions{}})
	ctx := application.WithApplication(context.Background(), app)

	done := make(chan AskOutput, 1)
	errs := make(chan error, 1)
	go func() {
		out, err := (&inboxChannel{}).Ask(ctx, AskInput{Prompt: "Écrire le thème ?"})
		if err != nil {
			errs <- err
			return
		}
		done <- out
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !events.fire() {
		if time.Now().After(deadline) {
			t.Fatal("the ask never subscribed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Still pending: the task keeps waiting.
	select {
	case out := <-done:
		t.Fatalf("resumed while pending: %+v", out)
	case <-time.After(50 * time.Millisecond):
	}

	inbox.mu.Lock()
	inbox.status, inbox.note = "approved", "oui"
	inbox.mu.Unlock()
	events.fire()
	select {
	case out := <-done:
		if out.Answer != "oui" {
			t.Errorf("answer = %q, want oui", out.Answer)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("the answer did not resume the task")
	}
}

func (f fakeTasks) Groups(context.Context, taskapi.GroupsRequest) (taskapi.GroupsResponse, error) {
	return taskapi.GroupsResponse{}, nil
}
func (f fakeTasks) SetGroup(context.Context, taskapi.SetGroupRequest) (taskapi.GroupLoad, error) {
	return taskapi.GroupLoad{}, nil
}
func (f fakeTasks) ClearGroup(context.Context, taskapi.ClearGroupRequest) (taskapi.ClearGroupResponse, error) {
	return taskapi.ClearGroupResponse{}, nil
}

// A request's title is the question's first line, without markdown marks.
func TestTitleOf(t *testing.T) {
	for q, want := range map[string]string{
		"## Validation du thème `acme-new`\n\nLe thème sera écrit…": "Validation du thème `acme-new`",
		"\n---\n**Écrire le thème ?**":                              "Écrire le thème ?",
		"":                                                          "Waiting for user input",
	} {
		if got := titleOf(q); got != want {
			t.Errorf("titleOf(%q) = %q, want %q", q, got, want)
		}
	}
	if got := []rune(titleOf(strings.Repeat("a", 300))); len(got) != 120 {
		t.Errorf("long title: %d runes", len(got))
	}
}

// An answer given before the subscription was live (no event the handler
// sees) still resumes the task: the request is read once after subscribing.
func TestInboxChannel_AnswerBeforeTheSubscription(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	inbox := &fakeInbox{status: "approved", note: "oui"}
	application.PutService[inboxapi.InboxService](app, "default", inbox)
	application.PutService[eventsapi.EventService](app, "default", &fakeEvents{})
	application.PutService[taskapi.TaskService](app, "default", fakeTasks{sm: &fakeSuspensions{}})
	ctx, cancel := context.WithTimeout(application.WithApplication(context.Background(), app), 2*time.Second)
	defer cancel()
	out, err := (&inboxChannel{}).Ask(ctx, AskInput{Prompt: "Écrire le thème ?"})
	if err != nil {
		t.Fatalf("the answer was missed: %v", err)
	}
	if out.Answer != "oui" {
		t.Errorf("answer = %q", out.Answer)
	}
	if inbox.filed.Expects != inboxapi.ExpectsText {
		t.Errorf("the question was filed expecting %q; want text: it is answered, or dismissed, never rejected", inbox.filed.Expects)
	}
}
