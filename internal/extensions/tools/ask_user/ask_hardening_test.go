// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// detachingGate: the person's UI is gone the first time (the ask falls back to
// the inbox); any later prompt answers "live answer".
type detachingGate struct{ prompts int }

func (g *detachingGate) Prompt(context.Context, hgapi.PromptRequest) (hgapi.PromptResponse, error) {
	g.prompts++
	if g.prompts > 1 {
		return hgapi.PromptResponse{Answer: "live answer"}, nil
	}
	return hgapi.PromptResponse{}, hgapi.ErrUserDetached
}
func (g *detachingGate) PromptPermission(context.Context, hgapi.PromptPermissionRequest) (hgapi.PromptPermissionResponse, error) {
	return hgapi.PromptPermissionResponse{}, nil
}
func (g *detachingGate) Capabilities() []string { return []string{"text"} }

func machineLoopCtx(t *testing.T, inbox inboxapi.InboxService) context.Context {
	t.Helper()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	application.PutService[inboxapi.InboxService](app, "default", inbox)
	return agentapi.WithMachineWaits(application.WithApplication(context.Background(), app))
}

// A direct ask whose person left falls back to the inbox; when the inbox's
// answer comes, the ask reads it — it does not prompt the person again.
func TestADirectAskThatFellBackToTheInboxReadsTheInbox(t *testing.T) {
	inbox := &countingInbox{fakeInbox: &fakeInbox{status: inboxapi.StatusPending}}
	gate := &detachingGate{}
	ctx := hgapi.WithHumanGate(agentapi.WithUserCommsInput(machineLoopCtx(t, inbox), "direct"), gate)

	_, err := executeAskUser(ctx, AskInput{Question: "ok?"})
	var aw *agentapi.ErrAwait
	if !errors.As(err, &aw) {
		t.Fatalf("first ask: %v, want it awaiting the inbox", err)
	}
	inbox.fakeInbox.status, inbox.fakeInbox.note = inboxapi.StatusApproved, "yes"
	out, err := executeAskUser(agentapi.WithAwaitResume(ctx, aw.Resume), AskInput{Question: "ok?"})
	if err != nil || out.Answer != "yes" || gate.prompts != 1 {
		t.Errorf("checked again: %+v, %v, prompts %d; want the inbox's answer, the person not prompted again", out, err, gate.prompts)
	}
}

// goneInbox: the request is no longer in the inbox.
type goneInbox struct{ *fakeInbox }

func (goneInbox) Get(context.Context, inboxapi.GetInput) (inboxapi.GetOutput, error) {
	return inboxapi.GetOutput{}, fmt.Errorf("load: %w", inboxapi.ErrGone)
}

// An ask whose request was deleted or moved out of the inbox ends with an
// error the model reads — it does not wait for an answer that cannot come.
func TestAnAskWhoseRequestIsGoneStopsWaiting(t *testing.T) {
	ctx := machineLoopCtx(t, goneInbox{&fakeInbox{}})
	_, err := (&inboxChannel{}).Ask(agentapi.WithAwaitResume(ctx, map[string]any{"request": "req-1"}), AskInput{Question: "ok?"})
	var aw *agentapi.ErrAwait
	if err == nil || errors.As(err, &aw) || !strings.Contains(err.Error(), "req-1") {
		t.Errorf("gone request: %v; want an error naming it, not another wait", err)
	}
}

// failingInbox cannot be read now (the workspace starting).
type failingInbox struct{ *fakeInbox }

func (failingInbox) Get(context.Context, inboxapi.GetInput) (inboxapi.GetOutput, error) {
	return inboxapi.GetOutput{}, errors.New("workspace not ready")
}

// While its request is pending, or unreadable for a while, the ask waits —
// and asks to be checked again every so often, so an answer whose event was
// missed, or a request that disappears, is seen.
func TestAWaitingAskIsCheckedAgainEverySoOften(t *testing.T) {
	ctx := machineLoopCtx(t, &fakeInbox{status: inboxapi.StatusPending})
	_, err := (&inboxChannel{}).Ask(ctx, AskInput{Question: "ok?"})
	var aw *agentapi.ErrAwait
	if !errors.As(err, &aw) || aw.RecheckEvery <= 0 {
		t.Fatalf("filed: %v; want an await checked again periodically", err)
	}
	ctx = machineLoopCtx(t, failingInbox{&fakeInbox{}})
	_, err = (&inboxChannel{}).Ask(agentapi.WithAwaitResume(ctx, map[string]any{"request": "req-1"}), AskInput{Question: "ok?"})
	if !errors.As(err, &aw) || aw.RecheckEvery <= 0 {
		t.Errorf("unreadable for now: %v; want it to keep awaiting, checked again periodically", err)
	}
}
