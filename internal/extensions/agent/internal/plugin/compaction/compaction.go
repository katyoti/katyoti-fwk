// SPDX-License-Identifier: Apache-2.0

// Package compaction keeps a long agent run inside its model's context window
// by summarizing older conversation turns into a single summary message while
// preserving a verbatim recent tail — the same strategy Claude Code uses
// (services/compact). It is wired as an agent plugin: an OnTurnStart hook
// checks the current context size and, once it crosses a threshold, replaces
// the older messages with a model-generated summary via LoopContext.
//
// Why a plugin (not loop-baked): compaction is an optional, composable
// behavior like loop-safety and keepalive. The loop already exposes
// LoopContext.ReplaceMessages for exactly this. Short runs never cross the
// threshold, so the hook is a near-zero-cost no-op for them.
//
// Interaction with prompt caching: after a compaction the conversation prefix
// changes, so the cache resets ONCE — but the new, smaller prefix is then
// re-cached and read at 0.1x for every subsequent turn. Net: bounded absolute
// size + cheap cached reads, which is the pair that actually controls spend on
// a long run.
package compaction

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// metaLastPromptTokens is the loop-meta key the machine stamps after each LLM
// call with that call's full prompt size (input + cache-read + cache-write).
// It is the authoritative current-context-size signal; the plugin also
// estimates from the live messages and triggers on whichever is larger.
const metaLastPromptTokens = "_last_prompt_tokens"

// All sizing is derived from the MODEL'S configured context window
// (LLMProviders.yaml `context_window:`, surfaced as ModelInfo.ContextWindow) —
// never a hardcoded token count or window assumption. The only constants are
// dimensionless policy ratios: when to trigger, and how much tail to keep, as
// fractions of that real window.
const (
	// defaultTriggerRatio: compact once the prompt reaches this fraction of the
	// model's context window, leaving headroom for the system prompt + tools +
	// the summary call itself.
	defaultTriggerRatio = 0.75
	// defaultKeepRecentRatio: size the preserved verbatim tail to roughly this
	// fraction of the window, so recent work is never paraphrased.
	defaultKeepRecentRatio = 0.25
	// defaultSummaryMaxTokens caps the summarizer's OUTPUT (not a window fact).
	defaultSummaryMaxTokens = 8_192
	// minPrefixMessages: never compact unless at least this many messages
	// would be summarized — otherwise the summary call costs more than it saves.
	minPrefixMessages = 4
)

// Config tunes the compaction plugin. Zero values fall back to the defaults.
// The trigger and tail sizes are FRACTIONS of the model's real context window,
// not absolute token counts — so the same config behaves correctly across a
// 128K model and a 1M model.
type Config struct {
	// TriggerRatio is the fraction of the model's context window at or above
	// which the next turn compacts. 0 → defaultTriggerRatio.
	TriggerRatio float64
	// KeepRecentRatio is the fraction of the context window kept verbatim as
	// the recent tail after a compaction. 0 → defaultKeepRecentRatio.
	KeepRecentRatio float64
	// SummaryMaxTokens caps the summarizer's output. 0 → defaultSummaryMaxTokens.
	SummaryMaxTokens int
}

func (c Config) withDefaults() Config {
	if c.TriggerRatio <= 0 {
		c.TriggerRatio = defaultTriggerRatio
	}
	if c.KeepRecentRatio <= 0 {
		c.KeepRecentRatio = defaultKeepRecentRatio
	}
	if c.SummaryMaxTokens <= 0 {
		c.SummaryMaxTokens = defaultSummaryMaxTokens
	}
	return c
}

// Plugin is the auto-compaction guard. One instance per run.
type Plugin struct {
	model llm.Model
	cfg   Config

	mu          sync.Mutex
	compactions int // count, for observability/tests
}

// NewPlugin builds the compaction plugin. model is the run's model, reused for
// the summarization call (no separate profile needed). Returns nil if model is
// nil (the loop simply runs without compaction).
func NewPlugin(model llm.Model, cfg Config) *Plugin {
	if model == nil {
		return nil
	}
	return &Plugin{model: model, cfg: cfg.withDefaults()}
}

