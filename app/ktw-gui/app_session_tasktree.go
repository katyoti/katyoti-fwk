// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// GetSessionTaskTree is the COLD-reconstruction accessor for the two
// session lenses: it parses one session's llm.jsonl and returns
//   - the TASK TREE (what ran, with which input, which output, which mode —
//     from `child_spawn` + `task_output` entries), and
//   - the TEAM NARRATIVE (from `team_event` entries), and
//   - the session's resource summary (model, tokens, duration).
//
// The frontend uses this when opening a non-live session (after restart /
// reconnect); live sessions build the same shapes from team.* events.

// SessionTaskNode is one task run in the session, flat — the tree shape is
// derived from ParentRunID. RunID doubles as the journal drill key.
type SessionTaskNode struct {
	RunID       string `json:"runId"`
	ParentRunID string `json:"parentRunId,omitempty"`
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Mode        string `json:"mode,omitempty"`
	// CommsMode is the parent's DELEGATION INTENT as expressed in the `delegate`
	// (or `spawn-teammate`) tool call's `mode` argument — e.g. `agent_as_human`.
	// It is the comms relationship the parent chose, which the resolved spawn
	// `Mode` (often the default `worker`) does NOT capture. The Team tab renders
	// it on the parent→child LINK; Mode renders in the child node. Empty when the
	// child was not spawned via a recorded delegate/spawn-teammate call.
	CommsMode string         `json:"commsMode,omitempty"`
	Status    string         `json:"status,omitempty"` // completed | failed | cancelled
	Input     map[string]any `json:"input,omitempty"`
	Output    map[string]any `json:"output,omitempty"`
	Timestamp string         `json:"ts,omitempty"`
}

// SessionTeamEntry is one team-lens narrative event.
type SessionTeamEntry struct {
	Action    string         `json:"action"`
	TeamID    string         `json:"teamId,omitempty"`
	TeamName  string         `json:"teamName,omitempty"`
	State     string         `json:"state,omitempty"`
	MemberID  string         `json:"memberId,omitempty"`
	RunID     string         `json:"runId,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
	Timestamp string         `json:"ts,omitempty"`
}

// SessionMessage is one inter-agent message (mailbox or group broadcast)
// reconstructed from a journaled agent_message event.
type SessionMessage struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Team      string `json:"team,omitempty"`
	Text      string `json:"text"`
	Summary   string `json:"summary,omitempty"`
	Timestamp string `json:"ts,omitempty"`
}

// SessionCall is one workflow `call` node attempt (observability) from a
// journaled call_completed event.
type SessionCall struct {
	Node       string `json:"node"`
	URI        string `json:"uri"`
	DurationMs int64  `json:"durationMs"`
	Attempt    int    `json:"attempt"`
	Outcome    string `json:"outcome"`
	Error      string `json:"error,omitempty"`
	RunID      string `json:"runId,omitempty"`
	Timestamp  string `json:"ts,omitempty"`
}

// SessionLedgerEntry is one shared-state ledger event (Magentic pattern):
// a snapshot (full state + version) or the terminal complete.
type SessionLedgerEntry struct {
	Kind      string         `json:"kind"` // "snapshot" | "complete"
	State     map[string]any `json:"state,omitempty"`
	Version   int            `json:"version,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	RunID     string         `json:"runId,omitempty"`
	Timestamp string         `json:"ts,omitempty"`
}

// SessionTaskTree is the cold-reconstruction payload for one session.
type SessionTaskTree struct {
	SessionID   string               `json:"sessionId"`
	Model       string               `json:"model,omitempty"`
	TotalTokens int                  `json:"totalTokens,omitempty"`
	DurationMs  int                  `json:"durationMs,omitempty"`
	Tasks       []SessionTaskNode    `json:"tasks,omitempty"`
	TeamEvents  []SessionTeamEntry   `json:"teamEvents,omitempty"`
	Messages    []SessionMessage     `json:"messages,omitempty"`
	Calls       []SessionCall        `json:"calls,omitempty"`
	Ledger      []SessionLedgerEntry `json:"ledger,omitempty"`
}

