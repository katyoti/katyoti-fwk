// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// SpawnerDefaultRetentionTTL bounds how long terminal child entries linger in
// LocalSpawner.children after completion. Long enough for in-flight readers
// (Result calls); short enough to keep RSS bounded in long-running processes.
const SpawnerDefaultRetentionTTL = 5 * time.Minute

// LocalSpawner manages child executors on a shared engine.Channel.
// Created per-workflow with the parent's engine.Channel injected.
// Children are ephemeral (no StateStore).
type LocalSpawner struct {
	channel *engine.Channel

	mu           sync.RWMutex
	children     map[string]*childEntry
	retentionTTL time.Duration // bounds terminal-child lifetime in `children`
	lruCap       int           // LRU count ceiling — see defaultLRUCap
}

type childEntry struct {
	handle      engine.ChildHandle
	cancel      context.CancelFunc
	cancelWith  context.CancelCauseFunc // cancels with a reason (Cancel)
	done        chan struct{}
	output      any
	err         error
	cancelUnsub func()    // unsubscribe handle for the external engine.CancelRequestTopic listener
	terminalAt  time.Time // zero until status becomes terminal; used for TTL pruning
}

// NewLocalSpawner creates a engine.Spawner that runs children on the given engine.Channel.
// The engine.Channel is shared with the parent executor so lifecycle events flow
// via the same EventBus.
func NewLocalSpawner(channel *engine.Channel) *LocalSpawner {
	return &LocalSpawner{
		channel:      channel,
		children:     make(map[string]*childEntry),
		retentionTTL: SpawnerDefaultRetentionTTL,
		lruCap:       defaultLRUCap,
	}
}

// SetRetentionTTL overrides the terminal-child retention window. Used by
// tests to drive fast pruning (e.g. 1ms) without waiting for the default.
func (s *LocalSpawner) SetRetentionTTL(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retentionTTL = d
}

// SetLRUCap overrides the LRU count ceiling for the children map.
func (s *LocalSpawner) SetLRUCap(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lruCap = n
}

// QueueDepth returns the current child-entry count.
func (s *LocalSpawner) QueueDepth() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.children)
}

// pruneStaleChildrenLocked removes terminal child entries older than
// retentionTTL then enforces the LRU count cap. Caller must hold s.mu.
// Eviction sites log to slog.Info with reason="ttl"|"lru" + map="spawner".
func (s *LocalSpawner) pruneStaleChildrenLocked(now time.Time) {
	pruneAndCap(s.children, s.retentionTTL, s.lruCap, now, "spawner",
		func(e *childEntry) time.Time {
			if e == nil {
				return time.Time{}
			}
			return e.terminalAt
		})
}

// DryRunSubstituted is the set of machine kinds replaced by the simulator
// strategy when DryRunMode is active. Substitution kicks in when the kind
// either burns LLM tokens (agent, task), blocks on the user (human-gate),
// triggers external side-effects (program), or BLOCKS waiting for events
// that won't fire in dry-run (ledger — its terminator events are emitted by
// agent tools that get substituted away, so it would hang forever).
//
// Other kinds (simulator, template, workflow) pass through:
// they're cheap, side-effect-free, AND terminate naturally without external
// stimulus.
//
// Exported so TaskService can apply the same substitution rule to top-level
// runs (not just spawner-mediated children).
var DryRunSubstituted = map[string]bool{
	"agent":      true,
	"task":       true,
	"human-gate": true,
	"ledger":     true, // suspends forever in dry-run (no agent to send ledger.complete)
}

// DryRunSyntheticConfig builds a simulator config that returns a deterministic
// placeholder identifying the substituted run. The {dry_run: true} marker in
// the output lets downstream consumers (tracer, tests, agents that read the
// child's result) detect simulated runs.
//
// `name` is the role/child name (used in the summary). `originalKind` is the
// kind being substituted (preserved in output for the tracer). `input` is the
// caller's input map (only its key set is preserved, not the values).
func DryRunSyntheticConfig(name, originalKind string, input map[string]any) map[string]any {
	inputKeys := make([]string, 0, len(input))
	for k := range input {
		inputKeys = append(inputKeys, k)
	}
	return map[string]any{
		"duration_ms": 0,
		"output": map[string]any{
			"summary":    "DRY_RUN: " + name,
			"child_kind": originalKind,
			"input_keys": inputKeys,
			"dry_run":    true,
		},
	}
}

