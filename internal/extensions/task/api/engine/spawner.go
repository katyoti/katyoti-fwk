// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// Spawner starts a machine's children (a workflow's steps). The task
// service's (WithSpawner) starts each as a run of its own — its group's slot,
// saved, under a stable id (ChildRunID) — and a parent settles each from its
// Outcome, waiting on its run's end (RunEndedTopic).
type Spawner interface {
	// Spawn starts a child; with a Site, the same parent and site give the
	// same child — started again, it is found.
	Spawn(ctx context.Context, spec ChildSpec) (ChildHandle, error)

	// Cancel signals cooperative cancellation to a running child.
	// Emits to.<name>.cancel on the bus.
	Cancel(runID string, reason string) error

	// Result returns the final output of a completed child.
	// Returns nil, false if the child hasn't completed or doesn't exist.
	Result(runID string) (output any, ok bool)

	// Outcome is how a child ended — ended false while it has not (or is not
	// known). A parent settles its children from it, never from an event's
	// payload: after a restart, or when the end event was published before it
	// listened, the outcome is still there.
	Outcome(runID string) (outcome ChildOutcome, ended bool)
}

// ChildSpec is the request to spawn a child machine.
type ChildSpec struct {
	// Kind is the machine type name for MachineDomain lookup (e.g., "agent", "workflow").
	Kind string

	// Name is the logical role name used in topic conventions (e.g., "researcher").
	// Used as key in state.results and in event topics (child.<name>.done).
	Name string

	// Config holds strategy-specific parameters (from TASK.md config: block or inline).
	Config map[string]any

	// Input holds caller-provided runtime parameters (validated against input-schema).
	Input map[string]any

	// Body is the markdown content (prompt text, template, etc.).
	Body string

	// ParentID is the parent workflow's run ID, for lineage tracking.
	ParentID string

	// Site names the call site that starts the child (a node, its iteration,
	// its attempt): with ParentID it gives the child's stable id
	// (ChildRunID) — started again from the same site, the same child is
	// found, never started twice. Empty: a fresh id.
	Site string

	// ParentMode is the parent agent's delegation mode (UC-053 / UC-054).
	// The spawner reads this to apply the parent's SpawnPolicies (e.g.,
	// coordinator-restricts-children) to the child's effective config.
	// Both delegate-spawn (taskplugin reads loop meta `_mode`) and
	// workflow-spawn (wkflow.stepSpawn reads workflow Machine's mode)
	// populate it. Empty string = root task (no policies applied).
	ParentMode string

	// Mode is the child's own delegation mode (UC-053). Pairs with
	// ParentMode: spawner calls presets.BuildChildOverride(Mode, ParentMode)
	// to compute the child's ConfigOverride. When empty, no override is
	// computed (preserves backward compat with callers that pre-built
	// the override themselves).
	Mode string

	// ParentSpawnPolicyNames carries the parent's runtime sticky policies
	// (by name). Closes UC-054 — the transitive case where a mid-worker
	// inherits the coordinator's policy via sticky propagation and must
	// re-apply it to its own children. Mode preset alone isn't enough at
	// runtime (a mid-worker resolves as `worker`, which has no policies);
	// the names here let the spawner reconstruct the policies on the fly.
	// Names are resolved via presets.PolicyByName.
	ParentSpawnPolicyNames []string

	// DryRunMode marks this spawn as part of a dry-run. When set, the spawner
	// substitutes LLM/program kinds (agent, task, human-gate, program) with the
	// simulator strategy, returning a deterministic placeholder output. The flag
	// is propagated into nested workflows' ChildSpecs so dry-run is inherited.
	DryRunMode bool

	// CurrentNode is the CWM context node inherited from the parent. Workflow
	// children spawned from a `kind: node` parent task receive the same node
	// unless the spawn site explicitly overrides it. Opaque map (the engine
	// has no CWM dependency); the receiving strategy reads it via
	// TaskExecRequest.Runtime.CurrentNode. nil = no node (parent was global,
	// or no node was supplied at the top of the chain).
	CurrentNode map[string]any

	// MaxDurationS, when > 0, bounds the child's wall-clock runtime: the
	// spawner wraps the child context with a timeout (Dimension 1 scheduling
	// limit at the engine level — the per-delegate counterpart of
	// TaskRunRequest.MaxDurationS). On expiry the child is cancelled and
	// fails with the context error.
	MaxDurationS int
}

// ChildHandle is the state of a spawned child.
type ChildHandle struct {
	RunID  string      `json:"runId"`
	Name   string      `json:"name"`
	Kind   string      `json:"kind"`
	Status ChildStatus `json:"status"`
}

// ChildOutcome is how a child ended: its status (done, failed, cancelled),
// its output when done, its error otherwise.
type ChildOutcome struct {
	Status ChildStatus `json:"status"`
	Output any         `json:"output,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// ChildStatus represents the lifecycle state of a spawned child.
type ChildStatus string

const (
	ChildRunning   ChildStatus = "running"
	ChildDone      ChildStatus = "done"
	ChildFailed    ChildStatus = "failed"
	ChildCancelled ChildStatus = "cancelled"
)

// Child topic constructors now live in `topics.go` — the canonical home for
// all topic-string producers (per `rule-topic-constructors-only` lint rule
// and CLAUDE.md "Topic Conventions"). Definitions kept here would be a
// regression target for the lint; moved 2026-05-17.

// ChildRunID is the stable id of the child a parent run starts at a call site
// (a workflow's node, its iteration and attempt; an agent's tool call): the
// same parent and site give the same id in every process — a parent replaying
// the step that started a child finds it instead of starting it again. Same
// shape as GenerateRunID.
func ChildRunID(parentRunID, site string) string {
	sum := sha256.Sum256([]byte(parentRunID + "\x00" + site))
	return "run-" + hex.EncodeToString(sum[:8])
}

// IsRunID reports whether id has the shape of a run id ("run-" and 16 hex).
func IsRunID(id string) bool {
	if len(id) != 20 || id[:4] != "run-" {
		return false
	}
	_, err := hex.DecodeString(id[4:])
	return err == nil
}

type spawnerKey struct{}

// WithSpawner makes sp the spawner a machine created with ctx starts its
// children with (the task service's: every child a run).
func WithSpawner(ctx context.Context, sp Spawner) context.Context {
	return context.WithValue(ctx, spawnerKey{}, sp)
}

// SpawnerFrom is the spawner WithSpawner put in ctx, nil when none.
func SpawnerFrom(ctx context.Context) Spawner {
	sp, _ := ctx.Value(spawnerKey{}).(Spawner)
	return sp
}
