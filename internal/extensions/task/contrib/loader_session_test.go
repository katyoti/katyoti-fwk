// SPDX-License-Identifier: Apache-2.0

package contrib

import "testing"

// TestParse_SessionModeExplicit — an explicit `session.mode` block round-trips
// verbatim onto Task.SessionMode (ms-action-system: session.mode is
// authoritative for how a task-declared action surfaces).
func TestParse_SessionModeExplicit(t *testing.T) {
	data := []byte(`---
name: journaled-action
type: agent
kind: object
types: [project]
session:
  mode: session
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.SessionMode != "session" {
		t.Errorf("SessionMode = %q, want %q", got.SessionMode, "session")
	}
}

// TestParse_SessionModeInferredFromChatConfig — a task with no explicit
// `session:` block but an agent `config.mode: chat` infers SessionMode "chat"
// so an existing conversational task auto-routes to a Chat tab as an action.
func TestParse_SessionModeInferredFromChatConfig(t *testing.T) {
	data := []byte(`---
name: chat-action
type: agent
kind: object
types: [project]
config:
  mode: chat
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.SessionMode != "chat" {
		t.Errorf("SessionMode = %q, want %q (inferred from config.mode)", got.SessionMode, "chat")
	}
}

// TestParse_SessionModeEmptyByDefault — a plain task (no session block, no chat
// config) leaves SessionMode empty; the resolver treats that as a headless run.
func TestParse_SessionModeEmptyByDefault(t *testing.T) {
	data := []byte(`---
name: plain-action
type: simulator
kind: object
types: [project]
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.SessionMode != "" {
		t.Errorf("SessionMode = %q, want empty", got.SessionMode)
	}
}
