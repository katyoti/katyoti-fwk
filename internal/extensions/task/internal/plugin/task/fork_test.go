// SPDX-License-Identifier: Apache-2.0

package task

import (
	"strings"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// TestForkRecursionGuard — verifies the structural safeguard that prevents
// a fork sub-agent from itself forking. The guard works via a sentinel tag
// (`forkBoilerplateTag`) injected by `BuildForkedMessages` into every fork
// child's initial messages. `IsInForkChild` detects the tag in subsequent
// turns; the agent loop's fork-tool handler uses this to refuse re-fork
// attempts.
//
// Without this guard a fork → fork → fork chain could spawn unbounded
// worktrees and concurrent agent runs. CLAUDE.md cites this as critical;
// audit `test-coverage.md` flagged the zero-test gap.
//
// This test exercises the three primitives in isolation: BuildForkedMessages
// injects the tag; IsInForkChild detects it; absent the tag, IsInForkChild
// returns false. A 5-level "what if recursion happened anyway" chain
// confirms the detection is recursion-safe.
func TestForkRecursionGuard(t *testing.T) {
	fm := NewForkManager()

	// 1. A fresh conversation has no fork tag → IsInForkChild is false.
	clean := []agentapi.Message{
		agentapi.NewTextMessage(agentapi.RoleUser, "do the task"),
		agentapi.NewTextMessage(agentapi.RoleAssistant, "ok"),
	}
	if fm.IsInForkChild(clean) {
		t.Error("fresh conversation should not be detected as in-fork-child")
	}

	// 2. BuildForkedMessages injects the sentinel tag.
	forked := fm.BuildForkedMessages(clean, "child instruction")
	if !fm.IsInForkChild(forked) {
		t.Fatal("BuildForkedMessages should produce messages tagged as in-fork-child")
	}

	// 3. The tag is in the LAST user message (the appended directive),
	// not in the carried-over parent messages.
	last := forked[len(forked)-1]
	if last.Role != agentapi.RoleUser {
		t.Errorf("last message role = %v, want user", last.Role)
	}
	lastText := messageText(last)
	if !strings.Contains(lastText, forkBoilerplateTag) {
		t.Errorf("last message missing fork tag: %q", lastText)
	}
	if !strings.Contains(lastText, "child instruction") {
		t.Errorf("last message missing child instruction: %q", lastText)
	}

	// 4. Recursion guard: if the agent (somehow) tries to re-fork, the second
	// `BuildForkedMessages` call still produces a tagged conversation.
	// IsInForkChild fires AT INSPECTION TIME (i.e. before the spawn), so
	// what matters is: any forked conversation is detectable. Five levels
	// deep, the detection still fires.
	for level := 2; level <= 5; level++ {
		forked = fm.BuildForkedMessages(forked, "deeper child")
		if !fm.IsInForkChild(forked) {
			t.Errorf("level %d: recursion guard failed to detect fork tag", level)
		}
	}

	// 5. Defense: BuildForkedMessages replaces tool-result content with the
	// cache-friendly placeholder. Without this, every forked conversation
	// would balloon the prompt cache.
	parentWithToolResult := []agentapi.Message{
		agentapi.NewTextMessage(agentapi.RoleUser, "fetch x"),
		{
			Role: agentapi.RoleAssistant,
			Parts: []llm.ContentPart{
				llm.ToolCallResponse{ToolCallID: "tc1", Name: "fetch", Content: "very long tool result content..."},
			},
		},
	}
	forkedWithToolResult := fm.BuildForkedMessages(parentWithToolResult, "go")
	if len(forkedWithToolResult) < 2 {
		t.Fatalf("forked output too short: %d", len(forkedWithToolResult))
	}
	// The carried-over assistant message should now hold the placeholder, not
	// the original tool result content.
	assistantMsg := forkedWithToolResult[1]
	for _, part := range assistantMsg.Parts {
		tr, ok := part.(llm.ToolCallResponse)
		if !ok {
			continue
		}
		if tr.Content != forkPlaceholder {
			t.Errorf("tool result content not replaced with placeholder; got %q", tr.Content)
		}
	}
}

// TestForkRecursionGuard_TagInAnyPart — IsInForkChild must scan every text
// part of every message; a tag hidden anywhere fires the guard. Regression
// guard against a future refactor that scans only the last message.
func TestForkRecursionGuard_TagInAnyPart(t *testing.T) {
	fm := NewForkManager()

	// Tag hidden in the middle of a long conversation.
	msgs := []agentapi.Message{
		agentapi.NewTextMessage(agentapi.RoleUser, "user input 1"),
		agentapi.NewTextMessage(agentapi.RoleAssistant, "assistant reply 1"),
		agentapi.NewTextMessage(agentapi.RoleUser, forkBoilerplateTag+"\nstart of fork chain"),
		agentapi.NewTextMessage(agentapi.RoleAssistant, "fork's first reply"),
		agentapi.NewTextMessage(agentapi.RoleUser, "user follow-up"),
	}
	if !fm.IsInForkChild(msgs) {
		t.Error("tag in middle message should still fire the guard")
	}
}

// messageText concatenates every TextContent part in a message; small
// helper so the test bodies stay readable.
func messageText(m agentapi.Message) string {
	var b strings.Builder
	for _, p := range m.Parts {
		if tc, ok := p.(llm.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
