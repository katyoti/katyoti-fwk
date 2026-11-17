// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// PrompterStub implements UserPrompter by sending <uiContainerID>/ui/ask_user
// requests via KTSP. The request is routed by the Session Manager
// to the registered UI container. Blocks until the user responds.
type PrompterStub struct {
	sender        ktsp.Sender
	sessionID     string
	uiContainerID string
}

// NewPrompterStub creates a prompter stub that routes prompts through KTSP.
// uiContainerID is the container name or UUID that owns the "ui" slot
// (e.g. "tui" for the default terminal UI container).
func NewPrompterStub(sender ktsp.Sender, sessionID, uiContainerID string) *PrompterStub {
	return &PrompterStub{
		sender:        sender,
		sessionID:     sessionID,
		uiContainerID: uiContainerID,
	}
}

// PromptText sends a text prompt via <uiContainerID>/ui/ask_user and returns the answer.
func (s *PrompterStub) PromptText(ctx context.Context, question string) (string, error) {
	result, err := s.sender.Call(ctx, s.uiContainerID+"/ui/ask_user", map[string]any{
		"sessionId": s.sessionID,
		"question":  question,
		"type":      "text",
	})
	if err != nil {
		return "", fmt.Errorf("ui/ask_user text: %w", err)
	}
	var resp struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", fmt.Errorf("parse ask_user response: %w", err)
	}
	return resp.Answer, nil
}

// PromptSelect sends a select prompt via <uiContainerID>/ui/ask_user and returns the selection.
func (s *PrompterStub) PromptSelect(ctx context.Context, question string, options []string) (string, error) {
	result, err := s.sender.Call(ctx, s.uiContainerID+"/ui/ask_user", map[string]any{
		"sessionId": s.sessionID,
		"question":  question,
		"type":      "select",
		"options":   options,
	})
	if err != nil {
		return "", fmt.Errorf("ui/ask_user select: %w", err)
	}
	var resp struct {
		Selected string `json:"selected"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", fmt.Errorf("parse ask_user response: %w", err)
	}
	return resp.Selected, nil
}

// PromptConfirm sends a confirm prompt via <uiContainerID>/ui/ask_user and returns the answer.
func (s *PrompterStub) PromptConfirm(ctx context.Context, question string) (bool, error) {
	result, err := s.sender.Call(ctx, s.uiContainerID+"/ui/ask_user", map[string]any{
		"sessionId": s.sessionID,
		"question":  question,
		"type":      "confirm",
	})
	if err != nil {
		return false, fmt.Errorf("ui/ask_user confirm: %w", err)
	}
	var resp struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return false, fmt.Errorf("parse ask_user response: %w", err)
	}
	return resp.Confirmed, nil
}
