// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"context"
	"fmt"
	"time"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// LedgerUpdateInput is the input for the ledger-update tool.
type LedgerUpdateInput struct {
	LedgerID string         `json:"ledger_id" jsonschema:"Run ID of the target ledger machine" required:"true"`
	Patch    map[string]any `json:"patch"     jsonschema:"JSON Merge Patch (RFC 7396) to apply: top-level keys overwrite, nested objects merge, null deletes a key" required:"true"`
}

// LedgerUpdateOutput is the output of the ledger-update tool.
type LedgerUpdateOutput struct {
	Status string `json:"status" jsonschema:"'published' on success"`
}

// LedgerSnapshotInput is the input for the ledger-snapshot tool.
type LedgerSnapshotInput struct {
	LedgerID string `json:"ledger_id" jsonschema:"Run ID of the target ledger machine" required:"true"`
}

// LedgerSnapshotOutput is the output of the ledger-snapshot tool.
type LedgerSnapshotOutput struct {
	State   map[string]any `json:"state"   jsonschema:"Latest cached snapshot state, or {} if none yet"`
	Version int            `json:"version" jsonschema:"Snapshot version (monotonic, -1 if no snapshot has been received yet)"`
}

// LedgerWaitUpdateInput is the input for the ledger-wait-update tool.
type LedgerWaitUpdateInput struct {
	LedgerID     string `json:"ledger_id"             jsonschema:"Run ID of the target ledger machine" required:"true"`
	SinceVersion int    `json:"since_version"         jsonschema:"Block until snapshot.version is strictly greater than this. Pass 0 to wait for the very first snapshot, or the version returned by your last ledger-snapshot/ledger-wait-update call to wait for the NEXT change." required:"true"`
	TimeoutMs    int    `json:"timeout_ms,omitempty"  jsonschema:"Max time to block in milliseconds. Default 30000 (30s). On timeout, returns whatever snapshot is currently cached (may be the same one)."`
}

// LedgerWaitUpdateOutput is the output of the ledger-wait-update tool.
type LedgerWaitUpdateOutput struct {
	State   map[string]any `json:"state"             jsonschema:"Snapshot state at the time of return"`
	Version int            `json:"version"           jsonschema:"Snapshot version (use this as since_version on the next call)"`
	Changed bool           `json:"changed"           jsonschema:"true if a newer snapshot arrived (version > since_version), false on timeout"`
}

// LedgerCompleteInput is the input for the ledger-complete tool (manager only).
type LedgerCompleteInput struct {
	LedgerID string `json:"ledger_id" jsonschema:"Run ID of the ledger machine to terminate" required:"true"`
	Reason   string `json:"reason,omitempty"    jsonschema:"Optional human-readable reason for completion"`
}

// LedgerCompleteOutput is the output of the ledger-complete tool.
type LedgerCompleteOutput struct {
	Status string `json:"status" jsonschema:"'completed' on success, 'already-completed' if a previous call (or the safety hook) already fired"`
}

// ledgerTools returns the role-appropriate typed tools for the given plugin
// instance. Both roles get snapshot + update + wait-update (the blocking
// wait is essential for both managers and workers — polling-based
// coordination would trip the agent loop's repeat-tool-call guard).
// Managers additionally get ledger-complete.
func ledgerTools(p *ledgerPlugin) []toolsapi.Tool {
	tools := []toolsapi.Tool{
		newLedgerUpdateTool(p),
		newLedgerSnapshotTool(p),
		newLedgerWaitUpdateTool(p),
	}
	if p.role == roleManager {
		tools = append(tools, newLedgerCompleteTool(p))
	}
	return tools
}

func newLedgerUpdateTool(p *ledgerPlugin) toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"ledger-update",
		"Publish a JSON Merge Patch (RFC 7396) to a ledger. Top-level keys overwrite, nested objects merge recursively, null deletes a key. Fire-and-forget.",
		func(_ context.Context, in LedgerUpdateInput) (LedgerUpdateOutput, error) {
			if in.LedgerID == "" {
				return LedgerUpdateOutput{}, fmt.Errorf("ledger_id is required")
			}
			p.ensureSubscribed(in.LedgerID)
			if err := p.publishUpdate(in.LedgerID, in.Patch); err != nil {
				return LedgerUpdateOutput{}, fmt.Errorf("publish: %w", err)
			}
			return LedgerUpdateOutput{Status: "published"}, nil
		},
		toolsapi.WithMutating(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📒", Label: "Ledger-Update", Color: "#82AAFF", Template: "{{.ledger_id}}"}),
	)
}

