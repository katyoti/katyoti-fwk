// SPDX-License-Identifier: Apache-2.0

// Package ask_user — inbox_ask.go is the async inbox-routing impl.
// Used when UserCommsInput is `inbox`.
//
// Flow when all three runtime services are reachable:
//
//  1. InboxService.File files a pending approval-request in the project's
//     inbox: title=question, description=full prompt, requested_by=task ID,
//     blocks_task=true. The inbox service owns where the inbox is and how a
//     request is shaped; this channel never touches CWM itself.
//  2. SuspensionManager.Suspend(taskID, inboxapi.UpdatedEventURI(id))
//     returns a per-task resume channel.
//  3. EventService.Subscribe to the same URI; the handler reads the request
//     back through InboxService.Get — once it is no longer pending it
//     packages an AskOutput and resumes the suspended task.
//  4. The goroutine blocks on (resume channel, ctx.Done()) and returns the
//     AskOutput.
//
// In an agent loop that waits as a machine (agentapi.MachineWaitsFrom), the
// ask does not block: it files the request and returns agentapi.ErrAwait on the
// request's update event. The loop waits — no slot held, its state saved, a
// restart survived — and calls the ask again with the request on its context
// (agentapi.AwaitResumeFrom): answered → the answer; still pending → ErrAwait
// again. The steps above are the blocking path, kept for the other callers.
//
// Fallback: if any of the three services (Inbox, Events,
// TaskService.SuspensionManager) is missing from ctx, the impl returns
// ErrInboxNotImplemented naming the resolution failure. Binaries without the
// inbox + events + task stack get a clean error instead of a half-broken inbox.
package ask_user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// ErrInboxNotImplemented surfaces when the runtime is missing a
// service the inbox path needs. The wrapped error names which
// service is unreachable so operators can diagnose quickly. The
// sentinel still wraps ErrNoUserChannel so generic channel-error
// handlers see the same chain; unwrap for specifics.
var ErrInboxNotImplemented = fmt.Errorf("%w (inbox channel requires InboxService + EventService + TaskService in this process)", ErrNoUserChannel)

type inboxChannel struct{}

func (c *inboxChannel) Ask(ctx context.Context, in AskInput) (AskOutput, error) {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return AskOutput{}, fmt.Errorf("%w [InboxService: %v]", ErrInboxNotImplemented, err)
	}
	if agentapi.MachineWaitsFrom(ctx) {
		return askAndAwait(ctx, inbox, in)
	}
	ev, err := application.UseService[eventsapi.EventService](ctx)
	if err != nil {
		return AskOutput{}, fmt.Errorf("%w [EventService: %v]", ErrInboxNotImplemented, err)
	}
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return AskOutput{}, fmt.Errorf("%w [TaskService: %v]", ErrInboxNotImplemented, err)
	}
	sm := ts.SuspensionManager()
	if sm == nil {
		return AskOutput{}, fmt.Errorf("%w [SuspensionManager: nil]", ErrInboxNotImplemented)
	}

	taskID := loopIDFromContext(ctx)
	question := in.PromptText()
	title := titleOf(question)
	filed, err := inbox.File(ctx, inboxapi.FileInput{
		Title: title, Description: question, RequestedBy: taskID, BlocksTask: true, Expects: inboxapi.ExpectsText,
	})
	if err != nil {
		return AskOutput{}, fmt.Errorf("ask-user: file the request: %w", err)
	}
	itemID := filed.Request.ID
	waitURI := inboxapi.UpdatedEventURI(itemID)

	resumeCh, err := sm.Suspend(taskID, waitURI)
	if err != nil {
		return AskOutput{}, fmt.Errorf("ask-user: SuspensionManager.Suspend: %w", err)
	}

	// Subscribe BEFORE blocking — handler fires Resume on item
	// status change. Workspace scopeID "*" so handlers see updates
	// regardless of which workspace the item lives in (a future
	// enhancement narrows this to the task's owning workspace).
	// check reads the request and resumes the task once it is answered —
	// through the ask's own context, which carries the application the inbox
	// reaches CWM by (a delivery's context carries none; the ask's stays
	// alive until the task resumes).
	check := func() {
		got, err := inbox.Get(ctx, inboxapi.GetInput{ID: itemID})
		if err != nil {
			slog.Warn("inbox ask: reading the request failed", "itemID", itemID, "error", err)
			return
		}
		out, answered := answerOf(got.Entry)
		if !answered {
			slog.Info("inbox ask: item still pending — keep waiting", "itemID", itemID, "status", got.Entry.Status)
			return
		}
		outJSON, _ := json.Marshal(out)
		if err := sm.Resume(taskID, outJSON); err != nil {
			slog.Warn("inbox ask: Resume failed", "taskID", taskID, "error", err)
			return
		}
		slog.Info("inbox ask: task resumed", "taskID", taskID, "answer", out.Answer)
	}
	// release ends a wait that will not be answered here (the subscription
	// failed, the ask was cancelled): the suspension is marked delivered, so
	// the task's next ask starts a fresh one instead of inheriting this one.
	release := func() { _ = sm.Resume(taskID, json.RawMessage(`null`)) }
	sub, subErr := ev.Subscribe(ctx, waitURI, "*", func(_ context.Context, _ json.RawMessage) {
		slog.Info("inbox ask: item.updated handler fired", "itemID", itemID, "taskID", taskID)
		check() // the payload may not carry the full body: read the request back
	})
	if subErr != nil {
		release()
		return AskOutput{}, fmt.Errorf("ask-user: EventService.Subscribe: %w", subErr)
	}
	// An answer given before the subscription was live fired no event the
	// handler saw: read the request once now.
	check()
	slog.Info("inbox ask: suspended waiting on inbox item", "itemID", itemID, "taskID", taskID, "waitURI", waitURI)
	defer func() {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}()

	// Block until handler resumes us OR ctx cancels.
	select {
	case payload := <-resumeCh:
		var out AskOutput
		if err := json.Unmarshal(payload, &out); err != nil {
			return AskOutput{}, fmt.Errorf("ask-user: decode resumed payload: %w", err)
		}
		return out, nil
	case <-ctx.Done():
		release()
		return AskOutput{}, fmt.Errorf("ask-user: cancelled while waiting on inbox item %s: %w", itemID, ctx.Err())
	}
}

