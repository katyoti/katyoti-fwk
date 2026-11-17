// SPDX-License-Identifier: Apache-2.0

// Package session_read implements the session-read agent tool: progressive,
// TURN-addressed navigation of a completed run's model-call journal, BY SESSION
// ID — so an evaluator gets an overview of a (possibly huge) run and zooms to
// full per-turn detail without ever constructing a sessions-dir path or
// shelling out. The session id comes from the task surface (AgentResult /
// task-status); resolving it to journals is this tool's job.
//
// Three zoom levels, picked by params:
//   - no range            → OVERVIEW: the run map (histogram + per-turn index).
//   - turns: "41" / "40-60" → ZOOM: structured per-turn detail (tool calls + inputs).
//   - tool: / grep:       → FILTER: the index entries that match (then zoom).
//   - raw: true (+ turns) → the raw jsonl events for that turn span (escape hatch).
package session_read

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// Input drives the zoom level. A turn is the addressing unit (one model
// request→response cycle); line numbers are not exposed (raw: true is the
// escape hatch to the exact bytes for a turn span).
type Input struct {
	SessionID string `json:"session_id" jsonschema:"The id of a completed run (from AgentResult / task-status). Not a file path."`
	Turns     string `json:"turns,omitempty" jsonschema:"Zoom: a turn index ('41') or range ('40-60') for full per-turn detail. Empty = overview."`
	Tool      string `json:"tool,omitempty" jsonschema:"Filter: only turns that invoked this tool."`
	Grep      string `json:"grep,omitempty" jsonschema:"Filter: only turns whose assistant text or tool inputs contain this substring."`
	Raw       bool   `json:"raw,omitempty" jsonschema:"With turns: return the raw, line-numbered jsonl events for that turn span (exact bytes) instead of the structured view."`
}

// Toolset is the deterministic over-provisioning view: the tools the model was
// GIVEN (turn-0 catalog) cross-referenced with the tools it actually USED. Pure
// facts — no notion of "privileged"/"needed" (that policy is the eval's job).
type Toolset struct {
	Given     int      `json:"given" jsonschema:"Tools in the turn-0 catalog the model saw."`
	Used      int      `json:"used" jsonschema:"Distinct catalog tools actually invoked."`
	NeverUsed []string `json:"never_used,omitempty" jsonschema:"Catalog tools never invoked — over-provisioning. Many never_used vs few used = a bloated capability surface."`
}

// ToolCall is one tool invocation in a turn.
type ToolCall struct {
	Name  string `json:"name" jsonschema:"Tool name."`
	Input string `json:"input,omitempty" jsonschema:"The input JSON the agent passed (truncated in detail; full via raw)."`
}

// TurnBrief is one line of the overview/filter map.
type TurnBrief struct {
	Turn       int      `json:"turn"`
	DurationMs int64    `json:"duration_ms,omitempty"`
	Tools      []string `json:"tools,omitempty" jsonschema:"Tools invoked this turn, IN ORDER (repeats kept)."`
	Summary    string   `json:"summary,omitempty"`
}

