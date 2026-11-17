// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// ContainerState tracks the engine container's operational state.
type ContainerState int

const (
	StateIdle    ContainerState = iota // Ready to accept tasks
	StateRunning                       // Agent loop is active
)

func (s ContainerState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	default:
		return "unknown"
	}
}

// EngineContainer wraps the engine as a KTSP container with compute role.
// It exposes two slots:
//   - agent: run/status/stop — manages the agent loop lifecycle
//   - tool:  call/list — dispatches tool calls to registered handlers
type EngineContainer struct {
	*ktsp.Endpoint

	mu         sync.RWMutex
	state      ContainerState
	turn       int
	maxTurns   int
	cancelFunc context.CancelFunc // to stop a running agent loop
	lastResult *agentapi.RunResult

	// The dispatcher handles tool calls on the tool slot (skeleton).
	dispatcher agentapi.ToolDispatcher
}

// NewEngineContainer creates a new engine container.
// The dispatcher is used to handle incoming tool/call and tool/list requests
// from other containers (skeleton side).
func NewEngineContainer(dispatcher agentapi.ToolDispatcher) *EngineContainer {
	ec := &EngineContainer{
		Endpoint: ktsp.NewEndpoint(
			"engine",
			"Agent loop orchestration. Runs LLM-driven tasks with tool dispatch.",
			"1.0.0",
		),
		state:      StateIdle,
		dispatcher: dispatcher,
	}
	ec.registerAgentSlot()
	ec.registerToolSlot()
	return ec
}

// State returns the current container state.
func (ec *EngineContainer) State() ContainerState {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.state
}

// SetRunning marks the container as running with the given turn info.
// Called by the agent loop to update state.
func (ec *EngineContainer) SetRunning(turn, maxTurns int) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.state = StateRunning
	ec.turn = turn
	ec.maxTurns = maxTurns
}

// SetIdle marks the container as idle with the given result.
// Called by the agent loop when it finishes.
func (ec *EngineContainer) SetIdle(result *agentapi.RunResult) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.state = StateIdle
	ec.lastResult = result
	ec.cancelFunc = nil
}

// SetCancelFunc stores a cancel function for the running agent loop.
func (ec *EngineContainer) SetCancelFunc(cancel context.CancelFunc) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.cancelFunc = cancel
}

// registerAgentSlot creates the agent slot with run/status/stop methods.
func (ec *EngineContainer) registerAgentSlot() {
	slot := ktsp.NewSlot("agent", ktsp.DefaultInstance, "Agent loop lifecycle management.", nil)

	// status — return current engine state
	slot.Add(ktsp.RawMethod("status", ktsp.MethodOpts{Desc: "Return current engine state"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		ec.mu.RLock()
		defer ec.mu.RUnlock()

		status := map[string]any{
			"state": ec.state.String(),
			"turn":  ec.turn,
		}
		if ec.maxTurns > 0 {
			status["maxTurns"] = ec.maxTurns
		}
		if ec.lastResult != nil {
			status["lastResult"] = map[string]any{
				"status":  ec.lastResult.Status.String(),
				"summary": ec.lastResult.Summary,
				"turns":   ec.lastResult.Turns,
			}
		}
		return json.Marshal(status)
	}))

	// stop — cancel the running agent loop
	slot.Add(ktsp.RawMethod("stop", ktsp.MethodOpts{Desc: "Cancel the running agent loop"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		ec.mu.Lock()
		defer ec.mu.Unlock()

		if ec.state != StateRunning {
			return nil, ktsp.ErrInvalidRequest("engine is not running")
		}
		if ec.cancelFunc != nil {
			ec.cancelFunc()
		}
		return json.Marshal(map[string]string{"status": "stopping"})
	}))

	ec.AddSlot(slot)
}

// registerToolSlot creates the tool slot with call/list methods.
// This is the skeleton: receives KTSP requests and dispatches to the local tool handlers.
func (ec *EngineContainer) registerToolSlot() {
	slot := ktsp.NewSlot("tool", ktsp.DefaultInstance, "Tool dispatch. Routes tool calls to registered handlers.", nil)

	// call — dispatch a tool by name
	slot.Add(ktsp.RawMethod("call", ktsp.MethodOpts{Desc: "Dispatch a tool by name"}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			Tool  string          `json:"tool"`
			Input json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}
		if p.Tool == "" {
			return nil, ktsp.ErrInvalidParams("tool name is required")
		}

		result, err := ec.dispatcher.Dispatch(ctx, p.Tool, p.Input)
		if err != nil {
			return json.Marshal(map[string]any{
				"result":  "",
				"isError": true,
				"error":   err.Error(),
			})
		}
		return json.Marshal(map[string]any{
			"result":  result,
			"isError": false,
		})
	}))

	// list — return available tool definitions
	slot.Add(ktsp.RawMethod("list", ktsp.MethodOpts{Desc: "Return available tool definitions"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if ec.dispatcher == nil {
			return json.Marshal([]agentapi.ToolDef{})
		}
		return json.Marshal(ec.dispatcher.ToolDefs())
	}))

	ec.AddSlot(slot)
}

// EngineContainerFactory creates a ContainerFactory function for registering
// the engine with the ContainerManager.
func EngineContainerFactory(dispatcher agentapi.ToolDispatcher) func() ktsp.Container {
	return func() ktsp.Container {
		return NewEngineContainer(dispatcher)
	}
}

// ContainerDescriptorSlots returns the slot names the engine container provides.
// Used when registering with the ContainerManager's catalog.
func ContainerDescriptorSlots() []string {
	return []string{"agent", "tool"}
}

// StatusInfo is the JSON response from agent/status.
type StatusInfo struct {
	State    string          `json:"state"`
	Turn     int             `json:"turn"`
	MaxTurns int             `json:"maxTurns,omitempty"`
	Last     *LastResultInfo `json:"lastResult,omitempty"`
}

// LastResultInfo is part of StatusInfo.
type LastResultInfo struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Turns   int    `json:"turns"`
}

// ParseStatusInfo parses an agent/status response.
func ParseStatusInfo(data json.RawMessage) (*StatusInfo, error) {
	var info StatusInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}
	return &info, nil
}