// titleOf is a request's title: the question's first line without its
// markdown marks, at most 120 characters. The whole question is the
// request's description, rendered as markdown where it is read.
func titleOf(question string) string {
	for _, line := range strings.Split(question, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#>-* "))
		line = strings.ReplaceAll(line, "**", "")
		if line == "" || strings.Trim(line, "-|: ") == "" {
			continue
		}
		if r := []rune(line); len(r) > 120 {
			line = string(r[:119]) + "…"
		}
		return line
	}
	return "Waiting for user input"
}

// requestKey is the key of the request id in an await's Resume.
const requestKey = "request"

// askAndAwait is the ask of a loop that waits as a machine. Called first, it
// files the request and awaits its update; called again (the update came, or
// the run was restored after a restart), it reads the request: answered → the
// answer, still pending → await again.
func askAndAwait(ctx context.Context, inbox inboxapi.InboxService, in AskInput) (AskOutput, error) {
	if resume, ok := agentapi.AwaitResumeFrom(ctx); ok {
		id, _ := resume[requestKey].(string)
		if id == "" {
			return AskOutput{}, fmt.Errorf("ask-user: called again without its inbox request")
		}
		got, err := inbox.Get(ctx, inboxapi.GetInput{ID: id})
		switch {
		case errors.Is(err, inboxapi.ErrGone):
			// Deleted, or moved out of the inbox: no answer will come.
			return AskOutput{}, fmt.Errorf("ask-user: the inbox request %s is gone (deleted or moved out of the inbox): no answer will come — ask again if you still need one: %w", id, err)
		case err != nil:
			// Unreadable now (the workspace starting): keep waiting — the
			// request's next update, or the next periodic check, asks again.
			slog.Warn("inbox ask: reading the request failed, waiting on", "request", id, "error", err)
		default:
			if out, answered := answerOf(got.Entry); answered {
				return out, nil
			}
		}
		return AskOutput{}, awaitAnswer(id)
	}

	question := in.PromptText()
	filed, err := inbox.File(ctx, inboxapi.FileInput{
		Title: titleOf(question), Description: question, RequestedBy: loopIDFromContext(ctx), BlocksTask: true,
		Expects: inboxapi.ExpectsText,
	})
	if err != nil {
		return AskOutput{}, fmt.Errorf("ask-user: file the request: %w", err)
	}
	slog.Info("inbox ask: request filed, the run waits for its answer", "request", filed.Request.ID)
	return AskOutput{}, awaitAnswer(filed.Request.ID)
}

// inboxRecheckEvery is how often a waiting ask reads its request again, event
// or not: an answer whose event no process heard, a request gone.
const inboxRecheckEvery = time.Hour

func awaitAnswer(requestID string) *agentapi.ErrAwait {
	return &agentapi.ErrAwait{
		EventURI:     inboxapi.UpdatedEventURI(requestID),
		Resume:       map[string]any{requestKey: requestID},
		Reason:       "an answer in the inbox (" + requestID + ")",
		RecheckEvery: inboxRecheckEvery,
	}
}

// answerOf is a request's answer once it is no longer pending: its decision
// note (a free-form answer), else its status (approved, rejected…). A
// dismissed request is no answer: it says the question was dismissed.
func answerOf(e inboxapi.Entry) (AskOutput, bool) {
	if e.Status == "" || e.Status == inboxapi.StatusPending {
		return AskOutput{}, false
	}
	if e.Status == inboxapi.StatusDismissed {
		out := AskOutput{Type: "text", Dismissed: true, Answer: "(no answer: the person dismissed the question)"}
		if e.DecisionNote != "" {
			out.Answer = "(no answer: the person dismissed the question) " + e.DecisionNote
		}
		return out, true
	}
	out := AskOutput{Type: "text", Answer: e.Status}
	if e.DecisionNote != "" {
		out.Answer = e.DecisionNote
	}
	return out, true
}