func (s *LocalSpawner) Spawn(ctx context.Context, spec engine.ChildSpec) (engine.ChildHandle, error) {
	// 0. Dry-run substitution: replace LLM/program kinds with the simulator
	//    strategy. DryRunSyntheticConfig captures the original kind in the
	//    output payload so the tracer can show "agent→sim manager" by reading
	//    output.child_kind on child.<name>.done.
	if spec.DryRunMode && DryRunSubstituted[spec.Kind] {
		spec.Config = DryRunSyntheticConfig(spec.Name, spec.Kind, spec.Input)
		spec.Kind = "simulator"
		spec.Body = "" // simulator has no body
	}

	// 1. Lookup factory.
	factory, err := engine.LookupFactory(spec.Kind)
	if err != nil {
		return engine.ChildHandle{}, fmt.Errorf("spawn %s: %w", spec.Name, err)
	}

	// 2. Build TaskExecRequest. Propagate DryRunMode so nested workflows
	//    inherit dry-run behavior for THEIR own children.
	execReq := &taskapi.TaskExecRequest{
		Config: spec.Config,
		Input:  spec.Input,
		Body:   spec.Body,
		Runtime: taskapi.RuntimeContext{
			ParentRunID: spec.ParentID,
			DryRunMode:  spec.DryRunMode,
			CurrentNode: spec.CurrentNode,
		},
	}

	// 2a. UC-053 / UC-054 — STRUCTURAL CLOSE of sec-1 / threat-T1.
	// Apply parent's spawn policies via presets.BuildChildOverride. Both
	// delegate-spawn (taskplugin sets ConfigOverride on TaskRunRequest, merged
	// by TaskService.buildExecRequest) AND workflow-spawn (this path) follow
	// the same merge convention: ConfigOverride wins over task-level config
	// for this invocation. Without this, workflow-spawned children of a
	// coordinator escape mode coercion entirely (the bug from sec-1).
	// Apply mode coercion whenever there is ANYTHING to apply: the child's
	// own mode, a parent mode (whose policies may restrict the child), or
	// inherited sticky policy names. Gating only on spec.Mode != "" lets a
	// MODELESS child under a policy-carrying parent silently escape
	// restriction (a nested workflow's plain `task:` steps carry no `mode:`,
	// so coordinator-restricts-children would never apply to grandchildren).
	// Empty Mode resolves to the worker preset.
	if override := childOverride(spec); override != nil {
		execReq.ConfigOverride = override
		if len(override) > 0 {
			merged := make(map[string]any, len(spec.Config)+len(override))
			for k, v := range spec.Config {
				merged[k] = v
			}
			// ConfigOverride wins over task-level config (mirrors TaskService).
			for k, v := range override {
				merged[k] = v
			}
			execReq.Config = merged
		}
	}

	// 3. Inject the shared engine.Channel into ctx so the child's factory + plugins
	//    can resolve it via engine.ChannelFrom(ctx). Without this, plugins
	//    that need EventBus access (e.g., ledger-update, orchestration's
	//    mailbox bridge) silently drop their wiring on workflow-spawned
	//    children. Mirrors what TaskService.Run does for top-level runs.
	machineCtx := engine.WithChannel(ctx, s.channel)

	// 4. Create machine.
	machine, err := factory.CreateMachine(machineCtx, execReq)
	if err != nil {
		return engine.ChildHandle{}, fmt.Errorf("spawn %s: create machine: %w", spec.Name, err)
	}

	// 5. Create executor on shared engine.Channel (no StateStore — ephemeral),
	//    with the hooks every run has (newRunExecutor): a child waiting on an
	//    external event (an inbox answer) is woken by it. No groups: a
	//    workflow's children hold no slot of their own.
	runID := engine.GenerateRunID()
	if spec.Site != "" && spec.ParentID != "" {
		runID = engine.ChildRunID(spec.ParentID, spec.Site)
		s.mu.RLock()
		existing, ok := s.children[runID]
		s.mu.RUnlock()
		if ok { // started from this site already: the same child
			return existing.handle, nil
		}
	}
	executor := newRunExecutor(machine, s.channel, nil, runID, nil, nil)

	handle := engine.ChildHandle{
		RunID:  runID,
		Name:   spec.Name,
		Kind:   spec.Kind,
		Status: engine.ChildRunning,
	}

	// childCtx inherits the engine.Channel via machineCtx so the running child also
	// observes it (executor.Run / plugin hooks invoked at runtime).
	// MaxDurationS (per-delegate Dimension 1 limit) bounds the child's
	// wall-clock runtime when set.
	// Cancelled with a cause (Cancel's reason): the child's error says why.
	childCtx, cancelWith := context.WithCancelCause(machineCtx)
	cancel := func() { cancelWith(nil) }
	if spec.MaxDurationS > 0 {
		var stopTimeout context.CancelFunc
		childCtx, stopTimeout = context.WithTimeout(childCtx, time.Duration(spec.MaxDurationS)*time.Second)
		cancel = func() { stopTimeout(); cancelWith(nil) }
	}
	entry := &childEntry{
		handle:     handle,
		cancel:     cancel,
		cancelWith: cancelWith,
		done:       make(chan struct{}),
	}

	s.mu.Lock()
	s.pruneStaleChildrenLocked(time.Now())
	s.children[runID] = entry
	s.mu.Unlock()

	// 5a. Subscribe to external cancel-request topic for this child. Lets
	// non-spawner callers (GUI, CLI, sidecars) trigger cancellation by
	// publishing engine.CancelRequestTopic(runID). The handler delegates to the
	// spawner's internal Cancel — which itself emits `to.<name>.cancel`
	// and terminates the child's context. The subscription is removed
	// implicitly when the goroutine below exits and the bus drops the
	// callback's reference.
	cancelTopic := engine.CancelRequestTopic(runID)
	cancelUnsub := s.channel.Bus().Subscribe(cancelTopic, func(_ string, payload any) {
		reason, _ := payload.(string)
		if reason == "" {
			reason = "external-cancel-request"
		}
		_ = s.Cancel(runID, reason)
	})
	entry.cancelUnsub = cancelUnsub

	// 5b. Emit child.started. The spawner fills the correlation envelope's
	// run-scoped fields (layered stamping — it knows both sides).
	startedEvt := &engine.ChildStartedEvent{
		ChildName:   spec.Name,
		ChildKind:   spec.Kind,
		RunID:       runID,
		ParentRunID: spec.ParentID,
		Mode:        spec.Mode,
	}
	startedEvt.Correlation = engine.Correlation{RunID: runID, ParentRunID: spec.ParentID}
	s.channel.Bus().Publish(engine.ChildStartedTopic(spec.Name), startedEvt)
	// Flat mirror for the SessionRecorder (it can't subscribe to the
	// name-templated topic above). This is the COMPLETE delegation event the
	// cold task-tree projection rebuilds the graph from, keyed by child run id.
	s.channel.Bus().Publish(engine.ChildStartedAuditTopic(), startedEvt)

	// 5b-bis. Framework task START for the child run — the symmetric bookend
	// to the task.output emitted at completion below. Stamped with the
	// parent's runID (like task.output) so the parent's SessionRecorder
	// captures the child's start dimensions + input. A child can never start
	// unrecorded — mirrors TaskService.Run's framework-emitted task.started.
	{
		startEmitter := spec.ParentID
		if startEmitter == "" {
			startEmitter = runID
		}
		taskStarted := &engine.TaskStartedEvent{
			RunID:      runID,
			TaskName:   spec.Name,
			TaskType:   spec.Kind,
			Mode:       spec.Mode,
			Dimensions: dimensionsForMode(spec.Mode),
			Input:      spec.Input,
		}
		taskStarted.Correlation = engine.Correlation{RunID: runID, ParentRunID: spec.ParentID}
		taskStarted.StampEvent(engine.EventTaskStarted, startEmitter)
		s.channel.Bus().Publish(engine.EventTaskStarted, taskStarted)
	}

	// 6. Run child in goroutine.
	go func() {
		defer close(entry.done)
		defer cancel()
		// Unsubscribe the external cancel-request listener once the child
		// is done, regardless of why (success, failure, internal cancel).
		// Prevents leaking the bus subscription past the child's lifetime.
		defer func() {
			if entry.cancelUnsub != nil {
				entry.cancelUnsub()
			}
		}()

		// A panicking machine must not take down the process (an unrecovered
		// panic in a goroutine is fatal) nor vanish without its terminal
		// lifecycle event — convert the panic into a normal child failure so
		// the parent's WaitSpec resumes on child.<name>.failed as usual.
		result, err := func() (r *engine.RunResult, runErr error) {
			defer func() {
				if p := recover(); p != nil {
					r, runErr = nil, fmt.Errorf("child %s panicked: %v", spec.Name, p)
				}
			}()
			return executor.Run(childCtx, engine.Event{Type: "start", Payload: execReq})
		}()

		// A step that failed ends the child with its error in the result.
		if err == nil && result != nil && result.Next == engine.Failed && result.Error != nil {
			err = result.Error
		}
		// A machine that ends with a Failure (an agent whose run failed, a
		// workflow whose step failed) has failed, as for a run the
		// TaskService starts.
		if err == nil && result != nil && result.Failure != nil {
			err = errors.New(result.Failure.Reason)
		}
		if errors.Is(err, context.Canceled) {
			if cause := context.Cause(childCtx); cause != nil && !errors.Is(cause, context.Canceled) {
				err = cause // cancelled with a reason: it is the child's error
			}
		}
		s.mu.Lock()
		if err != nil {
			entry.handle.Status = engine.ChildFailed
			entry.err = err
		} else if result != nil && result.Next == engine.Done {
			entry.handle.Status = engine.ChildDone
			entry.output = result.Output
		} else {
			entry.handle.Status = engine.ChildFailed
			entry.err = fmt.Errorf("child %s ended without output", spec.Name)
		}
		entry.terminalAt = time.Now()
		s.mu.Unlock()

		// Phase C.2: emit a task_output event for the child (mirrors the
		// top-level emission from TaskService.Run). Lets the L2 runner find
		// child dimensions via `expect_dimensions: {child: <name>, ...}` —
		// the runner matches by `task_name` so spec.Name is authoritative.
		{
			var outputMap map[string]any
			if m, ok := entry.output.(map[string]any); ok {
				outputMap = m
			} else if entry.output != nil {
				// Agent/simulator children often return a BARE value (a string
				// summary from complete, a number, etc.). Wrap it so the cold
				// task-tree + edge-detail panel can show "what the child returned"
				// instead of dropping it — task.output.Output is map-typed.
				outputMap = map[string]any{"result": entry.output}
			}
			status := "completed"
			errMsg := ""
			switch entry.handle.Status {
			case engine.ChildFailed:
				status = "failed"
				if entry.err != nil {
					errMsg = entry.err.Error()
				}
			case engine.ChildCancelled:
				status = "cancelled"
			}
			tEvt := &engine.TaskOutputEvent{
				RunID:    runID,
				TaskName: spec.Name,
				Status:   status,
				Output:   outputMap,
			}
			tEvt.Correlation = engine.Correlation{RunID: runID, ParentRunID: spec.ParentID}
			if errMsg != "" {
				// Surface the error string in the journal under a stable key
				// so L2 assertions can match on it. Output map is the only
				// payload slot; non-nil to ensure the key reaches the journal.
				if tEvt.Output == nil {
					tEvt.Output = map[string]any{}
				}
				tEvt.Output["__error"] = errMsg
			}
			// Stamp emitter as the parent's runID (when present) so the parent
			// run's SessionRecorder — which filters by `Emitter == ownRunID` —
			// picks up the child's terminal payload. The child's own runID is
			// preserved in tEvt.RunID for downstream lookup. Root spawns
			// (ParentID == "") stamp with the child's own runID; not useful to
			// the parent recorder but consistent.
			emitter := spec.ParentID
			if emitter == "" {
				emitter = runID
			}
			tEvt.StampEvent(engine.EventTaskOutput, emitter)
			s.channel.Bus().Publish(engine.EventTaskOutput, tEvt)
		}

		// Its end on its own run's topic: the parent waits on it, and reads
		// how it ended from Outcome.
		s.channel.Bus().Publish(engine.RunEndedTopic(runID), &engine.TaskOutputEvent{RunID: runID, TaskName: spec.Name})

		// Emit lifecycle event (correlation: spawner knows run + parent).
		corr := engine.Correlation{RunID: runID, ParentRunID: spec.ParentID}
		if entry.handle.Status == engine.ChildDone {
			doneEvt := &engine.ChildDoneEvent{
				ChildName: spec.Name,
				RunID:     runID,
				Output:    entry.output,
			}
			doneEvt.Correlation = corr
			s.channel.Bus().Publish(engine.ChildDoneTopic(spec.Name), doneEvt)
			s.channel.Bus().Publish(engine.ChildDoneAuditTopic(), doneEvt)
		} else {
			errMsg := "unknown error"
			if entry.err != nil {
				errMsg = entry.err.Error()
			}
			failedEvt := &engine.ChildFailedEvent{
				ChildName: spec.Name,
				RunID:     runID,
				Error:     errMsg,
			}
			failedEvt.Correlation = corr
			s.channel.Bus().Publish(engine.ChildFailedTopic(spec.Name), failedEvt)
			s.channel.Bus().Publish(engine.ChildFailedAuditTopic(), failedEvt)
		}
	}()

	return handle, nil
}

