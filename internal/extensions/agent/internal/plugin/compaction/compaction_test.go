// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// fakeModel returns a canned summary for GenerateContent and records the
// transcript it was asked to summarize. window is the model's reported context
// window — the plugin sizes its trigger/tail off it (LLMProviders.yaml in prod).
type fakeModel struct {
	reply    string
	window   int
	gotMsgs  []llm.MessageContent
	called   int
	errReply error
}

func (f *fakeModel) Info() llm.ModelInfo { return llm.ModelInfo{ContextWindow: f.window} }

func (f *fakeModel) GenerateContent(_ context.Context, msgs []llm.MessageContent, _ ...llm.CallOption) (*llm.ContentResponse, error) {
	f.called++
	f.gotMsgs = msgs
	if f.errReply != nil {
		return nil, f.errReply
	}
	return &llm.ContentResponse{Choices: []*llm.ContentChoice{{Content: f.reply}}}, nil
}

// fakeLoop is a minimal LoopContext for driving onTurnStart.
type fakeLoop struct {
	mu   sync.Mutex
	msgs []api.Message
	meta map[string]any
}

func newFakeLoop(msgs []api.Message) *fakeLoop {
	return &fakeLoop{msgs: msgs, meta: map[string]any{}}
}

func (l *fakeLoop) GetMeta(k string) any { l.mu.Lock(); defer l.mu.Unlock(); return l.meta[k] }
func (l *fakeLoop) SetMeta(k string, v any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.meta[k] = v
}
func (l *fakeLoop) Messages() []api.Message { l.mu.Lock(); defer l.mu.Unlock(); return l.msgs }
func (l *fakeLoop) PushMessage(m api.Message) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, m)
}
func (l *fakeLoop) ReplaceMessages(m []api.Message) { l.mu.Lock(); defer l.mu.Unlock(); l.msgs = m }
func (l *fakeLoop) AbortSignal() <-chan struct{}    { return nil }
func (l *fakeLoop) SetModelOverride(string)         {}
func (l *fakeLoop) SetMaxTokens(int)                {}
func (l *fakeLoop) SetThinkingBudget(int)           {}
func (l *fakeLoop) ToolDefs() []api.ToolDef         { return nil }
func (l *fakeLoop) Turn() int                       { return 0 }
func (l *fakeLoop) TotalUsage() api.Usage           { return api.Usage{} }
func (l *fakeLoop) Mode() string                    { return "" }
func (l *fakeLoop) IsChat() bool                    { return false }
func (l *fakeLoop) RunID() string                   { return "" }
func (l *fakeLoop) InheritedPlugins() []any         { return nil }
func (l *fakeLoop) PermissionProfile() string       { return "" }

var _ api.LoopContext = (*fakeLoop)(nil)

func bigText(n int) string { return strings.Repeat("x", n) }

// TestCompaction_BelowThreshold_NoOp: a small conversation is left untouched.
func TestCompaction_BelowThreshold_NoOp(t *testing.T) {
	fm := &fakeModel{reply: "<summary>S</summary>", window: 200_000}
	p := NewPlugin(fm, Config{}) // defaults: trigger 0.75 × 200K = 150K
	loop := newFakeLoop([]api.Message{
		api.NewTextMessage(api.RoleUser, "hi"),
		api.NewTextMessage(api.RoleAssistant, "hello"),
	})
	if err := p.onTurnStart(loop, 2); err != nil {
		t.Fatal(err)
	}
	if fm.called != 0 {
		t.Errorf("summarizer called %d times, want 0 below threshold", fm.called)
	}
	if len(loop.Messages()) != 2 {
		t.Errorf("messages mutated: got %d, want 2", len(loop.Messages()))
	}
}