// TurnDetail is the zoomed, structured view of one turn.
type TurnDetail struct {
	Turn         int        `json:"turn"`
	DurationMs   int64      `json:"duration_ms,omitempty"`
	SystemPrompt string     `json:"system_prompt,omitempty" jsonschema:"The cold-prompt system text (turn 0 only)."`
	Tools        []string   `json:"tools,omitempty" jsonschema:"The cold-prompt tool catalog (turn 0 only)."`
	Text         string     `json:"text,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
}

// Output carries the run summary plus whichever view the params selected.
type Output struct {
	Turns           int            `json:"turns" jsonschema:"Total turns (model calls) in the run."`
	Model           string         `json:"model,omitempty"`
	TotalDurationMs int64          `json:"total_duration_ms,omitempty"`
	SystemPromptLen int            `json:"system_prompt_chars,omitempty" jsonschema:"Length of the cold system prompt (zoom turn 0 for the text)."`
	Histogram       map[string]int `json:"histogram,omitempty" jsonschema:"tool name -> invocation count (the redundancy view)."`
	ToolCallTotal   int            `json:"tool_call_total,omitempty" jsonschema:"Total tool invocations (not deduped)."`
	ToolsDistinct   int            `json:"tools_distinct,omitempty" jsonschema:"Distinct tools invoked. total ≫ distinct = redundancy."`
	Toolset         *Toolset       `json:"toolset,omitempty" jsonschema:"Catalog-given vs actually-used — the over-provisioning view (overview only)."`

	Index  []TurnBrief  `json:"index,omitempty" jsonschema:"Overview/filter: one compact line per turn — the map to zoom from."`
	Detail []TurnDetail `json:"detail,omitempty" jsonschema:"Zoom: structured per-turn detail."`
	Raw    string       `json:"raw,omitempty" jsonschema:"raw: true — the line-numbered jsonl events for the turn span."`
}

const toolDescription = `Navigate a completed run's journal by SESSION ID, turn-addressed + progressive — for evaluating long runs without drowning in context. Default (no range) returns an OVERVIEW: the tool histogram (redundancy), totals, and a one-line-per-turn index. Zoom with turns:"41" or "40-60" for structured per-turn detail (tool calls + inputs); filter with tool:/grep: to find the turns to zoom; raw:true (+turns) dumps the exact jsonl. Pass only the session id — this tool resolves the journal; never construct a path or shell out.`

const (
	maxSummary     = 140
	maxDetailInput = 2000
)

// NewSessionReadTool builds the read-only session-read tool.
func NewSessionReadTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"session-read",
		toolDescription,
		executeSessionRead,
		toolsapi.WithReadOnly(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📄", Label: "Read run", Color: "#82AAFF", Template: "{{.session_id}}"}),
	)
}

func executeSessionRead(_ context.Context, in Input) (Output, error) {
	id := strings.TrimSpace(in.SessionID)
	if id == "" {
		return Output{}, fmt.Errorf("session_id is required")
	}
	dir, err := sessionDir(id)
	if err != nil {
		return Output{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "llm-model-calls.jsonl"))
	if err != nil {
		return Output{}, fmt.Errorf("read run journal for session %q: %w", id, err)
	}
	lines := splitLines(string(data))
	r := parseRun(lines)
	if len(r.turns) == 0 {
		return Output{}, fmt.Errorf("no model calls recorded in session %q", id)
	}

	switch {
	case in.Raw && strings.TrimSpace(in.Turns) != "":
		return rawMode(lines, r, in.Turns)
	case strings.TrimSpace(in.Turns) != "":
		return detailMode(r, in.Turns)
	case in.Tool != "" || in.Grep != "":
		return filterMode(r, in.Tool, in.Grep), nil
	default:
		return overviewMode(r), nil
	}
}

// --- run model -----------------------------------------------------------

type turn struct {
	index      int
	callID     string
	durationMs int64
	text       string
	toolCalls  []ToolCall
}

type run struct {
	model        string
	systemPrompt string
	tools        []string
	turns        []turn
}

func parseRun(lines []string) run {
	var r run
	turnOf := map[string]int{}
	for _, ln := range lines {
		var evt callEvent
		if json.Unmarshal([]byte(ln), &evt) != nil {
			continue
		}
		switch evt.Kind {
		case "call_start":
			idx := len(r.turns)
			turnOf[evt.CallID] = idx
			r.turns = append(r.turns, turn{index: idx, callID: evt.CallID})
			if evt.Model != "" && r.model == "" {
				r.model = evt.Model
			}
			if idx == 0 {
				req := decodeReqBody(evt.RequestBody)
				r.systemPrompt = extractSystem(req)
				r.tools = extractToolNames(req["tools"])
			}
		case "call_end":
			idx, ok := turnOf[evt.CallID]
			if !ok {
				continue
			}
			t := &r.turns[idx]
			t.durationMs = evt.DurationMs
			for _, c := range callEndContents(evt) {
				switch c.Type {
				case "tool_use":
					if c.Name != "" {
						t.toolCalls = append(t.toolCalls, ToolCall{Name: c.Name, Input: string(c.Input)})
					}
				case "text":
					if c.Text != "" && t.text == "" {
						t.text = c.Text
					}
				}
			}
		}
	}
	return r
}

// --- modes ---------------------------------------------------------------

