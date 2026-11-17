// SPDX-License-Identifier: Apache-2.0

// Package ledger provides a "shared state" machine kind for the Magentic
// multi-agent pattern. A ledger is a long-running suspended machine that
// holds a JSON state snapshot, accepts merge-patch updates from peer agents
// (via the LedgerUpdateTopic), publishes new snapshots on every change, and
// terminates when a complete event arrives.
//
// Coordination model (RFC 7396 JSON Merge Patch):
//   - Top-level keys overwrite
//   - Nested objects merge recursively
//   - null in the patch deletes a key
//   - Last-writer-wins (events serialize through the executor inbox)
//
// Topics: see internal/extensions/task/api/engine/topics.go
//   - LedgerSnapshotTopic(id): outgoing — emitted on every state change
//   - LedgerUpdateTopic(id): incoming — patches from peer agents
//   - LedgerCompleteTopic(id): incoming — terminates with optional final state
package ledger

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// MachineStrategyDef registers "ledger" in the MachineDomain.
var MachineStrategyDef = strategies.FuncStrategy[engine.MachineFactory](
	"ledger", "Shared mutable state for Magentic multi-agent pattern",
	func() (engine.MachineFactory, error) {
		return &ledgerFactory{}, nil
	},
)

// LedgerConfig is the strategy-specific config decoded from TASK.md `config:` block.
type LedgerConfig struct {
	// Initial seeds the snapshot at startup. Optional — defaults to empty map.
	// Overridden by input.initial when present.
	Initial map[string]any `json:"initial,omitempty" yaml:"initial"`

	// ID is the topic ID this ledger publishes/subscribes on. When set, topics
	// are ledger.<ID>.snapshot/update/complete instead of using the runID.
	// Lets sibling agents address the ledger via a stable, workflow-shared ID
	// (e.g., one passed in via workflow input bindings). Overridden by
	// input.id when present.
	ID string `json:"id,omitempty" yaml:"id"`
}

// ledgerState is the checkpointable state held in engine.State.Data.
type ledgerState struct {
	Snapshot map[string]any `json:"snapshot"`
	Version  int            `json:"version"`
}

// ── Factory ───────────────────────────────────────────────────────────

type ledgerFactory struct{}

func (f *ledgerFactory) Name() string { return "ledger" }

func (f *ledgerFactory) CreateMachine(_ context.Context, request any) (engine.Machine, error) {
	cfg, input := resolveRequest(request)

	// Resolve initial state: input.initial > config.Initial > empty.
	initial := cloneMap(cfg.Initial)
	if input != nil {
		if seed, ok := input["initial"].(map[string]any); ok {
			initial = cloneMap(seed)
		}
	}
	if initial == nil {
		initial = make(map[string]any)
	}

	// Resolve topic ID: input.id > config.ID > "" (use runID).
	topicID := cfg.ID
	if input != nil {
		if id, ok := input["id"].(string); ok && id != "" {
			topicID = id
		}
	}

	return &ledgerMachine{initial: initial, topicID: topicID}, nil
}

func resolveRequest(request any) (LedgerConfig, map[string]any) {
	var cfg LedgerConfig
	var input map[string]any

	if r, ok := request.(*taskapi.TaskExecRequest); ok {
		if configMap, ok := r.Config.(map[string]any); ok {
			data, _ := json.Marshal(configMap)
			_ = json.Unmarshal(data, &cfg)
		}
		input = r.Input
	}
	return cfg, input
}

// ── SchemaProvider ────────────────────────────────────────────────────

func (f *ledgerFactory) DefaultInputSchema() json.RawMessage {
	return json.RawMessage(`{"name":"ledger-input","root":"LedgerInput","types":{"LedgerInput":{"type":"object","fields":{"initial":{"type":"object","description":"Initial snapshot to seed the ledger","catchall":"any","optional":true}}}}}`)
}

func (f *ledgerFactory) DefaultOutputSchema() json.RawMessage {
	return json.RawMessage(`{"name":"ledger-output","root":"LedgerOutput","types":{"LedgerOutput":{"type":"object","description":"Final ledger snapshot at completion","catchall":"any"}}}`)
}

func (f *ledgerFactory) BodyFieldName() string { return "" }

var _ engine.SchemaProvider = (*ledgerFactory)(nil)

// ── Machine ───────────────────────────────────────────────────────────

type ledgerMachine struct {
	initial map[string]any
	topicID string // when set, used in place of state.RunID for topic suffixes
}

func (m *ledgerMachine) Name() string { return "ledger" }

// StateLayout names the layout of the ledgerState it saves: bump it when that
// shape changes (engine.StateLayouter).
func (m *ledgerMachine) StateLayout() string { return "ledger/1" }

// RestartsAfterCrash: a ledger's steps (apply a patch, emit its snapshot) are
// safe to do again — it restarts from its last state (engine.Restartable).
func (m *ledgerMachine) RestartsAfterCrash() bool { return true }

func (m *ledgerMachine) InitialState(_ engine.Event) engine.State {
	return engine.State{
		Name: "running",
		Data: &ledgerState{
			Snapshot: cloneMap(m.initial),
			Version:  0,
		},
	}
}

// topicKey returns the ID used to construct the ledger's topic suffixes.
// Prefers a config-supplied stable ID so workflow siblings can address the
// ledger; falls back to the engine-assigned RunID.
func (m *ledgerMachine) topicKey(state *engine.State) string {
	if m.topicID != "" {
		return m.topicID
	}
	return state.RunID
}

