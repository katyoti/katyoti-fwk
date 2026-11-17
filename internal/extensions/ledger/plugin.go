// SPDX-License-Identifier: Apache-2.0

// Ledger agent plugins.
//
// Two role-specific plugins share one implementation; the role decides which
// tools are exposed and whether the safety-net hooks are wired:
//
//   - ledger-worker  — given to delegates (specialists, workers, teammates).
//     Tools: ledger-snapshot, ledger-update.
//     No ledger-complete (workers can't tear down shared state).
//     No safety hook.
//
//   - ledger-manager — given to the delegator (manager / coordinator / team-leader).
//     Tools: ledger-snapshot, ledger-update, ledger-complete.
//     Safety hook: OnResult + OnAbort fire LedgerCompleteEvent
//     for every ledger ID this agent has interacted with, in
//     case the agent finished without calling ledger-complete
//     explicitly. Without this, the ledger machine suspends
//     forever and the workflow hangs.
//
// Both plugins are wired by AgentPluginExtension() (see extension.go).
//
// The ledger is in-process shared state for one team run: the ledger
// machine holds it, these plugins reach it by EventBus publish, and it ends
// with the run. It is not persisted — durable state is CWM objects.

package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// pluginRole distinguishes the two ledger plugin variants.
type pluginRole string

const (
	roleWorker  pluginRole = "worker"
	roleManager pluginRole = "manager"
)

// WorkerPluginStrategyDef registers the `ledger-worker` plugin strategy.
// Use `plugins: [ledger-worker]` in TASK.md for delegates that read + write
// shared state but never tear it down.
var WorkerPluginStrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"ledger-worker", "Ledger client tools (snapshot + update) for delegate agents",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "ledger-worker",
			CreateFunc:  workerPluginFactory,
		}, nil
	},
)

// ManagerPluginStrategyDef registers the `ledger-manager` plugin strategy.
// Use `plugins: [ledger-manager]` in TASK.md for the delegator (manager /
// coordinator / team-leader). Adds the `ledger-complete` tool plus an
// OnResult/OnAbort safety hook that auto-fires LedgerCompleteEvent for every
// tracked ledger if the agent finishes without calling ledger-complete.
var ManagerPluginStrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"ledger-manager", "Ledger client tools (snapshot + update + complete) for delegator agents, with auto-complete safety net",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "ledger-manager",
			CreateFunc:  managerPluginFactory,
		}, nil
	},
)

func workerPluginFactory(ctx context.Context, loop agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	ch := engine.ChannelFrom(ctx)
	if ch == nil {
		return nil, nil
	}
	return []agentapi.AgentPlugin{newLedgerPlugin(loop, ch.Bus(), roleWorker)}, nil
}

func managerPluginFactory(ctx context.Context, loop agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	ch := engine.ChannelFrom(ctx)
	if ch == nil {
		return nil, nil
	}
	return []agentapi.AgentPlugin{newLedgerPlugin(loop, ch.Bus(), roleManager)}, nil
}

// ledgerPlugin holds the per-agent EventBus reference, snapshot cache, and
// (for managers) the set of ledger IDs the agent has interacted with — used
// by the safety hook to fire ledger-complete on agent termination.
type ledgerPlugin struct {
	loop agentapi.LoopContext
	bus  engine.EventBus
	role pluginRole

	mu        sync.Mutex
	cache     map[string]*engine.LedgerSnapshotEvent // id → latest snapshot
	subbed    map[string]bool                        // id → already subscribed
	completed map[string]bool                        // id → already completed (manager only; prevents double-fire)
}

func newLedgerPlugin(loop agentapi.LoopContext, bus engine.EventBus, role pluginRole) *ledgerPlugin {
	return &ledgerPlugin{
		loop:      loop,
		bus:       bus,
		role:      role,
		cache:     make(map[string]*engine.LedgerSnapshotEvent),
		subbed:    make(map[string]bool),
		completed: make(map[string]bool),
	}
}

func (p *ledgerPlugin) Name() string {
	if p.role == roleManager {
		return "ledger-manager"
	}
	return "ledger-worker"
}

func (p *ledgerPlugin) Stable() bool { return true }

// Hooks returns the OnResult + OnAbort safety net for managers (auto-fires
// LedgerCompleteEvent for every tracked ledger ID), or empty hooks for workers.
func (p *ledgerPlugin) Hooks() agentapi.PluginHooks {
	if p.role != roleManager {
		return agentapi.PluginHooks{}
	}
	return agentapi.PluginHooks{
		OnResult: func(_ agentapi.LoopContext, _ *agentapi.LoopResult) error {
			p.completeAllTracked("agent-result")
			return nil
		},
		OnAbort: func(_ agentapi.LoopContext) error {
			p.completeAllTracked("agent-abort")
			return nil
		},
	}
}

func (p *ledgerPlugin) SystemMessage() string {
	common := "Ledger tools coordinate shared mutable state across agents. " +
		"Use `ledger-update` (publish a JSON merge patch), `ledger-snapshot` (read current state immediately), " +
		"and `ledger-wait-update` (BLOCK until a newer snapshot arrives — pass the version you last saw as " +
		"`since_version`). " +
		"**Prefer `ledger-wait-update` over looping `ledger-snapshot`** — repeated `ledger-snapshot` calls with " +
		"the same arguments will be aborted by the agent loop's repeat-tool-call guard, and the wait variant " +
		"is faster, cheaper, and event-driven. The `ledger_id` argument is the run ID of the ledger machine."
	if p.role == roleManager {
		return common +
			" (Manager role) You also have `ledger-complete` to terminate the ledger when coordination is finished. " +
			"Call it as your last action before `complete`. (If you forget, the safety net fires complete on your behalf when you return.)"
	}
	return common + " (Worker role) The ledger_id is provided to you by the manager."
}