func (p *Plugin) Name() string          { return "compaction" }
func (p *Plugin) SystemMessage() string { return "" }
func (p *Plugin) ToolDefs() []api.ToolDef {
	return nil
}
func (p *Plugin) HandleToolCall(context.Context, string, json.RawMessage) (*api.ToolCallResult, error) {
	return nil, fmt.Errorf("compaction: no tools")
}

// Stable keeps the plugin in the stable class — it contributes no per-turn
// system text or tools, so it never perturbs the cached prefix on its own.
func (p *Plugin) Stable() bool { return true }

func (p *Plugin) Hooks() api.PluginHooks {
	return api.PluginHooks{
		OnTurnStart: p.onTurnStart,
	}
}

// Compactions returns how many times this run has compacted (test/observability).
func (p *Plugin) Compactions() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.compactions
}

// onTurnStart fires before each turn's prompt is assembled. If the context has
// grown past the trigger, it summarizes the older messages and swaps in
// [summary] + recent-tail. On any failure it fails OPEN (leaves the
// conversation untouched and lets the turn proceed) — compaction is an
// optimization, never a correctness gate.
func (p *Plugin) onTurnStart(lc api.LoopContext, _ int) error {
	// Size everything off the MODEL'S real context window (from
	// LLMProviders.yaml). No declared window → we can't compute a threshold,
	// so we don't compact (rather than invent a magic number).
	window := p.model.Info().ContextWindow
	if window <= 0 {
		return nil
	}
	trigger := int(p.cfg.TriggerRatio * float64(window))
	keepTokens := int(p.cfg.KeepRecentRatio * float64(window))

	msgs := lc.Messages()

	reported := metaInt(lc.GetMeta(metaLastPromptTokens))
	estimated := estimateMessagesTokens(msgs)
	size := reported
	if estimated > size {
		size = estimated
	}
	if size < trigger {
		return nil
	}

	cut := tailCut(msgs, keepTokens)
	if cut < minPrefixMessages {
		// Not enough older history to summarize meaningfully — the bulk is in
		// the tail we'd keep anyway. Skip (avoids a pointless summary call).
		return nil
	}

	prefix := msgs[:cut]
	tail := msgs[cut:]

	summary, err := p.summarize(lc, prefix)
	if err != nil || strings.TrimSpace(summary) == "" {
		// Fail open: keep the full conversation, proceed uncompacted.
		return nil
	}

	newMsgs := make([]api.Message, 0, len(tail)+1)
	newMsgs = append(newMsgs, api.NewTextMessage(api.RoleUser, continuationMessage(summary)))
	newMsgs = append(newMsgs, tail...)
	lc.ReplaceMessages(newMsgs)

	// Clear the stale size signal so the next turn doesn't re-trigger on the
	// pre-compaction reading (the real post-compaction size lands after the
	// next call completes).
	lc.SetMeta(metaLastPromptTokens, 0)

	p.mu.Lock()
	p.compactions++
	p.mu.Unlock()
	return nil
}

// summarize renders the prefix to a text transcript and asks the model to
// summarize it with TEXT ONLY (no tools). Returns the raw model text.
func (p *Plugin) summarize(lc api.LoopContext, prefix []api.Message) (string, error) {
	transcript := renderTranscript(prefix)
	if strings.TrimSpace(transcript) == "" {
		return "", fmt.Errorf("empty transcript")
	}

	// system = the compact instruction; user = the transcript to summarize.
	// No tools are passed, so the model can only answer with text.
	msgs := []llm.MessageContent{
		api.NewTextMessage(api.RoleSystem, compactPrompt()),
		api.NewTextMessage(api.RoleUser, "Here is the conversation transcript to summarize:\n\n"+transcript),
	}

	// Run on the loop's abort signal so a cancelled run doesn't hang on the
	// summary call.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-lc.AbortSignal():
			cancel()
		case <-ctx.Done():
		}
	}()

	resp, err := p.model.GenerateContent(ctx, msgs, llm.WithMaxTokens(p.cfg.SummaryMaxTokens))
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty summary response")
	}
	return resp.Choices[0].Content, nil
}

