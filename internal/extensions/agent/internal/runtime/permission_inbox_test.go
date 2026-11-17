// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// fakeInbox files requests in memory; entry is what Get answers, getErr the
// error it fails with.
type fakeInbox struct {
	filed   []inboxapi.FileInput
	fileErr error
	entry   inboxapi.Entry
	getErr  error
}

func (f *fakeInbox) File(_ context.Context, in inboxapi.FileInput) (inboxapi.FileOutput, error) {
	if f.fileErr != nil {
		return inboxapi.FileOutput{}, f.fileErr
	}
	f.filed = append(f.filed, in)
	id := fmt.Sprintf("sys-inbox.AR-%03d", len(f.filed))
	f.entry = inboxapi.Entry{ID: id, Title: in.Title, Status: inboxapi.StatusPending, Expects: in.Expects}
	return inboxapi.FileOutput{Request: f.entry}, nil
}
func (f *fakeInbox) Get(_ context.Context, in inboxapi.GetInput) (inboxapi.GetOutput, error) {
	if f.getErr != nil {
		return inboxapi.GetOutput{}, f.getErr
	}
	if in.ID != f.entry.ID {
		return inboxapi.GetOutput{}, fmt.Errorf("unknown request %s", in.ID)
	}
	return inboxapi.GetOutput{Entry: f.entry}, nil
}
func (f *fakeInbox) Answer(context.Context, inboxapi.AnswerInput) (inboxapi.AnswerOutput, error) {
	return inboxapi.AnswerOutput{}, errors.New("not used")
}
func (f *fakeInbox) Triage(context.Context, inboxapi.TriageInput) (inboxapi.TriageOutput, error) {
	return inboxapi.TriageOutput{}, errors.New("not used")
}
func (f *fakeInbox) Dismiss(context.Context, inboxapi.DismissInput) (inboxapi.DismissOutput, error) {
	return inboxapi.DismissOutput{}, errors.New("not used")
}
func (f *fakeInbox) Ensure(context.Context, inboxapi.EnsureInput) (inboxapi.EnsureOutput, error) {
	return inboxapi.EnsureOutput{}, errors.New("not used")
}
func (f *fakeInbox) Relocate(context.Context, inboxapi.RelocateInput) (inboxapi.RelocateOutput, error) {
	return inboxapi.RelocateOutput{}, errors.New("not used")
}

// inboxRunCtx is an inbox-channel run's dispatch context: an application with
// the HumanGate and the inbox (each nil-able), the loop waiting as a machine.
func inboxRunCtx(t *testing.T, gate hgapi.HumanGate, inbox inboxapi.InboxService) context.Context {
	t.Helper()
	return agentapi.WithMachineWaits(inboxAppCtx(t, gate, inbox))
}

// inboxAppCtx is inboxRunCtx without the machine wait.
func inboxAppCtx(t *testing.T, gate hgapi.HumanGate, inbox inboxapi.InboxService) context.Context {
	t.Helper()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	if gate != nil {
		application.PutService[hgapi.HumanGate](app, "default", gate)
	}
	if inbox != nil {
		application.PutService[inboxapi.InboxService](app, "default", inbox)
	}
	return agentapi.WithUserCommsInput(application.WithApplication(context.Background(), app), "inbox")
}

// resumeSeeingTool records whether it was called with an await resume.
type resumeSeeingTool struct {
	*permissionTestTool
	sawResume bool
}

func (t *resumeSeeingTool) ToolCall(ctx context.Context, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	if _, ok := agentapi.AwaitResumeFrom(ctx); ok {
		t.sawResume = true
	}
	return t.permissionTestTool.ToolCall(ctx, input)
}