func (m *ledgerMachine) Step(_ context.Context, state *engine.State, event engine.Event) (engine.ActionFunc, error) {
	ls := ensureState(state, m.initial)
	runID := m.topicKey(state)

	switch event.Label {
	case "complete":
		// Terminate: emit completion event, set Output.
		var finalState map[string]any
		var reason string
		if evt, ok := event.Payload.(*engine.LedgerCompleteEvent); ok && evt != nil {
			if evt.FinalState != nil {
				finalState = evt.FinalState
			}
			reason = evt.Reason
		}
		if finalState == nil {
			finalState = ls.Snapshot
		}
		return func(_ context.Context) (*engine.StepResult, error) {
			return &engine.StepResult{
				NextState: "complete",
				Data:      ls,
				Output:    finalState,
				Emit: func() []engine.Event {
					done := &engine.LedgerCompleteEvent{FinalState: finalState, Reason: reason}
					return []engine.Event{
						{Type: engine.LedgerCompleteTopic(runID), Payload: done},
						// Flat mirror for the SessionRecorder (cold ledger view).
						{Type: engine.LedgerCompleteAuditTopic(), Payload: done},
					}
				}(),
			}, nil
		}, nil

	case "update":
		// Apply RFC 7396 merge patch, increment version, emit snapshot, suspend.
		var patch map[string]any
		if evt, ok := event.Payload.(*engine.LedgerUpdateEvent); ok && evt != nil {
			patch = evt.Patch
		}
		return func(_ context.Context) (*engine.StepResult, error) {
			ls.Snapshot = mergePatch(ls.Snapshot, patch)
			ls.Version++
			return &engine.StepResult{
				NextState: "running",
				Data:      ls,
				Emit: func() []engine.Event {
					snap := &engine.LedgerSnapshotEvent{State: cloneMap(ls.Snapshot), Version: ls.Version}
					return []engine.Event{
						{Type: engine.LedgerSnapshotTopic(runID), Payload: snap},
						// Flat mirror for the SessionRecorder (cold ledger view).
						{Type: engine.LedgerSnapshotAuditTopic(), Payload: snap},
					}
				}(),
				Wait: m.makeWaitSpec(runID),
			}, nil
		}, nil

	default:
		// First step (no event) — emit initial snapshot, suspend on update/complete.
		return func(_ context.Context) (*engine.StepResult, error) {
			return &engine.StepResult{
				NextState: "running",
				Data:      ls,
				Emit: func() []engine.Event {
					snap := &engine.LedgerSnapshotEvent{State: cloneMap(ls.Snapshot), Version: ls.Version}
					return []engine.Event{
						{Type: engine.LedgerSnapshotTopic(runID), Payload: snap},
						// Flat mirror for the SessionRecorder (cold ledger view).
						{Type: engine.LedgerSnapshotAuditTopic(), Payload: snap},
					}
				}(),
				Wait: m.makeWaitSpec(runID),
			}, nil
		}, nil
	}
}

// makeWaitSpec returns the WaitSpec the ledger uses to suspend between events.
// Listens on update + complete topics for this run; WaitCustom so each event
// re-enters the loop (multiple updates allowed before complete).
func (m *ledgerMachine) makeWaitSpec(runID string) *engine.WaitSpec {
	return &engine.WaitSpec{
		Conditions: []engine.WaitCondition{
			{Topic: engine.LedgerUpdateTopic(runID), Label: "update"},
			{Topic: engine.LedgerCompleteTopic(runID), Label: "complete"},
		},
		Mode: engine.WaitCustom,
	}
}

// ensureState returns *ledgerState, restoring from a map (post-checkpoint) if needed.
func ensureState(state *engine.State, initial map[string]any) *ledgerState {
	if ls, ok := state.Data.(*ledgerState); ok {
		return ls
	}
	// State.Data may be a map after deserialization — try to extract.
	if m, ok := state.Data.(map[string]any); ok {
		ls := &ledgerState{}
		if snap, ok := m["snapshot"].(map[string]any); ok {
			ls.Snapshot = snap
		}
		if v, ok := m["version"].(float64); ok {
			ls.Version = int(v)
		}
		if v, ok := m["version"].(int); ok {
			ls.Version = v
		}
		if ls.Snapshot == nil {
			ls.Snapshot = cloneMap(initial)
		}
		return ls
	}
	return &ledgerState{Snapshot: cloneMap(initial), Version: 0}
}

// ── JSON Merge Patch (RFC 7396) ───────────────────────────────────────

// mergePatch applies an RFC 7396 JSON Merge Patch to dst.
// Rules:
//   - if patch is not an object: returns the patch as-is
//   - for each key in patch:
//   - if value is null: delete key from dst
//   - if value is object AND dst[key] is object: recursively merge
//   - else: dst[key] = value (overwrite)
//
// The dst map is mutated and returned (callers should clone first if they want immutability).
func mergePatch(dst, patch map[string]any) map[string]any {
	if dst == nil {
		dst = make(map[string]any)
	}
	if patch == nil {
		return dst
	}
	for k, v := range patch {
		if v == nil {
			delete(dst, k)
			continue
		}
		// Recursive merge for nested objects.
		if patchObj, isPatchObj := v.(map[string]any); isPatchObj {
			if dstObj, isDstObj := dst[k].(map[string]any); isDstObj {
				dst[k] = mergePatch(dstObj, patchObj)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

// cloneMap does a shallow JSON-roundtrip clone of a map (handles nested
// maps + slices via json marshalling). Used to avoid mutating caller's data
// when we hand snapshots out via events.
func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst map[string]any
	if err := json.Unmarshal(data, &dst); err != nil {
		return src
	}
	return dst
}

// Compile-time check.
var _ engine.MachineFactory = (*ledgerFactory)(nil)
var _ engine.Machine = (*ledgerMachine)(nil)