func overviewMode(r run) Output {
	out := baseSummary(r)
	out.Histogram = map[string]int{}
	seen := map[string]bool{}
	for i := range r.turns {
		t := &r.turns[i]
		for _, tc := range t.toolCalls {
			out.ToolCallTotal++
			out.Histogram[tc.Name]++
			if !seen[tc.Name] {
				seen[tc.Name] = true
				out.ToolsDistinct++
			}
		}
		out.Index = append(out.Index, brief(t))
	}
	out.Toolset = toolsetOf(r, out.Histogram)
	return out
}

// toolsetOf cross-references the turn-0 catalog against the tools actually used.
// `used` counts only catalog tools that were invoked; a tool invoked but absent
// from the catalog is a fallback defect the eval reads off histogram-vs-tools,
// not an over-provisioning fact, so it is excluded here.
func toolsetOf(r run, histogram map[string]int) *Toolset {
	if len(r.tools) == 0 {
		return nil
	}
	ts := &Toolset{Given: len(r.tools)}
	for _, name := range r.tools {
		if histogram[name] > 0 {
			ts.Used++
		} else {
			ts.NeverUsed = append(ts.NeverUsed, name)
		}
	}
	return ts
}

func filterMode(r run, tool, grep string) Output {
	out := baseSummary(r)
	tool = strings.TrimSpace(tool)
	grep = strings.TrimSpace(grep)
	for i := range r.turns {
		t := &r.turns[i]
		if turnMatches(t, tool, grep) {
			out.Index = append(out.Index, brief(t))
		}
	}
	return out
}

func detailMode(r run, spec string) (Output, error) {
	lo, hi, err := parseRange(spec, len(r.turns))
	if err != nil {
		return Output{}, err
	}
	out := baseSummary(r)
	for i := lo; i <= hi; i++ {
		t := &r.turns[i]
		d := TurnDetail{Turn: t.index, DurationMs: t.durationMs, Text: t.text}
		for _, tc := range t.toolCalls {
			d.ToolCalls = append(d.ToolCalls, ToolCall{Name: tc.Name, Input: truncate(tc.Input, maxDetailInput)})
		}
		if t.index == 0 {
			d.SystemPrompt = r.systemPrompt
			d.Tools = r.tools
		}
		out.Detail = append(out.Detail, d)
	}
	return out, nil
}

func rawMode(lines []string, r run, spec string) (Output, error) {
	lo, hi, err := parseRange(spec, len(r.turns))
	if err != nil {
		return Output{}, err
	}
	want := map[string]bool{}
	for i := lo; i <= hi; i++ {
		want[r.turns[i].callID] = true
	}
	out := baseSummary(r)
	var b strings.Builder
	for n, ln := range lines {
		var e struct {
			CallID string `json:"call_id"`
		}
		if json.Unmarshal([]byte(ln), &e) == nil && want[e.CallID] {
			fmt.Fprintf(&b, "%d\t%s\n", n+1, ln)
		}
	}
	out.Raw = b.String()
	return out, nil
}

// --- helpers -------------------------------------------------------------

func baseSummary(r run) Output {
	out := Output{Turns: len(r.turns), Model: r.model, SystemPromptLen: len(r.systemPrompt)}
	for i := range r.turns {
		out.TotalDurationMs += r.turns[i].durationMs
	}
	return out
}

func brief(t *turn) TurnBrief {
	b := TurnBrief{Turn: t.index, DurationMs: t.durationMs}
	for _, tc := range t.toolCalls {
		b.Tools = append(b.Tools, tc.Name)
	}
	if s := truncate(t.text, maxSummary); s != "" {
		b.Summary = s
	} else if len(b.Tools) > 0 {
		b.Summary = "→ " + strings.Join(b.Tools, ", ")
	}
	return b
}