// TestCompaction_Triggers_KeepsTail summarizes the prefix and keeps a verbatim
// recent tail. The first new message is the framed summary; the tail follows.
func TestCompaction_Triggers_KeepsTail(t *testing.T) {
	fm := &fakeModel{reply: "<analysis>scratch</analysis><summary>Did the work.</summary>", window: 20_000}
	// window 20K; trigger 0.5×=10K, keep 0.25×=5K. Each message ~4000 chars → ~1000 tokens.
	p := NewPlugin(fm, Config{TriggerRatio: 0.5, KeepRecentRatio: 0.25})

	var msgs []api.Message
	for i := 0; i < 10; i++ {
		msgs = append(msgs, api.NewTextMessage(api.RoleUser, "U"+bigText(4000)))
		msgs = append(msgs, api.NewTextMessage(api.RoleAssistant, "A"+bigText(4000)))
	}
	// 20 messages × ~1000 tokens ≈ 20K > trigger.
	loop := newFakeLoop(msgs)

	if err := p.onTurnStart(loop, 5); err != nil {
		t.Fatal(err)
	}
	if fm.called != 1 {
		t.Fatalf("summarizer called %d times, want 1", fm.called)
	}
	if p.Compactions() != 1 {
		t.Errorf("compactions = %d, want 1", p.Compactions())
	}

	out := loop.Messages()
	if len(out) >= len(msgs) {
		t.Errorf("compaction did not shrink: got %d, want < %d", len(out), len(msgs))
	}
	// First message is the framed summary, as a user message.
	if out[0].Role != api.RoleUser {
		t.Errorf("first message role = %v, want user", out[0].Role)
	}
	first := out[0].Parts[0].(llm.TextContent).Text
	if !strings.Contains(first, "Did the work.") {
		t.Errorf("summary not spliced in; got: %s", first[:min(120, len(first))])
	}
	if strings.Contains(first, "<analysis>") || strings.Contains(first, "scratch") {
		t.Error("analysis scratchpad leaked into the summary message")
	}
	// The post-compaction signal was cleared so the next turn re-measures.
	if got := metaInt(loop.GetMeta(metaLastPromptTokens)); got != 0 {
		t.Errorf("_last_prompt_tokens = %d, want 0 after compaction", got)
	}
}

// TestCompaction_FailOpen: a summarizer error leaves the conversation intact.
func TestCompaction_FailOpen(t *testing.T) {
	fm := &fakeModel{errReply: context.DeadlineExceeded, window: 5_000}
	// window 5K; trigger 0.5×=2.5K, keep 0.1×=500. ~5K of messages → triggers.
	p := NewPlugin(fm, Config{TriggerRatio: 0.5, KeepRecentRatio: 0.1})
	msgs := []api.Message{
		api.NewTextMessage(api.RoleUser, bigText(4000)),
		api.NewTextMessage(api.RoleAssistant, bigText(4000)),
		api.NewTextMessage(api.RoleUser, bigText(4000)),
		api.NewTextMessage(api.RoleAssistant, bigText(4000)),
		api.NewTextMessage(api.RoleUser, bigText(4000)),
	}
	loop := newFakeLoop(msgs)
	if err := p.onTurnStart(loop, 3); err != nil {
		t.Fatalf("fail-open must not error: %v", err)
	}
	if len(loop.Messages()) != len(msgs) {
		t.Errorf("messages mutated on summarizer failure: got %d, want %d", len(loop.Messages()), len(msgs))
	}
	if p.Compactions() != 0 {
		t.Errorf("compactions = %d, want 0 on failure", p.Compactions())
	}
}

// TestTailCut_SnapsPastToolResult: the tail must never begin with a tool-result
// message (its tool_use would be in the summarized prefix → an API 400).
func TestTailCut_SnapsPastToolResult(t *testing.T) {
	toolMsg := api.Message{Role: api.RoleTool, Parts: []llm.ContentPart{
		llm.ToolCallResponse{ToolCallID: "t1", Name: "file-read", Content: bigText(100)},
	}}
	msgs := []api.Message{
		api.NewTextMessage(api.RoleUser, bigText(100)),
		api.NewTextMessage(api.RoleAssistant, bigText(100)), // assistant tool_use (text stand-in)
		toolMsg, // tool result — must NOT be the tail head
		api.NewTextMessage(api.RoleAssistant, bigText(100)),
	}
	// keepTokens tiny → naive cut would land on the tool message; snap forward.
	cut := tailCut(msgs, 1)
	if cut < len(msgs) && msgs[cut].Role == api.RoleTool {
		t.Fatalf("tail starts on a tool-result message (cut=%d)", cut)
	}
}

// TestRenderTranscript_OmitsImages keeps the summary call lean and avoids
// prompt-too-long from embedded media.
func TestRenderTranscript_OmitsImages(t *testing.T) {
	msgs := []api.Message{
		api.NewTextMessage(api.RoleUser, "look at this"),
		{Role: api.RoleUser, Parts: []llm.ContentPart{llm.ImageURLContent{URL: "http://x.test/y.png"}}},
	}
	tx := renderTranscript(msgs)
	if !strings.Contains(tx, "look at this") {
		t.Error("text dropped from transcript")
	}
	if !strings.Contains(tx, "[image omitted]") {
		t.Error("image not marked omitted")
	}
	if strings.Contains(tx, "y.png") {
		t.Error("image URL embedded — should be omitted")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