func (p *ledgerPlugin) ToolDefs() []agentapi.ToolDef {
	tools := ledgerTools(p)
	defs := make([]agentapi.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = t.ToolDef()
	}
	return defs
}

func (p *ledgerPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	for _, t := range ledgerTools(p) {
		if t.ToolDef().Name == name {
			return t.ToolCall(ctx, input)
		}
	}
	return nil, fmt.Errorf("unknown tool: %q", name)
}

// ensureSubscribed lazy-subscribes to `ledger.<id>.snapshot` and caches each event.
// Idempotent — safe to call repeatedly. Also tracks the ID for the manager's
// safety hook (so we know which ledgers to complete on agent termination).
func (p *ledgerPlugin) ensureSubscribed(id string) {
	p.mu.Lock()
	if p.subbed[id] {
		p.mu.Unlock()
		return
	}
	p.subbed[id] = true
	p.mu.Unlock()

	topic := engine.LedgerSnapshotTopic(id)
	_ = p.bus.Subscribe(topic, func(_ string, payload any) {
		evt, ok := payload.(*engine.LedgerSnapshotEvent)
		if !ok {
			return
		}
		p.mu.Lock()
		p.cache[id] = evt
		p.mu.Unlock()
	})
}

// readCache returns the latest cached snapshot for id, or nil if none.
func (p *ledgerPlugin) readCache(id string) *engine.LedgerSnapshotEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cache[id]
}

// agentSource returns this agent's identity for the LedgerUpdateEvent.Source field.
// Falls back to "agent" when no teammate name is set.
func (p *ledgerPlugin) agentSource() string {
	if p.loop == nil {
		return "agent"
	}
	if name, ok := p.loop.GetMeta("_teammate_name").(string); ok && name != "" {
		return name
	}
	if name, ok := p.loop.GetMeta("_runId").(string); ok && name != "" {
		return name
	}
	return "agent"
}

// publishUpdate emits a LedgerUpdateEvent on the ledger.<id>.update topic.
func (p *ledgerPlugin) publishUpdate(id string, patch map[string]any) error {
	if patch == nil {
		patch = map[string]any{}
	}
	evt := &engine.LedgerUpdateEvent{Patch: patch, Source: p.agentSource()}
	evt.StampEvent(engine.LedgerUpdateTopic(id), p.agentSource())
	return p.bus.Publish(engine.LedgerUpdateTopic(id), evt)
}

// publishComplete emits a LedgerCompleteEvent on the ledger.<id>.complete topic.
// Idempotent per id: subsequent calls for the same id are no-ops, so explicit
// `ledger-complete` followed by the safety hook doesn't double-fire.
func (p *ledgerPlugin) publishComplete(id, reason string) error {
	p.mu.Lock()
	if p.completed[id] {
		p.mu.Unlock()
		return nil
	}
	p.completed[id] = true
	p.mu.Unlock()

	evt := &engine.LedgerCompleteEvent{Reason: reason}
	evt.StampEvent(engine.LedgerCompleteTopic(id), p.agentSource())
	return p.bus.Publish(engine.LedgerCompleteTopic(id), evt)
}

// completeAllTracked fires LedgerCompleteEvent for every ledger ID this
// agent has interacted with. Called by the manager's OnResult/OnAbort hooks
// as a safety net so workflows don't hang when the manager forgot.
func (p *ledgerPlugin) completeAllTracked(reason string) {
	p.mu.Lock()
	ids := make([]string, 0, len(p.subbed))
	for id := range p.subbed {
		if !p.completed[id] {
			ids = append(ids, id)
		}
	}
	p.mu.Unlock()

	for _, id := range ids {
		_ = p.publishComplete(id, reason)
	}
}

// waitForSnapshot blocks up to timeout for the cache to be populated for id.
// Returns the cached snapshot or nil on timeout.
func (p *ledgerPlugin) waitForSnapshot(id string, timeout time.Duration) *engine.LedgerSnapshotEvent {
	deadline := time.Now().Add(timeout)
	for {
		if snap := p.readCache(id); snap != nil {
			return snap
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitForVersionAbove blocks up to timeout for a snapshot whose version is
// strictly greater than `since`. Returns:
//   - (snap, true)  if a newer snapshot arrived
//   - (cur,  false) on timeout (cur is whatever's in the cache; may be nil)
//
// This is the foundation of `ledger-wait-update`. Each tool call has unique
// args (the agent passes the version it's already seen as `since`), which
// avoids the agent loop's repeat-tool-call circuit breaker — that guard
// would otherwise abort agents polling the same `ledger-snapshot` until
// state changes.
func (p *ledgerPlugin) waitForVersionAbove(id string, since int, timeout time.Duration) (*engine.LedgerSnapshotEvent, bool) {
	deadline := time.Now().Add(timeout)
	for {
		snap := p.readCache(id)
		if snap != nil && snap.Version > since {
			return snap, true
		}
		if time.Now().After(deadline) {
			return snap, false
		}
		time.Sleep(50 * time.Millisecond)
	}
}