func turnMatches(t *turn, tool, grep string) bool {
	if tool != "" {
		hit := false
		for _, tc := range t.toolCalls {
			if tc.Name == tool {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if grep != "" {
		if strings.Contains(t.text, grep) {
			return true
		}
		for _, tc := range t.toolCalls {
			if strings.Contains(tc.Input, grep) || strings.Contains(tc.Name, grep) {
				return true
			}
		}
		return false
	}
	return true
}

// parseRange parses "41" or "40-60" into clamped [lo,hi] turn indices.
func parseRange(spec string, n int) (int, int, error) {
	spec = strings.TrimSpace(spec)
	lo, hi := 0, 0
	if i := strings.IndexByte(spec, '-'); i >= 0 {
		a, err1 := strconv.Atoi(strings.TrimSpace(spec[:i]))
		b, err2 := strconv.Atoi(strings.TrimSpace(spec[i+1:]))
		if err1 != nil || err2 != nil {
			return 0, 0, fmt.Errorf("invalid turn range %q (want 'a-b')", spec)
		}
		lo, hi = a, b
	} else {
		v, err := strconv.Atoi(spec)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid turn %q", spec)
		}
		lo, hi = v, v
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= n {
		hi = n - 1
	}
	if lo >= n {
		return 0, 0, fmt.Errorf("turn %d out of range (run has %d turns)", lo, n)
	}
	return lo, hi, nil
}

func splitLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := raw[:0]
	for _, ln := range raw {
		if strings.TrimSpace(ln) != "" {
			out = append(out, ln)
		}
	}
	return out
}

// sessionDir resolves a session id to its journal directory. Session dirs are
// named `<id>_<task>`, so an exact match is tried first, then a prefix scan.
func sessionDir(id string) (string, error) {
	base := application.Env().SessionsDir()
	if st, err := os.Stat(filepath.Join(base, id)); err == nil && st.IsDir() {
		return filepath.Join(base, id), nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("list sessions dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), id) {
			return filepath.Join(base, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no session directory for id %q under %s", id, base)
}

// callEvent is the slice of llm.ModelCallEvent this tool reads.
type callEvent struct {
	Kind         string          `json:"kind"`
	CallID       string          `json:"call_id"`
	Model        string          `json:"model"`
	RequestBody  json.RawMessage `json:"request_body"`
	DurationMs   int64           `json:"duration_ms"`
	SSESummary   *sseSummary     `json:"sse_summary"`
	ResponseBody json.RawMessage `json:"response_body"`
}

type sseSummary struct {
	Contents []sseContent `json:"contents"`
}

type sseContent struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	Text  string          `json:"text"`
}

// callEndContents returns the assistant's response content blocks for one turn,
// from the streamed SSE summary or (non-streamed) the raw response body.
func callEndContents(evt callEvent) []sseContent {
	if evt.SSESummary != nil && len(evt.SSESummary.Contents) > 0 {
		return evt.SSESummary.Contents
	}
	if len(evt.ResponseBody) > 0 {
		var rb struct {
			Content []sseContent `json:"content"` // Anthropic non-streamed shape
		}
		if json.Unmarshal(evt.ResponseBody, &rb) == nil {
			return rb.Content
		}
	}
	return nil
}

// decodeReqBody parses the request_body, which is sometimes a JSON object and
// sometimes a JSON string carrying JSON (provider-dependent).
func decodeReqBody(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	b := []byte(raw)
	if b[0] == '"' {
		var s string
		if json.Unmarshal(b, &s) != nil {
			return nil
		}
		b = []byte(s)
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// extractSystem pulls the system prompt: a `system` string, a `system` block
// list ([{type,text}]), or a system-role message (OpenAI shape).
func extractSystem(req map[string]any) string {
	if req == nil {
		return ""
	}
	switch s := req["system"].(type) {
	case string:
		return s
	case []any:
		return joinTextBlocks(s)
	}
	if msgs, ok := req["messages"].([]any); ok {
		for _, m := range msgs {
			mm, ok := m.(map[string]any)
			if ok && mm["role"] == "system" {
				return contentToString(mm["content"])
			}
		}
	}
	return ""
}

func contentToString(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		return joinTextBlocks(c)
	}
	return ""
}

func joinTextBlocks(blocks []any) string {
	var b strings.Builder
	for _, blk := range blocks {
		bm, ok := blk.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := bm["text"].(string); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(t)
		}
	}
	return b.String()
}

// extractToolNames reads the tool catalog from the request's `tools` array —
// `name` (Anthropic) or `function.name` (OpenAI).
func extractToolNames(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var names []string
	for _, t := range arr {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if n, ok := tm["name"].(string); ok && n != "" {
			names = append(names, n)
			continue
		}
		if fn, ok := tm["function"].(map[string]any); ok {
			if n, ok := fn["name"].(string); ok && n != "" {
				names = append(names, n)
			}
		}
	}
	return names
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