var _ api.AgentPlugin = (*Plugin)(nil)

// ── helpers ──────────────────────────────────────────────────────────────

// tailCut returns the index where the preserved verbatim tail begins: it walks
// backward accumulating estimated tokens until keepTokens is reached, then
// snaps the boundary forward past any leading tool-result message so the tail
// never starts with an orphan tool_result (whose tool_use would be in the
// summarized prefix → an API 400). Returns a value in [0, len(msgs)].
func tailCut(msgs []api.Message, keepTokens int) int {
	if len(msgs) == 0 {
		return 0
	}
	acc := 0
	cut := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		acc += estimateMessageTokens(msgs[i])
		cut = i
		if acc >= keepTokens {
			break
		}
	}
	// Snap forward past tool-result messages: a tool result must stay with the
	// assistant tool_use it answers, which lives just before it. If the tail
	// would start on a tool message, move the boundary forward until it starts
	// on a non-tool message.
	for cut < len(msgs) && msgs[cut].Role == api.RoleTool {
		cut++
	}
	return cut
}

// renderTranscript flattens messages into a plain-text transcript for the
// summarizer. Tool calls and results are rendered as labeled lines; binary /
// image parts are noted but not embedded (they bloat the summary call and add
// nothing to a text summary — matching Claude Code's image stripping).
func renderTranscript(msgs []api.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		role := roleLabel(m.Role)
		for _, part := range m.Parts {
			switch v := part.(type) {
			case llm.TextContent:
				if strings.TrimSpace(v.Text) == "" {
					continue
				}
				fmt.Fprintf(&b, "%s: %s\n", role, v.Text)
			case llm.ToolCall:
				name, args := "", ""
				if v.FunctionCall != nil {
					name = v.FunctionCall.Name
					args = v.FunctionCall.Arguments
				}
				fmt.Fprintf(&b, "%s [tool_call] %s(%s)\n", role, name, args)
			case llm.ToolCallResponse:
				fmt.Fprintf(&b, "[tool_result %s] %s\n", v.Name, v.Content)
			case llm.ImageURLContent:
				fmt.Fprintf(&b, "%s [image omitted]\n", role)
			case llm.BinaryContent:
				fmt.Fprintf(&b, "%s [binary %s omitted]\n", role, v.MIMEType)
			}
		}
	}
	return b.String()
}

func roleLabel(r llm.ChatMessageType) string {
	switch r {
	case api.RoleUser:
		return "User"
	case api.RoleAssistant:
		return "Assistant"
	case api.RoleSystem:
		return "System"
	case api.RoleTool:
		return "Tool"
	default:
		return string(r)
	}
}

// estimateMessagesTokens sums the rough token estimate over all messages.
func estimateMessagesTokens(msgs []api.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateMessageTokens(m)
	}
	return total
}

// estimateMessageTokens approximates a message's token cost at ~4 chars/token,
// plus a small per-part overhead for structural tokens. Deliberately rough —
// it only needs to be monotonic and in the right order of magnitude to drive
// the trigger and tail-budget decisions.
func estimateMessageTokens(m api.Message) int {
	chars := 0
	parts := 0
	for _, part := range m.Parts {
		parts++
		switch v := part.(type) {
		case llm.TextContent:
			chars += len(v.Text)
		case llm.ToolCall:
			if v.FunctionCall != nil {
				chars += len(v.FunctionCall.Name) + len(v.FunctionCall.Arguments)
			}
		case llm.ToolCallResponse:
			chars += len(v.Name) + len(v.Content)
		case llm.ImageURLContent:
			chars += len(v.URL)
		case llm.BinaryContent:
			chars += len(v.Data) / 3 // base64 inflation, then /4 → ~/3 of raw
		}
	}
	return chars/4 + parts*4
}

// metaInt coerces a loop-meta value to int (stored as int by the machine).
func metaInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
