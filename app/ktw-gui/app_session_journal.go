// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// SessionJournalToolCall captures one tool invocation in the journal
// (`tool_call` + `tool_result` pair from llm.jsonl). Input/Result are
// the raw payloads as they appeared on the wire so the GUI can decide
// how to render them (table, code block, link to artifact, etc.).
//
// `ID` is the agent's per-call identifier (e.g. `call_vhm8...`); the
// parser uses it to pair the tool_call event with its tool_result.
// `Label` is the friendly label from the tool's `display` block (e.g.
// "Ask" / "Write") — falls back to Name when absent.
type SessionJournalToolCall struct {
	ID     string          `json:"id,omitempty"`
	Name   string          `json:"name"`
	Label  string          `json:"label,omitempty"`
	Icon   string          `json:"icon,omitempty"`
	Input  json.RawMessage `json:"input,omitempty"`
	Result string          `json:"result,omitempty"`
	Error  bool            `json:"error,omitempty"`
}

// SessionJournalTurn is one agent turn — the user message that
// triggered it, the assistant's reply, the tool calls, and the
// per-turn timing/token telemetry.
//
// A single session journal can interleave SEVERAL runs (a task that
// delegates writes the child's events into the same llm.jsonl). Each
// turn therefore carries its originating run: `RunID` (the raw
// emitter) plus `RunLabel` — a short, first-appearance-ordered handle
// (T1, T2, …) so the UI can attribute every turn to a task. Turn
// numbers are per-run, so "T2 · Turn 1" and "T1 · Turn 1" are distinct
// turns that must NOT be merged.
type SessionJournalTurn struct {
	Number     int                      `json:"number"`
	RunID      string                   `json:"runId,omitempty"`
	RunLabel   string                   `json:"runLabel,omitempty"`
	UserText   string                   `json:"userText,omitempty"`
	AssistText string                   `json:"assistText,omitempty"`
	ToolCalls  []SessionJournalToolCall `json:"toolCalls,omitempty"`
	DurationMs int                      `json:"durationMs,omitempty"`
	InputTok   int                      `json:"inputTok,omitempty"`
	OutputTok  int                      `json:"outputTok,omitempty"`
}

