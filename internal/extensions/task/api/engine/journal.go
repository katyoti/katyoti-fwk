// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// JournalEntryType identifies the kind of journal entry.
type JournalEntryType string

const (
	JournalSessionStart JournalEntryType = "session_start"
	JournalSessionEnd   JournalEntryType = "session_end"
	JournalLLMRequest   JournalEntryType = "llm_request"
	JournalLLMResponse  JournalEntryType = "llm_response"
	JournalToolCall     JournalEntryType = "tool_call"
	JournalToolResult   JournalEntryType = "tool_result"
	// JournalTaskStarted records a task run's START — the framework-emitted
	// bookend to JournalTaskOutput. Carries the full id set (run / session /
	// parent), task name + type, delegation mode, resolved spawn dimensions,
	// and the validated input. Emitted by TaskService.Run / LocalSpawner.Spawn
	// the instant a run begins, so the spawn graph + per-run start context can
	// be rebuilt from the journal alone — independent of the caller.
	JournalTaskStarted JournalEntryType = "task_started"
	// JournalTaskOutput records the terminal Output map of a completed top-level
	// run. Emitted by TaskService.Run after executor.Run returns with status
	// Completed. Carries the full Output payload (including framework-prefixed
	// keys like __spawn_dimensions when the simulator's RecordContext is on)
	// so post-hoc consumers (L2 test runner, ktw-utils session show) can read
	// the terminal result without holding a reference to the live TaskService.
	JournalTaskOutput JournalEntryType = "task_output"
	// JournalRunLifecycle is a transition of a run (RunLifecycleEvent).
	JournalRunLifecycle JournalEntryType = "run_lifecycle"

	// JournalMailboxCrossTeamDrop records a cross-team mailbox message that
	// was rejected at receive (R5+I1 / security T3). Sender/receiver names +
	// both teams + reason are captured. Message text is intentionally
	// omitted from the audit channel.
	JournalMailboxCrossTeamDrop JournalEntryType = "mailbox_cross_team_drop"

	// JournalChildSpawn records a structured spawn-intent event from the
	// delegate tool (I2). Captures parent + child + mode + override map
	// so a replayed session reconstructs the spawn graph.
	JournalChildSpawn JournalEntryType = "child_spawn"

	// JournalChildPermissionDenied records a permission denial from
	// SpawnPermissionHook or WorkspacePathHook (I3). Captures hook + tool
	// + reason + bounded denied value.
	JournalChildPermissionDenied JournalEntryType = "child_permission_denied"

	// JournalChildStarted / JournalChildDone / JournalChildFailed record the
	// COMPLETE child-lifecycle events (childName + childRunId + parentRunId +
	// mode + kind on start; output on done; error on failed). Unlike
	// JournalChildSpawn (pre-fork intent, no child run id) these carry the run
	// id, so the cold task-tree projection rebuilds the delegation graph keyed
	// by RUN ID — the only key that survives a recursive delegate chain reusing
	// one task-type name across many runs.
	JournalChildStarted JournalEntryType = "child_started"
	JournalChildDone    JournalEntryType = "child_done"
	JournalChildFailed  JournalEntryType = "child_failed"

	// Comms / calls / ledger — the rest of the "complete journal". Mirror the
	// in-process runtime topics so a reopened session reconstructs inter-agent
	// messages (mailbox + group), workflow call observability, and the Magentic
	// shared-state ledger — the same data the live Team tab shows.
	JournalAgentMessage   JournalEntryType = "agent_message"
	JournalCall           JournalEntryType = "call_completed"
	JournalLedgerSnapshot JournalEntryType = "ledger_snapshot"
	JournalLedgerComplete JournalEntryType = "ledger_complete"

	// JournalTeamEvent records one TeamEvent:
	// team created / member joined-left / message / item
	// shared / state changed / completed — with the correlation envelope.
	// The session log alone rebuilds the team-lens narrative from these.
	JournalTeamEvent JournalEntryType = "team_event"
)

// JournalEntry is a single line in the JSONL session log.
// Entries include Seq and Turn for cross-journal correlation.
type JournalEntry struct {
	Type      JournalEntryType `json:"type"`
	Seq       int64            `json:"seq,omitempty"`
	Turn      int              `json:"turn,omitempty"`
	Timestamp time.Time        `json:"ts"`
	Data      any              `json:"data,omitempty"`
}

// Journal writes LLM-perspective session events to a JSONL file.
// Always owned by a SessionRecorder which provides the shared seq counter.
type Journal struct {
	mu       sync.Mutex
	out      rotatingFile
	path     string
	recorder *SessionRecorder
}

// Log writes a typed entry to the journal. Safe for concurrent use.
func (j *Journal) Log(entryType JournalEntryType, data any) {
	j.LogWithTurn(entryType, 0, data)
}

// LogWithTurn writes a typed entry with an explicit turn number.
func (j *Journal) LogWithTurn(entryType JournalEntryType, turn int, data any) {
	// No unlocked "closed?" check: Close nils the file under the mutex, and
	// reading it unlocked races with that — the check below, under the lock,
	// is the one.
	entry := JournalEntry{
		Type:      entryType,
		Seq:       j.recorder.NextSeq(),
		Timestamp: time.Now(),
		Data:      data,
	}

	if turn > 0 {
		entry.Turn = turn
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.out.file == nil {
		return // closed
	}

	line, err := json.Marshal(entry)
	if err != nil {
		slog.Warn("journal: marshal failed", "path", j.path, "type", entryType, "err", err)
		return
	}
	// Scrub secrets at the single marshaled-line chokepoint (nil-safe).
	line = j.recorder.redactor.RedactBytes(line)
	if err := j.out.write(append(line, '\n')); err != nil {
		slog.Warn("journal: write failed", "path", j.path, "type", entryType, "err", err)
	}
}

// Path returns the journal file path (empty if disabled).
func (j *Journal) Path() string {
	return j.path
}

// Close flushes and closes the journal file. The handle is nilled under the
// write mutex so a late Log lands on the closed-journal fast path instead of
// writing to a closed descriptor.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.out.file == nil {
		return nil
	}
	err := j.out.close()
	return err
}
