// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
)

// fakeGate is a programmable HumanGate for testing.
type fakeGate struct {
	textAnswer    string
	selectAnswer  string
	confirmAnswer bool
	err           error
}

func (g *fakeGate) Prompt(_ context.Context, in hgapi.PromptRequest) (hgapi.PromptResponse, error) {
	if g.err != nil {
		return hgapi.PromptResponse{}, g.err
	}
	switch in.Type {
	case "text", "":
		return hgapi.PromptResponse{Answer: g.textAnswer}, nil
	case "select":
		return hgapi.PromptResponse{Answer: g.selectAnswer}, nil
	case "confirm":
		return hgapi.PromptResponse{Confirmed: g.confirmAnswer}, nil
	default:
		return hgapi.PromptResponse{}, errors.New("unsupported type")
	}
}

func (g *fakeGate) PromptPermission(_ context.Context, _ hgapi.PromptPermissionRequest) (hgapi.PromptPermissionResponse, error) {
	return hgapi.PromptPermissionResponse{}, errors.New("not used")
}
func (g *fakeGate) Capabilities() []string { return []string{"text", "select", "confirm"} }

func ctxWithGate(t *testing.T, gate hgapi.HumanGate) context.Context {
	t.Helper()
	cfgSvc := config.NewConfigService(loader.NewMapLoader(nil))
	app := application.New(cfgSvc)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	application.PutService[hgapi.HumanGate](app, "default", gate)
	return application.WithApplication(context.Background(), app)
}

// ── Text prompt ─────────────────────────────────────────────────────────

func TestAskUser_Text(t *testing.T) {
	gate := &fakeGate{textAnswer: "Paris"}
	ctx := ctxWithGate(t, gate)
	out, err := executeAskUser(ctx, AskInput{Question: "Capital of France?", Type: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Answer != "Paris" || out.Type != "text" {
		t.Errorf("got %+v", out)
	}
}

func TestAskUser_TextDefault(t *testing.T) {
	gate := &fakeGate{textAnswer: "hello"}
	ctx := ctxWithGate(t, gate)
	// Type omitted → default to "text".
	out, err := executeAskUser(ctx, AskInput{Question: "hi?"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != "text" {
		t.Errorf("Type = %q, want text (default)", out.Type)
	}
}

// ── Select prompt ───────────────────────────────────────────────────────

// ── Confirm prompt ─────────────────────────────────────────────────────

// ── Error cases ─────────────────────────────────────────────────────────

// TestAskUser_EmptyQuestion_YieldsToUser pins the chat-loop yield
// behavior: an empty Question is the "yield to user; wait
// for next message" idle signal — NOT an error. The HumanGate is still
// invoked (so the loop suspends until the user types something), and
// whatever the user sends back becomes the tool result. The translator
// renders "Waiting for your next message…" as the banner placeholder.
func TestAskUser_EmptyQuestion_YieldsToUser(t *testing.T) {
	gate := &fakeGate{textAnswer: "ok"}
	ctx := ctxWithGate(t, gate)
	out, err := executeAskUser(ctx, AskInput{Question: ""})
	if err != nil {
		t.Fatalf("empty question should yield to user, not error; got %v", err)
	}
	if out.Answer != "ok" {
		t.Errorf("Answer = %q, want %q (passthrough from gate)", out.Answer, "ok")
	}
	if out.Type != "text" {
		t.Errorf("Type = %q, want 'text' (default prompt type)", out.Type)
	}
}

func TestAskUser_UnknownType_Error(t *testing.T) {
	gate := &fakeGate{}
	ctx := ctxWithGate(t, gate)
	_, err := executeAskUser(ctx, AskInput{Question: "q", Type: "radio"})
	if !errors.Is(err, ErrNotAConversation) {
		t.Errorf("err = %v, want a conversation only", err)
	}
}

func TestAskUser_NoGate_Error(t *testing.T) {
	// Bare context with no HumanGate registered.
	_, err := executeAskUser(context.Background(), AskInput{Question: "q"})
	if err == nil || !strings.Contains(err.Error(), "no HumanGate") {
		t.Errorf("err = %v, want 'no HumanGate'", err)
	}
}

func TestAskUser_GateError_Propagates(t *testing.T) {
	gate := &fakeGate{err: errors.New("prompt failed")}
	ctx := ctxWithGate(t, gate)
	_, err := executeAskUser(ctx, AskInput{Question: "q"})
	if err == nil || !strings.Contains(err.Error(), "prompt failed") {
		t.Errorf("err = %v", err)
	}
}

// ── Describer ──────────────────────────────────────────────────────────

func TestDescribeAskUser(t *testing.T) {
	raw, _ := json.Marshal(AskInput{Question: "What?", Type: "select"})
	got := describeAskUser(raw)
	if !strings.Contains(got, "type=select") {
		t.Errorf("got %q", got)
	}
	if !strings.Contains(got, "What?") {
		t.Errorf("got %q, want question included", got)
	}
}

// ── prompt/question alias ────────────────────────────────────────────────

// TestPromptText_AcceptsPromptAlias pins the alias resolution: some
// models emit the prompt under `prompt` rather than `question`
// regardless of the advertised schema. PromptText() must surface
// either so the question never silently becomes empty (which would
// render as the "waiting for input" placeholder).
func TestPromptText_AcceptsPromptAlias(t *testing.T) {
	cases := []struct {
		name string
		in   AskInput
		want string
	}{
		{"question only", AskInput{Question: "Q?"}, "Q?"},
		{"prompt alias only", AskInput{Prompt: "P?"}, "P?"},
		{"question wins over prompt", AskInput{Question: "Q?", Prompt: "P?"}, "Q?"},
		{"both empty", AskInput{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.PromptText(); got != tc.want {
				t.Errorf("PromptText() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestExecuteAskUser_PromptAlias_ReachesGate confirms the alias flows
// end-to-end through the interactive channel: a `prompt`-field input
// reaches the gate as a non-empty question (not the empty-yield path).
func TestExecuteAskUser_PromptAlias_ReachesGate(t *testing.T) {
	gate := &fakeGate{textAnswer: "blue"}
	ctx := ctxWithGate(t, gate)
	out, err := executeAskUser(ctx, AskInput{Prompt: "Favorite color?"})
	if err != nil {
		t.Fatalf("executeAskUser with prompt alias: %v", err)
	}
	if out.Answer != "blue" {
		t.Errorf("Answer = %q, want blue", out.Answer)
	}
}

// TestInteractiveAsk_DetachFallsOverToInbox pins the disconnect
// auto-transition: when the gate reports the user detached mid-
// prompt, the interactive channel re-routes the SAME ask through
// the inbox channel instead of failing. The test context has no
// Inbox/Event/Task services, so the inbox attempt surfaces as
// ErrInboxNotImplemented — which proves the route was taken (a
// plain failure would surface the detach error itself).
func TestInteractiveAsk_DetachFallsOverToInbox(t *testing.T) {
	gate := &fakeGate{err: hgapi.ErrUserDetached}
	ctx := ctxWithGate(t, gate)
	_, err := executeAskUser(ctx, AskInput{Question: "Still there?"})
	if err == nil {
		t.Fatal("expected an error from the inbox stub in a bare test context")
	}
	if !errors.Is(err, ErrInboxNotImplemented) {
		t.Fatalf("err = %v, want the inbox-channel chain (ErrInboxNotImplemented)", err)
	}
	if errors.Is(err, hgapi.ErrUserDetached) {
		t.Fatal("detach sentinel leaked through — the fall-over did not run")
	}
}