// SessionJournalRun is the legend entry for one run that contributed to
// the journal — its short label, raw id, parent, resolved task name, and
// model. Ordered so the ROOT run (no parent in this journal) is T1, then
// its descendants.
type SessionJournalRun struct {
	Label       string `json:"label"`
	RunID       string `json:"runId"`
	ParentRunID string `json:"parentRunId,omitempty"`
	Task        string `json:"task,omitempty"`
	Model       string `json:"model,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

// SessionModelCost is one row of the Cost tab: the aggregated token usage and
// USD cost for a single model across every run in the session that used it.
// Built from the per-call Usage.Cost the agent loop computes at source.
type SessionModelCost struct {
	Model            string  `json:"model"`
	Provider         string  `json:"provider,omitempty"`
	Runs             int     `json:"runs"`
	InputTokens      int     `json:"inputTokens"`
	OutputTokens     int     `json:"outputTokens"`
	CacheReadTokens  int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int     `json:"cacheWriteTokens,omitempty"`
	InputUSD         float64 `json:"inputUsd,omitempty"`
	OutputUSD        float64 `json:"outputUsd,omitempty"`
	CacheReadUSD     float64 `json:"cacheReadUsd,omitempty"`
	CacheWriteUSD    float64 `json:"cacheWriteUsd,omitempty"`
	TotalUSD         float64 `json:"totalUsd"`
}

// SessionJournalConversation is the full parsed view of an
// `llm.jsonl`. Mirrors the shape that `ktw-utils journal` builds, so
// the GUI's journal browser and the CLI's TUI show structurally the
// same content (modulo the rendering substrate).
type SessionJournalConversation struct {
	SessionID     string               `json:"sessionId"`
	Task          string               `json:"task,omitempty"`
	Model         string               `json:"model,omitempty"`
	Provider      string               `json:"provider,omitempty"`
	SystemMessage string               `json:"systemMessage,omitempty"`
	ToolNames     []string             `json:"toolNames,omitempty"`
	Runs          []SessionJournalRun  `json:"runs,omitempty"`
	Models        []SessionModelCost   `json:"models,omitempty"`
	Turns         []SessionJournalTurn `json:"turns,omitempty"`
	TotalTok      int                  `json:"totalTok,omitempty"`
	TotalUSD      float64              `json:"totalUsd,omitempty"`
	DurationMs    int                  `json:"durationMs,omitempty"`
}

// GetSessionJournalResult wraps Conversation + an optional error
// string so the frontend can show a banner instead of swallowing.
type GetSessionJournalResult struct {
	Conversation *SessionJournalConversation `json:"conversation,omitempty"`
	Error        string                      `json:"error,omitempty"`
}

// GetSessionJournal reads the LLM-level journal (`<sessionDir>/llm.jsonl`)
// for a single session and returns the parsed conversation — the same
// data ktw-utils' `journal` command browses, projected for the GUI.
//
// Resolution: reuses `UniverseList` to find the universe's workspace
// root, then `application.EnvOf(root).SessionsDir()` to get the data
// dir's sessions root. Mirrors `GetSession`'s pattern exactly so the
// pair stays consistent.
//
// Wails-bound: window.go.main.App.GetSessionJournal(universeName, sessionId).
func (a *App) GetSessionJournal(universeName, sessionId string) GetSessionJournalResult {
	if universeName == "" || sessionId == "" {
		return GetSessionJournalResult{Error: "universeName and sessionId are required"}
	}

	listResult := a.listUniverses()
	if listResult.Error != "" {
		return GetSessionJournalResult{Error: listResult.Error}
	}
	var workspaceRoot string
	for _, u := range listResult.Universes {
		if u.Name == universeName {
			workspaceRoot = u.HomePath
			break
		}
	}
	if workspaceRoot == "" {
		return GetSessionJournalResult{Error: fmt.Sprintf("universe %q not found in registry", universeName)}
	}

	sessionDir := filepath.Join(application.EnvOf(workspaceRoot).SessionsDir(), sessionId)
	if _, err := os.Stat(sessionDir); err != nil {
		return GetSessionJournalResult{Error: fmt.Sprintf("session %q not found in universe %q", sessionId, universeName)}
	}

	conv, err := parseLLMJournal(filepath.Join(sessionDir, "llm.jsonl"))
	if err != nil {
		return GetSessionJournalResult{Error: fmt.Sprintf("parse llm.jsonl: %v", err)}
	}
	conv.SessionID = sessionId
	return GetSessionJournalResult{Conversation: conv}
}

// parseLLMJournal walks `llm.jsonl` line by line, decoding the
// well-known event types into the projected conversation shape.
//
// Matches the parser in `app/ktw-utils/internal/journal/parse.go` —
// that package lives under `internal/` so it's not importable from
// ktw-gui (Go's internal-package rule), but the file format itself is
// stable. If we extract the parser to a shared package later, this
// file should switch to importing it.
//
// Unknown event types are skipped silently — mid-run journals contain
// events the GUI doesn't render, and we'd rather tolerate them than
// fail the whole fetch.
func parseLLMJournal(path string) (*SessionJournalConversation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conv := &SessionJournalConversation{}
	// Turns are keyed by (emitter, turn): one journal can interleave
	// multiple runs (a delegating task writes its children's events into
	// the same llm.jsonl), and each run numbers its turns from 1. Keying
	// by turn number alone merges different runs' turns into one card.
	turnMap := make(map[string]*SessionJournalTurn)
	turnSeq := make(map[string]int) // composite key → first seq (temporal order)
	rt := newRunTracker()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0 // monotonic append order — drives temporal turn ordering
	for scanner.Scan() {
		lineNo++
		var entry struct {
			Type string          `json:"type"`
			Seq  int             `json:"seq"`
			Turn int             `json:"turn"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		// Every agent event embeds EventBase.emitter (the run id) +, for
		// lifecycle events, a correlation envelope with parentRunId. Resolve to
		// a run accumulator; labels are assigned post-pass (root = T1).
		var meta struct {
			Emitter     string `json:"emitter"`
			RunID       string `json:"run_id"`
			TaskName    string `json:"task_name"`
			Correlation struct {
				RunID       string `json:"runId"`
				ParentRunID string `json:"parentRunId"`
			} `json:"correlation"`
		}
		_ = json.Unmarshal(entry.Data, &meta)

		// Resolve WHICH run this event belongs to. Most events are emitted BY
		// their own run (emitter == the run id). The exception is the framework
		// task lifecycle: task.started / task.output are stamped with the
		// PARENT's emitter (so the parent's recorder captures the child's
		// bookends), but they DESCRIBE the child run (run_id / correlation.runId).
		// Attribute those to the child — otherwise the child's task NAME lands on
		// the parent's run while the child's actual run (carrying the LLM turns)
		// stays nameless and shows up as the bare model id.
		runID := meta.Emitter
		if entry.Type == "task_started" || entry.Type == "task_output" {
			if meta.RunID != "" {
				runID = meta.RunID
			} else if meta.Correlation.RunID != "" {
				runID = meta.Correlation.RunID
			}
		}
		// Global audit events (child_spawn, mailbox_cross_team_drop,
		// child_permission_denied) carry no run id — skip; not run-scoped turns.
		if runID == "" {
			continue
		}
		run := rt.ensure(runID)
		if meta.Correlation.ParentRunID != "" {
			run.parentRunID = meta.Correlation.ParentRunID
		}

		switch entry.Type {
		case "session_start":
			var d struct {
				Model       string `json:"model"`
				Provider    string `json:"provider"`
				Task        string `json:"task"`
				ParentRunID string `json:"parentRunId"`
			}
			_ = json.Unmarshal(entry.Data, &d)
			if run.model == "" {
				run.model = d.Model
			}
			if run.provider == "" {
				run.provider = d.Provider
			}
			if d.Task != "" {
				run.task = d.Task
			}
			if d.ParentRunID != "" {
				run.parentRunID = d.ParentRunID
			}

		case "task_started", "task_output":
			// Framework lifecycle bookends carry the resolved task name (and,
			// for task_started, the dimensions/input). The name is the most
			// reliable run label.
			if meta.TaskName != "" {
				run.task = meta.TaskName
			}
			if entry.Type == "task_output" {
				var d struct {
					Status string `json:"status"`
				}
				_ = json.Unmarshal(entry.Data, &d)
				if d.Status != "" {
					run.status = d.Status
				}
			}

		case "llm_request":
			// Real on-disk shape (per a hello-world session from 2026-04-14):
			//   messages: [
			//     {role: "human", text: "Please start the task."},
			//     {role: "ai",    parts: [{type: "tool_call", tool_call: {...}}]},
			//     {role: "tool",  parts: [{type: "tool_response", tool_response: {...}}]},
			//   ]
			// (Not the `{role: "user", content[].text}` shape ktw-utils' parser
			// assumed.) The user text is at `msg.text` for human messages;
			// ai/tool messages carry structured `parts`.
			var d struct {
				Turn          int      `json:"turn"`
				SystemMessage string   `json:"systemPrompt"`
				ToolNames     []string `json:"toolNames"`
				Messages      []struct {
					Role string `json:"role"`
					Text string `json:"text"`
				} `json:"messages"`
			}
			_ = json.Unmarshal(entry.Data, &d)
			// Capture per-run so the header can take the ROOT run's prompt +
			// tools (resolved post-pass), not whichever run wrote last.
			if d.SystemMessage != "" {
				run.sysMsg = d.SystemMessage
			}
			if len(d.ToolNames) > 0 {
				run.toolNames = d.ToolNames
			}
			t := getOrCreateJournalTurn(turnMap, turnSeq, run, entry.Turn, lineNo)
			// Walk backwards to find the LATEST human-role text message —
			// that's the prompt that triggered this turn. Subsequent turns
			// will see the same original human message until the user adds
			// another input (ask_user response etc., delivered as a tool
			// result, not a human message).
			for i := len(d.Messages) - 1; i >= 0; i-- {
				msg := d.Messages[i]
				if msg.Role == "human" && msg.Text != "" {
					if t.UserText == "" {
						t.UserText = msg.Text
					}
					break
				}
			}

		case "llm_response":
			// Assistant content is `[{Content: "text", FuncCall: {...},
			// ToolCalls: [...], ReasoningContent: "..."}]` — PascalCase
			// per the agent's internal model emitter. The text payload
			// rides in `Content`; tool invocations show up separately as
			// `tool_call` events further down the stream.
			var d struct {
				Turn       int `json:"turn"`
				DurationMs int `json:"durationMs"`
				Content    []struct {
					Content          string `json:"Content"`
					ReasoningContent string `json:"ReasoningContent"`
				} `json:"content"`
				Usage struct {
					InputTokens              int    `json:"input_tokens"`
					OutputTokens             int    `json:"output_tokens"`
					CacheReadInputTokens     int    `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int    `json:"cache_creation_input_tokens"`
					Model                    string `json:"model"`
					Provider                 string `json:"provider"`
					Cost                     *struct {
						InputUSD      float64 `json:"inputUsd"`
						OutputUSD     float64 `json:"outputUsd"`
						CacheReadUSD  float64 `json:"cacheReadUsd"`
						CacheWriteUSD float64 `json:"cacheWriteUsd"`
						TotalUSD      float64 `json:"totalUsd"`
					} `json:"cost"`
				} `json:"usage"`
			}
			_ = json.Unmarshal(entry.Data, &d)
			t := getOrCreateJournalTurn(turnMap, turnSeq, run, entry.Turn, lineNo)
			t.DurationMs = d.DurationMs
			t.InputTok = d.Usage.InputTokens
			t.OutputTok = d.Usage.OutputTokens
			// Accumulate per-run tokens + cost (priced at source on the Usage).
			if d.Usage.Model != "" && run.model == "" {
				run.model = d.Usage.Model
			}
			if d.Usage.Provider != "" && run.provider == "" {
				run.provider = d.Usage.Provider
			}
			run.inTok += d.Usage.InputTokens
			run.outTok += d.Usage.OutputTokens
			run.cacheRead += d.Usage.CacheReadInputTokens
			run.cacheWrite += d.Usage.CacheCreationInputTokens
			if c := d.Usage.Cost; c != nil {
				run.cost.input += c.InputUSD
				run.cost.output += c.OutputUSD
				run.cost.cacheRead += c.CacheReadUSD
				run.cost.cacheWrite += c.CacheWriteUSD
				run.cost.total += c.TotalUSD
			}
			var assist []byte
			for _, block := range d.Content {
				if block.Content != "" {
					if len(assist) > 0 {
						assist = append(assist, '\n')
					}
					assist = append(assist, block.Content...)
				}
			}
			if len(assist) > 0 {
				t.AssistText = string(assist)
			}

		case "tool_call":
			var d struct {
				Tool    string          `json:"tool"`
				ID      string          `json:"id"`
				Input   json.RawMessage `json:"input"`
				Display struct {
					Icon  string `json:"icon"`
					Label string `json:"label"`
				} `json:"display"`
			}
			_ = json.Unmarshal(entry.Data, &d)
			t := getOrCreateJournalTurn(turnMap, turnSeq, run, entry.Turn, lineNo)
			t.ToolCalls = append(t.ToolCalls, SessionJournalToolCall{
				ID:    d.ID,
				Name:  d.Tool,
				Label: d.Display.Label,
				Icon:  d.Display.Icon,
				Input: d.Input,
			})

		case "tool_result":
			// `result` carries successful payload (a JSON string); `error`
			// carries the diagnostic for the tool_error subtype. Match by
			// `id` first (each call has a unique id; falls back to
			// name+order for pre-id sessions).
			var d struct {
				Tool   string `json:"tool"`
				ID     string `json:"id"`
				Result string `json:"result"`
				Error  string `json:"error"`
			}
			_ = json.Unmarshal(entry.Data, &d)
			t := getOrCreateJournalTurn(turnMap, turnSeq, run, entry.Turn, lineNo)
			matched := false
			if d.ID != "" {
				for i := range t.ToolCalls {
					if t.ToolCalls[i].ID == d.ID && t.ToolCalls[i].Result == "" && !t.ToolCalls[i].Error {
						setToolResult(&t.ToolCalls[i], d.Result, d.Error)
						matched = true
						break
					}
				}
			}
			if !matched {
				for i := len(t.ToolCalls) - 1; i >= 0; i-- {
					if t.ToolCalls[i].Name == d.Tool && t.ToolCalls[i].Result == "" && !t.ToolCalls[i].Error {
						setToolResult(&t.ToolCalls[i], d.Result, d.Error)
						break
					}
				}
			}

		case "session_end":
			var d struct {
				DurationMs int `json:"durationMs"`
			}
			_ = json.Unmarshal(entry.Data, &d)
			if d.DurationMs > run.durationMs {
				run.durationMs = d.DurationMs
			}
		}
	}

	// Emit turns in temporal order (file order == append order == real
	// time). Composite (emitter,turn) keys keep different runs' turns
	// distinct so a delegating parent and its child don't collapse.
	keys := make([]string, 0, len(turnMap))
	for k := range turnMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return turnSeq[keys[i]] < turnSeq[keys[j]] })
	for _, k := range keys {
		conv.Turns = append(conv.Turns, *turnMap[k])
	}

	// Assign tree-ordered labels (root = T1), then project the run legend, the
	// per-model cost table, and the session totals from the per-run accumulators.
	ordered := rt.finalize()
	labelByEmitter := make(map[string]string, len(ordered))
	modelAgg := map[string]*SessionModelCost{}
	var modelOrder []string
	for _, r := range ordered {
		labelByEmitter[r.runID] = r.label
		conv.Runs = append(conv.Runs, SessionJournalRun{
			Label: r.label, RunID: r.runID, ParentRunID: r.parentRunID,
			Task: r.task, Model: r.model, Provider: r.provider,
		})
		conv.TotalTok += r.inTok + r.outTok
		conv.TotalUSD += r.cost.total

		mk := r.model
		if mk == "" {
			mk = "(unknown)"
		}
		m, ok := modelAgg[mk]
		if !ok {
			m = &SessionModelCost{Model: mk, Provider: r.provider}
			modelAgg[mk] = m
			modelOrder = append(modelOrder, mk)
		}
		m.Runs++
		m.InputTokens += r.inTok
		m.OutputTokens += r.outTok
		m.CacheReadTokens += r.cacheRead
		m.CacheWriteTokens += r.cacheWrite
		m.InputUSD += r.cost.input
		m.OutputUSD += r.cost.output
		m.CacheReadUSD += r.cost.cacheRead
		m.CacheWriteUSD += r.cost.cacheWrite
		m.TotalUSD += r.cost.total
	}
	for _, k := range modelOrder {
		conv.Models = append(conv.Models, *modelAgg[k])
	}

	// Header identity + system prompt + tools reflect the ROOT run (T1), not
	// whichever run's events happened to land first/last in the file.
	if len(ordered) > 0 {
		root := ordered[0]
		conv.Task = root.task
		conv.Model = root.model
		conv.Provider = root.provider
		conv.SystemMessage = root.sysMsg
		conv.ToolNames = root.toolNames
		conv.DurationMs = root.durationMs
	}

	// Backfill each turn's RunLabel now that tree-ordered labels are known.
	for i := range conv.Turns {
		conv.Turns[i].RunLabel = labelByEmitter[conv.Turns[i].RunID]
	}

	// Orchestration narrative: a workflow/coordinator run produces NO LLM turns
	// of its own — it just spawns children. Its per-run view would otherwise be
	// blank ("No turns for this run"). Synthesize a single turn that lists what
	// it launched + each child's outcome, so the root and team nodes show their
	// delegation instead of nothing.
	hasTurns := make(map[string]bool, len(conv.Turns))
	for _, t := range conv.Turns {
		hasTurns[t.RunID] = true
	}
	childrenByParent := make(map[string][]*runAcc)
	for _, r := range ordered {
		if r.parentRunID != "" {
			childrenByParent[r.parentRunID] = append(childrenByParent[r.parentRunID], r)
		}
	}
	for _, r := range ordered {
		if hasTurns[r.runID] {
			continue
		}
		kids := childrenByParent[r.runID]
		if len(kids) == 0 {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Orchestrated %d task(s):\n", len(kids))
		for _, k := range kids {
			name := k.task
			if name == "" {
				name = shortRun(k.runID)
			}
			if k.status != "" {
				fmt.Fprintf(&b, "• %s — %s\n", name, k.status)
			} else {
				fmt.Fprintf(&b, "• %s\n", name)
			}
		}
		conv.Turns = append(conv.Turns, SessionJournalTurn{
			Number:     1,
			RunID:      r.runID,
			RunLabel:   r.label,
			AssistText: strings.TrimRight(b.String(), "\n"),
		})
	}

	if err := scanner.Err(); err != nil {
		return conv, fmt.Errorf("scan: %w", err)
	}
	return conv, nil
}

// shortRun trims a run id to a readable tail for fallback labels.
func shortRun(runID string) string {
	if len(runID) > 10 {
		return runID[:10] + "…"
	}
	return runID
}

// llmCost is the per-run USD cost accumulator (mirrors llm.Cost / Usage.Cost).
type llmCost struct{ input, output, cacheRead, cacheWrite, total float64 }

// runAcc accumulates everything learned about one run (emitter) during the
// single pass: identity, parent (for tree-ordered labeling), per-run token +
// cost totals (priced at source on each Usage), and the run's system prompt /
// tool list. The T-label is assigned AFTER the pass so the ROOT (no parent
// present in this journal) is T1.
type runAcc struct {
	runID       string
	parentRunID string
	order       int // first-appearance index
	label       string
	task        string
	status      string // terminal status from task_output (completed/failed/…)
	model       string
	provider    string
	sysMsg      string
	toolNames   []string
	durationMs  int
	inTok       int
	outTok      int
	cacheRead   int
	cacheWrite  int
	cost        llmCost
}

// getOrCreateJournalTurn returns the turn for (run, turn number), creating it
// the first time and recording its append order in seqMap for temporal
// ordering. RunLabel is left empty here and filled post-pass once labels are
// known (root = T1).
func getOrCreateJournalTurn(m map[string]*SessionJournalTurn, seqMap map[string]int, run *runAcc, turn, order int) *SessionJournalTurn {
	key := run.runID + "\x00" + strconv.Itoa(turn)
	if t, ok := m[key]; ok {
		return t
	}
	t := &SessionJournalTurn{Number: turn, RunID: run.runID}
	m[key] = t
	seqMap[key] = order
	return t
}

// runTracker collects runs by emitter (first-appearance order) and assigns
// tree-ordered T-labels only at finalize() — so the root run is T1 regardless
// of which run's events happen to land first in the file.
type runTracker struct {
	order []string
	byID  map[string]*runAcc
}

func newRunTracker() *runTracker {
	return &runTracker{byID: make(map[string]*runAcc)}
}

func (rt *runTracker) ensure(emitter string) *runAcc {
	if r, ok := rt.byID[emitter]; ok {
		return r
	}
	r := &runAcc{runID: emitter, order: len(rt.order)}
	rt.byID[emitter] = r
	rt.order = append(rt.order, emitter)
	return r
}

// depthOf returns how many parent hops separate r from a root. A run whose
// parentRunID isn't present in this journal is treated as a root (depth 0).
func (rt *runTracker) depthOf(r *runAcc) int {
	d, seen, cur := 0, map[string]bool{}, r
	for cur != nil && cur.parentRunID != "" && !seen[cur.runID] {
		seen[cur.runID] = true
		p, ok := rt.byID[cur.parentRunID]
		if !ok {
			break
		}
		d++
		cur = p
	}
	return d
}

// finalize sorts runs by (depth, first-appearance) and assigns T-labels, so
// roots come first (T1…) and descendants follow. Returns the ordered runs.
func (rt *runTracker) finalize() []*runAcc {
	ordered := make([]*runAcc, 0, len(rt.order))
	for _, id := range rt.order {
		ordered = append(ordered, rt.byID[id])
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		di, dj := rt.depthOf(ordered[i]), rt.depthOf(ordered[j])
		if di != dj {
			return di < dj
		}
		return ordered[i].order < ordered[j].order
	})
	for i, r := range ordered {
		r.label = fmt.Sprintf("T%d", i+1)
	}
	return ordered
}

// looksLikeError sniffs the tool-result string for the typical
// `{"status":"error", ...}` / `{"error":"..."}` shapes the agent loop
// emits. Cheap heuristic — preferable to JSON-decoding every result
// since the journal can carry megabytes of tool output.
func looksLikeError(s string) bool {
	if s == "" {
		return false
	}
	return strings.Contains(s, `"status":"error"`) || strings.Contains(s, `"error":`)
}

// setToolResult populates Result + Error on a tool-call entry from a
// tool_result event. An `error` field on the wire is a tool_error
// subtype (Error=true with the error string surfaced as Result so the
// UI can render it under the same code block as successful results).
func setToolResult(c *SessionJournalToolCall, result, errMsg string) {
	if errMsg != "" {
		c.Error = true
		c.Result = errMsg
		return
	}
	c.Result = result
	c.Error = looksLikeError(result)
}
