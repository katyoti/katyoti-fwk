// SPDX-License-Identifier: Apache-2.0

package humangate

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// MachineStrategyDef registers "human-gate" in the MachineDomain.
var MachineStrategyDef = strategies.FuncStrategy[engine.MachineFactory](
	"human-gate", "User prompt gate",
	func() (engine.MachineFactory, error) {
		return &humanGateFactory{}, nil
	},
)

type humanGateFactory struct{}

func (f *humanGateFactory) Name() string { return "human-gate" }

// CreateMachine reads a task's request: the question is config `question`
// (else `message`, else the input's), as a workflow step declares it. A bare
// map (a direct caller) gives `message`.
func (f *humanGateFactory) CreateMachine(_ context.Context, request any) (engine.Machine, error) {
	var message string
	switch r := request.(type) {
	case *taskapi.TaskExecRequest:
		cfg, _ := r.Config.(map[string]any)
		message = firstString(cfg, "question", "message")
		if message == "" {
			message = firstString(r.Input, "question", "message")
		}
	case map[string]any:
		message = firstString(r, "message", "question")
	}
	if message == "" {
		return nil, fmt.Errorf("human-gate: no question (config 'question')")
	}

	return engine.OneShotMachine("human-gate", func(ctx context.Context) (any, error) {
		gate, err := application.UseService[hgapi.HumanGate](ctx)
		if err != nil {
			return nil, fmt.Errorf("human-gate: no HumanGate service: %w", err)
		}
		resp, err := gate.Prompt(ctx, hgapi.PromptRequest{Type: "text", Question: message})
		if err != nil {
			return nil, err
		}
		return resp.Answer, nil
	}), nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, _ := m[k].(string); s != "" {
			return s
		}
	}
	return ""
}