func newLedgerSnapshotTool(p *ledgerPlugin) toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"ledger-snapshot",
		"Read the latest snapshot of a ledger. Subscribes to snapshot events on first call. Forces a fresh emit by publishing an empty-patch update so callers always get current state.",
		func(_ context.Context, in LedgerSnapshotInput) (LedgerSnapshotOutput, error) {
			if in.LedgerID == "" {
				return LedgerSnapshotOutput{}, fmt.Errorf("ledger_id is required")
			}
			p.ensureSubscribed(in.LedgerID)
			// Force a fresh emit so even agents that subscribe AFTER the
			// initial snapshot still get current state. The empty patch
			// causes a no-op merge but a version bump + re-emit.
			_ = p.publishUpdate(in.LedgerID, map[string]any{})
			snap := p.waitForSnapshot(in.LedgerID, 500*time.Millisecond)
			if snap == nil {
				return LedgerSnapshotOutput{State: map[string]any{}, Version: -1}, nil
			}
			return LedgerSnapshotOutput{State: snap.State, Version: snap.Version}, nil
		},
		toolsapi.WithMutating(), // not read-only — triggers a re-emit
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📖", Label: "Ledger-Read", Color: "#82AAFF", Template: "{{.ledger_id}}"}),
	)
}

// newLedgerWaitUpdateTool blocks the agent's turn until the ledger's
// snapshot version exceeds `since_version` (or the timeout fires). This is
// the event-driven coordination primitive — agents pass the version they
// last saw, and each subsequent call has unique arguments (since_version
// monotonically increases), so the agent loop's repeat-tool-call circuit
// breaker never fires. Replaces polling-based `ledger-snapshot` loops in
// the Magentic pattern.
//
// Available to both manager and worker roles: managers wait for workers to
// post results; workers wait for the manager to assign tasks.
func newLedgerWaitUpdateTool(p *ledgerPlugin) toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"ledger-wait-update",
		"Block until the ledger has a snapshot newer than `since_version`. Returns the new snapshot. Use this instead of looping `ledger-snapshot` — it's faster, cheaper, and avoids the repeat-tool-call abort guard. Pass 0 to wait for the first snapshot, or the version from your last call to wait for the next change.",
		func(_ context.Context, in LedgerWaitUpdateInput) (LedgerWaitUpdateOutput, error) {
			if in.LedgerID == "" {
				return LedgerWaitUpdateOutput{}, fmt.Errorf("ledger_id is required")
			}
			p.ensureSubscribed(in.LedgerID)
			// Trigger an initial emit so the cache gets populated even if no
			// other agent has updated yet (matches `ledger-snapshot`'s behavior).
			_ = p.publishUpdate(in.LedgerID, map[string]any{})

			timeoutMs := in.TimeoutMs
			if timeoutMs <= 0 {
				timeoutMs = 30_000
			}
			snap, changed := p.waitForVersionAbove(in.LedgerID, in.SinceVersion, time.Duration(timeoutMs)*time.Millisecond)
			if snap == nil {
				return LedgerWaitUpdateOutput{State: map[string]any{}, Version: -1, Changed: false}, nil
			}
			return LedgerWaitUpdateOutput{State: snap.State, Version: snap.Version, Changed: changed}, nil
		},
		toolsapi.WithMutating(), // not read-only — triggers a re-emit on subscribe
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "⏳", Label: "Ledger-Wait", Color: "#82AAFF", Template: "{{.ledger_id}} > v{{.since_version}}"}),
	)
}

// newLedgerCompleteTool publishes LedgerCompleteEvent so the ledger machine
// terminates cleanly and the surrounding workflow can finalize. Manager-only —
// delegates can't tear down shared state. Idempotent: a previous call (or the
// safety hook) makes subsequent calls return "already-completed".
func newLedgerCompleteTool(p *ledgerPlugin) toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"ledger-complete",
		"Terminate a ledger so the surrounding workflow can finalize. Call this as your last action after coordination is done. Idempotent — safe to call once and then have the safety net no-op on agent termination.",
		func(_ context.Context, in LedgerCompleteInput) (LedgerCompleteOutput, error) {
			if in.LedgerID == "" {
				return LedgerCompleteOutput{}, fmt.Errorf("ledger_id is required")
			}
			// Track the ID even on explicit complete so the safety hook
			// short-circuits the second-fire correctly.
			p.ensureSubscribed(in.LedgerID)
			p.mu.Lock()
			already := p.completed[in.LedgerID]
			p.mu.Unlock()
			if already {
				return LedgerCompleteOutput{Status: "already-completed"}, nil
			}
			reason := in.Reason
			if reason == "" {
				reason = "manager-explicit"
			}
			if err := p.publishComplete(in.LedgerID, reason); err != nil {
				return LedgerCompleteOutput{}, fmt.Errorf("publish: %w", err)
			}
			return LedgerCompleteOutput{Status: "completed"}, nil
		},
		toolsapi.WithMutating(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "🛑", Label: "Ledger-Complete", Color: "#82AAFF", Template: "{{.ledger_id}}"}),
	)
}