func (s *LocalSpawner) Cancel(runID string, reason string) error {
	s.mu.RLock()
	entry, ok := s.children[runID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("child %q not found", runID)
	}

	// Emit cancel signal on bus for cooperative cancellation.
	s.channel.Bus().Publish(engine.ChildCancelTopic(entry.handle.Name), reason)

	// Cancel context as hard stop, with the reason.
	if entry.cancelWith != nil && reason != "" {
		entry.cancelWith(fmt.Errorf("cancelled: %s", reason))
	}
	entry.cancel()

	s.mu.Lock()
	entry.handle.Status = engine.ChildCancelled
	if entry.terminalAt.IsZero() {
		entry.terminalAt = time.Now()
	}
	s.mu.Unlock()

	return nil
}

func (s *LocalSpawner) Result(runID string) (any, bool) {
	s.mu.RLock()
	entry, ok := s.children[runID]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if entry.handle.Status != engine.ChildDone {
		return nil, false
	}
	return entry.output, true
}

// Outcome is how a child of this spawner ended (ended false while it runs).
func (s *LocalSpawner) Outcome(runID string) (engine.ChildOutcome, bool) {
	s.mu.RLock()
	entry, ok := s.children[runID]
	s.mu.RUnlock()
	if !ok {
		return engine.ChildOutcome{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry.terminalAt.IsZero() { // set, with its status, before its end is published
		return engine.ChildOutcome{}, false
	}
	out := engine.ChildOutcome{Status: entry.handle.Status}
	switch {
	case entry.handle.Status == engine.ChildDone:
		out.Output = entry.output
	case entry.err != nil:
		out.Error = entry.err.Error()
	default:
		out.Error = "child " + entry.handle.Name + " " + string(entry.handle.Status)
	}
	return out, true
}

// Ensure LocalSpawner implements engine.Spawner.
var _ engine.Spawner = (*LocalSpawner)(nil)

// childOverride is the config a child's spawn policies impose (its mode, its
// parent's, the sticky policies it inherits — UC-053 / UC-054); no mode is
// the worker preset. Resurrecting the sticky policies from their names lets a mid-worker
// carrying the coordinator's policy re-apply it to its own children.
func childOverride(spec engine.ChildSpec) map[string]any {
	// No mode is the worker preset, as for the delegate tool: never no
	// override, which would leave an agent child its dangerous tools denied.
	return presets.BuildChildOverrideWithPolicies(
		presets.Mode(spec.Mode),
		presets.Mode(spec.ParentMode),
		presets.PoliciesByName(spec.ParentSpawnPolicyNames),
	)
}
