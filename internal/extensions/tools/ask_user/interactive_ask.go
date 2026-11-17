// SPDX-License-Identifier: Apache-2.0

// Package ask_user — interactive_ask.go is the HumanGate-backed
// implementation. Used when UserCommsInput is `direct` (or empty,
// which selectChannel maps to interactive for safety).
//
// The dispatcher in ask_user.go selects this impl by default; the
// silent + inbox impls take over when UserCommsInput points to them.
package ask_user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/humangate/formdef"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// interactiveChannel routes ask-user calls through the registered
// HumanGate service. Stateless — every call resolves the gate from
// ctx so CLI / TUI / GUI overrides are honored.
type interactiveChannel struct{}

func (c *interactiveChannel) Ask(ctx context.Context, in AskInput) (AskOutput, error) {
	promptType := in.Type
	if promptType == "" {
		promptType = "text"
	}

	// Prefer THIS run's gate from ctx (parallel-chat isolation) — the
	// app-global "default" service is shared/overwritten when two runs are
	// live, which misroutes answers. Fall back to the registry when ctx
	// carries none (pipe mode, single-run callers).
	gate := hgapi.HumanGateFromContext(ctx)
	if gate == nil {
		var err error
		gate, err = application.UseService[hgapi.HumanGate](ctx)
		if err != nil || gate == nil {
			return AskOutput{}, fmt.Errorf("ask-user: no HumanGate registered (cannot prompt the user)")
		}
	}

	if promptType == "select" && len(in.Options) == 0 {
		return AskOutput{}, fmt.Errorf("ask-user: 'select' type requires at least one option")
	}
	if promptType == "form" && !hasCapability(gate, "form") {
		return AskOutput{}, fmt.Errorf("ask-user: form prompts are not supported by the current UI")
	}

	req := hgapi.PromptRequest{
		Type:     promptType,
		Question: in.PromptText(),
		Options:  in.Options,
	}

	if promptType == "form" {
		if in.FormDef == "" {
			return AskOutput{}, fmt.Errorf("ask-user: 'form' type requires the 'form_def' field (YAML content)")
		}
		fd, err := formdef.Parse([]byte(in.FormDef))
		if err != nil {
			return AskOutput{}, fmt.Errorf("ask-user: invalid form definition: %w", err)
		}
		converted, err := formdef.Convert(fd)
		if err != nil {
			return AskOutput{}, fmt.Errorf("ask-user: form conversion failed: %w", err)
		}
		req.FormSchema = map[string]any{
			"jsonSchema": converted.JSONSchema,
			"uiSchema":   converted.UISchema,
			"data":       map[string]any{},
			"title":      fd.Screen.Title,
		}
	}

	// A person answers at a live UI — maybe minutes: the run waits without
	// its group slot meanwhile (the task service's suspension manager).
	var resp hgapi.PromptResponse
	var err error
	prompt := func() { resp, err = gate.Prompt(ctx, req) }
	if ts, tsErr := application.UseService[taskapi.TaskService](ctx); tsErr == nil && ts.SuspensionManager() != nil {
		if werr := ts.SuspensionManager().WhileWaiting(ctx, loopIDFromContext(ctx), prompt); werr != nil && err == nil {
			err = werr
		}
	} else {
		prompt()
	}
	if errors.Is(err, hgapi.ErrUserDetached) {
		// The user's UI disconnected while the prompt was pending
		// (past the gate's grace period). Fall over to the inbox
		// channel with the SAME question: the task suspends on an
		// approval-request item instead of failing, and the user
		// finds the question in the Inbox when they return. Future
		// asks try the HumanGate again first — on re-attach they go
		// back to the live prompt automatically.
		slog.Info("ask-user: user detached during prompt — falling over to inbox", "question", in.PromptText())
		return (&inboxChannel{}).Ask(ctx, in)
	}
	if err != nil {
		return AskOutput{}, fmt.Errorf("ask-user: %s prompt failed: %w", promptType, err)
	}

	switch promptType {
	case "confirm":
		answer := "false"
		if resp.Confirmed {
			answer = "true"
		}
		return AskOutput{Answer: answer, Type: "confirm"}, nil
	case "form":
		data, _ := json.Marshal(resp.Data)
		return AskOutput{Answer: string(data), Type: "form"}, nil
	default:
		return AskOutput{Answer: resp.Answer, Type: promptType}, nil
	}
}

// hasCapability checks whether the gate advertises the given prompt
// capability. Kept in this file (vs channel.go) because it's only
// used by the interactive impl.
func hasCapability(gate hgapi.HumanGate, cap string) bool {
	for _, c := range gate.Capabilities() {
		if c == cap {
			return true
		}
	}
	return false
}