// GetSessionTaskTreeResult wraps the tree + an optional error string.
type GetSessionTaskTreeResult struct {
	Tree  *SessionTaskTree `json:"tree,omitempty"`
	Error string           `json:"error,omitempty"`
}

// GetSessionTaskTree parses `<sessionDir>/llm.jsonl` into the task-tree +
// team-narrative payload. Resolution mirrors GetSessionJournal exactly.
//
// Wails-bound: window.go.main.App.GetSessionTaskTree(universeName, sessionId).
func (a *App) GetSessionTaskTree(universeName, sessionId string) GetSessionTaskTreeResult {
	if universeName == "" || sessionId == "" {
		return GetSessionTaskTreeResult{Error: "universeName and sessionId are required"}
	}

	listResult := a.listUniverses()
	if listResult.Error != "" {
		return GetSessionTaskTreeResult{Error: listResult.Error}
	}
	var workspaceRoot string
	for _, u := range listResult.Universes {
		if u.Name == universeName {
			workspaceRoot = u.HomePath
			break
		}
	}
	if workspaceRoot == "" {
		return GetSessionTaskTreeResult{Error: fmt.Sprintf("universe %q not found in registry", universeName)}
	}

	sessionDir := filepath.Join(application.EnvOf(workspaceRoot).SessionsDir(), sessionId)
	if _, err := os.Stat(sessionDir); err != nil {
		return GetSessionTaskTreeResult{Error: fmt.Sprintf("session %q not found in universe %q", sessionId, universeName)}
	}

	tree, err := parseSessionTaskTree(filepath.Join(sessionDir, "llm.jsonl"))
	if err != nil {
		return GetSessionTaskTreeResult{Error: fmt.Sprintf("parse llm.jsonl: %v", err)}
	}
	tree.SessionID = sessionId
	return GetSessionTaskTreeResult{Tree: tree}
}

