// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

const (
	outputRecoveryCountKey = "_output_recovery_count"
	maxOutputRecoveries    = 3
)

// outputRecoveryPlugin is an AgentPlugin that recovers from max_tokens truncation.
// Uses H7 OnDecision to detect truncation and force continuation with escalated limits.
type outputRecoveryPlugin struct{}

func (p *outputRecoveryPlugin) Name() string            { return "output_recovery" }
func (p *outputRecoveryPlugin) SystemMessage() string   { return "" }
func (p *outputRecoveryPlugin) ToolDefs() []api.ToolDef { return nil }
func (p *outputRecoveryPlugin) Stable() bool            { return true }
func (p *outputRecoveryPlugin) HandleToolCall(_ context.Context, _ string, _ json.RawMessage) (*api.ToolCallResult, error) {
	return nil, nil
}

func (p *outputRecoveryPlugin) Hooks() api.PluginHooks {
	return api.PluginHooks{
		// A turn that is not cut ends a run of cut ones: the next cut answer
		// is recovered afresh.
		OnTurnEnd: func(ctx api.LoopContext, _ int) error {
			if stop, _ := ctx.GetMeta("_last_stop_reason").(string); stop != "max_tokens" {
				ctx.SetMeta(outputRecoveryCountKey, 0)
			}
			return nil
		},
		OnDecision: func(ctx api.LoopContext, decision *api.Decision) error {
			stopReason, _ := ctx.GetMeta("_last_stop_reason").(string)
			if stopReason != "max_tokens" {
				return nil
			}

			count := 0
			if v := ctx.GetMeta(outputRecoveryCountKey); v != nil {
				count, _ = v.(int)
			}
			if count >= maxOutputRecoveries {
				return nil
			}

			// Escalate at once above the default limit (16K): 32K, then 64K.
			escalated := []int{32768, 65536, 65536}
			idx := count
			if idx >= len(escalated) {
				idx = len(escalated) - 1
			}
			ctx.SetMaxTokens(escalated[idx])
			ctx.SetMeta(outputRecoveryCountKey, count+1)

			decision.Continue = true
			decision.Messages = append(decision.Messages, api.NewTextMessage(api.RoleUser, "Continue from where you left off."))
			return nil
		},
	}
}

// NewOutputRecoveryPlugin creates the output recovery plugin.
func NewOutputRecoveryPlugin() api.AgentPlugin {
	return &outputRecoveryPlugin{}
}