// parseSessionTaskTree walks llm.jsonl and projects the delegation tree +
// team narrative + resource summary. Unknown entry types are skipped (same
// tolerance policy as parseLLMJournal).
//
// Nodes are keyed by RUN ID — never by name. A recursive delegate chain reuses
// one task-TYPE name ("prompt-worker") across many runs, so name-keying
// collapses every run into a single node and destroys the parent→child edges.
// Identity + parent come from whichever of these the journal carries (all
// run-id keyed):
//   - child_started (preferred): the COMPLETE delegation event — real label +
//     childRunId + parentRunId + mode + kind (new journals).
//   - task_started / task_output: framework bookends — run_id + parentRunId +
//     task-type name + mode + input + terminal output (every run, old + new).
//   - child_done / child_failed: terminal status + output for delegated runs.
//
// `child_spawn` (the pre-fork audit) is intentionally NOT a node source: it has
// no child run id (can't be keyed or edged), includes MCP tool-call + empty
// names, and under fan-out can't be rejoined to a run. It survives only as the
// in-process audit; the tree is built from run-id-bearing events.
func parseSessionTaskTree(path string) (*SessionTaskTree, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tree := &SessionTaskTree{}
	byRun := map[string]*SessionTaskNode{}
	var order []string
	// Delegation INTENT keyed by the target task name. The `delegate` /
	// `spawn-teammate` tool calls live in the PARENT's stream and reference the
	// child by task_name (no child run id yet), so we collect them here and
	// attach to the matching child node (by Name) after the scan. Last write
	// wins — fine for the common one-spawn-per-name case.
	commsByName := map[string]string{}

	node := func(runID string) *SessionTaskNode {
		if n, ok := byRun[runID]; ok {
			return n
		}
		n := &SessionTaskNode{RunID: runID}
		byRun[runID] = n
		order = append(order, runID)
		return n
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 32*1024*1024)
	for scanner.Scan() {
		var entry struct {
			Type string          `json:"type"`
			TS   string          `json:"ts"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "session_start":
			var d struct {
				Model string `json:"model"`
			}
			if json.Unmarshal(entry.Data, &d) == nil && d.Model != "" {
				tree.Model = d.Model
			}

		case "session_end":
			var d struct {
				DurationMs int `json:"durationMs"`
				Usage      struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(entry.Data, &d) == nil {
				tree.DurationMs = d.DurationMs
				tree.TotalTokens = d.Usage.InputTokens + d.Usage.OutputTokens
			}

		case "child_started":
			// The COMPLETE delegation event: real label + run id + parent + mode
			// + kind, all in one place. Authoritative for the human-readable name
			// (a task_started for the same run carries only the generic task-type
			// name) — so the label always wins.
			var d struct {
				RunID       string `json:"runId"`
				ParentRunID string `json:"parentRunId"`
				ChildName   string `json:"childName"`
				ChildKind   string `json:"childKind"`
				Mode        string `json:"mode"`
			}
			if json.Unmarshal(entry.Data, &d) != nil || d.RunID == "" {
				continue
			}
			n := node(d.RunID)
			if d.ChildName != "" {
				n.Name = d.ChildName
			}
			if n.ParentRunID == "" {
				n.ParentRunID = d.ParentRunID
			}
			if d.ChildKind != "" {
				n.Kind = d.ChildKind
			}
			if d.Mode != "" {
				n.Mode = d.Mode
			}
			if n.Timestamp == "" {
				n.Timestamp = entry.TS
			}

		case "child_done":
			var d struct {
				RunID  string         `json:"runId"`
				Output map[string]any `json:"output"`
			}
			if json.Unmarshal(entry.Data, &d) != nil || d.RunID == "" {
				continue
			}
			n := node(d.RunID)
			if n.Status == "" {
				n.Status = "completed"
			}
			if n.Output == nil {
				n.Output = d.Output
			}

		case "child_failed":
			var d struct {
				RunID string `json:"runId"`
			}
			if json.Unmarshal(entry.Data, &d) != nil || d.RunID == "" {
				continue
			}
			node(d.RunID).Status = "failed"

		case "tool_call":
			// Capture the DELEGATION INTENT from `delegate` / `spawn-teammate`
			// tool calls. Their `mode` argument is what the parent ASKED for
			// (e.g. `agent_as_human`) — the comms relationship, which the child's
			// RESOLVED spawn mode (often the default `worker`) does not reflect.
			// Keyed by target task_name; attached to the child node post-scan.
			var d struct {
				Tool  string `json:"tool"`
				Input struct {
					TaskName string `json:"task_name"`
					Mode     string `json:"mode"`
				} `json:"input"`
			}
			if json.Unmarshal(entry.Data, &d) != nil {
				continue
			}
			if d.Input.Mode == "" || d.Input.TaskName == "" {
				continue
			}
			if d.Tool == "delegate" || d.Tool == "spawn-teammate" {
				commsByName[d.Input.TaskName] = d.Input.Mode
			}

		case "task_started":
			// Framework START bookend — run id + parent + task-type name + mode +
			// input, emitted for EVERY run (delegate AND workflow children, plus
			// the root). The reliable run-id-keyed source on old journals that
			// predate child_started. The child_started label (when present) wins
			// for Name; task_started fills name only if none set yet.
			var d struct {
				RunID    string         `json:"run_id"`
				TaskName string         `json:"task_name"`
				TaskType string         `json:"task_type"`
				Mode     string         `json:"mode"`
				Input    map[string]any `json:"input"`
				Corr     struct {
					ParentRunID string `json:"parentRunId"`
				} `json:"correlation"`
			}
			if json.Unmarshal(entry.Data, &d) != nil || d.RunID == "" {
				continue
			}
			n := node(d.RunID)
			if n.Name == "" {
				n.Name = d.TaskName
			}
			if n.ParentRunID == "" {
				n.ParentRunID = d.Corr.ParentRunID
			}
			if n.Kind == "" {
				n.Kind = d.TaskType
			}
			if n.Mode == "" {
				n.Mode = d.Mode
			}
			if n.Input == nil {
				n.Input = d.Input
			}
			if n.Timestamp == "" {
				n.Timestamp = entry.TS
			}

		case "task_output":
			var d struct {
				RunID    string         `json:"run_id"`
				TaskName string         `json:"task_name"`
				Status   string         `json:"status"`
				Output   map[string]any `json:"output"`
				Corr     struct {
					ParentRunID string `json:"parentRunId"`
				} `json:"correlation"`
			}
			if json.Unmarshal(entry.Data, &d) != nil || d.RunID == "" {
				continue
			}
			n := node(d.RunID)
			if n.Name == "" {
				n.Name = d.TaskName
			}
			if d.Status != "" {
				n.Status = d.Status
			}
			if n.Output == nil {
				n.Output = d.Output
			}
			if n.ParentRunID == "" {
				n.ParentRunID = d.Corr.ParentRunID
			}
			n.Timestamp = entry.TS

		case "team_event":
			var d struct {
				Action   string         `json:"action"`
				TeamID   string         `json:"team_id"`
				TeamName string         `json:"team_name"`
				State    string         `json:"state"`
				MemberID string         `json:"member_id"`
				Detail   map[string]any `json:"detail"`
				Corr     struct {
					RunID string `json:"runId"`
				} `json:"correlation"`
			}
			if json.Unmarshal(entry.Data, &d) != nil {
				continue
			}
			tree.TeamEvents = append(tree.TeamEvents, SessionTeamEntry{
				Action:    d.Action,
				TeamID:    d.TeamID,
				TeamName:  d.TeamName,
				State:     d.State,
				MemberID:  d.MemberID,
				RunID:     d.Corr.RunID,
				Detail:    d.Detail,
				Timestamp: entry.TS,
			})

		case "agent_message":
			var d struct {
				From    string `json:"from"`
				To      string `json:"to"`
				Team    string `json:"team"`
				Text    string `json:"text"`
				Summary string `json:"summary"`
			}
			if json.Unmarshal(entry.Data, &d) != nil {
				continue
			}
			tree.Messages = append(tree.Messages, SessionMessage{
				From: d.From, To: d.To, Team: d.Team, Text: d.Text, Summary: d.Summary,
				Timestamp: entry.TS,
			})

		case "call_completed":
			var d struct {
				Node       string `json:"node"`
				URI        string `json:"uri"`
				DurationMs int64  `json:"durationMs"`
				Attempt    int    `json:"attempt"`
				Outcome    string `json:"outcome"`
				Error      string `json:"error"`
				Emitter    string `json:"emitter"`
			}
			if json.Unmarshal(entry.Data, &d) != nil {
				continue
			}
			tree.Calls = append(tree.Calls, SessionCall{
				Node: d.Node, URI: d.URI, DurationMs: d.DurationMs, Attempt: d.Attempt,
				Outcome: d.Outcome, Error: d.Error, RunID: d.Emitter, Timestamp: entry.TS,
			})

		case "ledger_snapshot":
			var d struct {
				State   map[string]any `json:"state"`
				Version int            `json:"version"`
				Emitter string         `json:"emitter"`
			}
			if json.Unmarshal(entry.Data, &d) != nil {
				continue
			}
			tree.Ledger = append(tree.Ledger, SessionLedgerEntry{
				Kind: "snapshot", State: d.State, Version: d.Version, RunID: d.Emitter, Timestamp: entry.TS,
			})

		case "ledger_complete":
			var d struct {
				FinalState map[string]any `json:"finalState"`
				Reason     string         `json:"reason"`
				Emitter    string         `json:"emitter"`
			}
			if json.Unmarshal(entry.Data, &d) != nil {
				continue
			}
			tree.Ledger = append(tree.Ledger, SessionLedgerEntry{
				Kind: "complete", State: d.FinalState, Reason: d.Reason, RunID: d.Emitter, Timestamp: entry.TS,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for _, runID := range order {
		n := byRun[runID]
		// Attach the parent's delegation intent (comms mode) to the child node
		// by name, unless an explicit value already rode an event.
		if n.CommsMode == "" && n.Name != "" {
			if cm, ok := commsByName[n.Name]; ok {
				n.CommsMode = cm
			}
		}
		tree.Tasks = append(tree.Tasks, *n)
	}
	return tree, nil
}
