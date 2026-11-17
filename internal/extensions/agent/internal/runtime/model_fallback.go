// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// NewModelFallbackPlugin wraps the fallback hook as a normal plugin so
// the assembly step attaches it like every other built-in (previously
// the hook was appended raw onto the merged hook chain). Returns nil
// when no fallback model is configured — callers skip the add.
func NewModelFallbackPlugin(fallbackModelID string) api.AgentPlugin {
	hook := ModelFallbackHook(fallbackModelID)
	if hook == nil {
		return nil
	}
	return &modelFallbackPlugin{hook: hook}
}

type modelFallbackPlugin struct {
	hook func(api.LoopContext, *api.LoopError) error
}

func (p *modelFallbackPlugin) Name() string            { return "model_fallback" }
func (p *modelFallbackPlugin) SystemMessage() string   { return "" }
func (p *modelFallbackPlugin) ToolDefs() []api.ToolDef { return nil }
func (p *modelFallbackPlugin) Stable() bool            { return true }
func (p *modelFallbackPlugin) HandleToolCall(_ context.Context, _ string, _ json.RawMessage) (*api.ToolCallResult, error) {
	return nil, fmt.Errorf("model_fallback plugin has no tools")
}
func (p *modelFallbackPlugin) Hooks() api.PluginHooks {
	return api.PluginHooks{OnError: p.hook}
}

const fallbackAttemptedKey = "_model_fallback_attempted"

// ModelFallbackHook returns an H9 OnError hook that switches to a fallback model
// when the primary model hits a retryable error (overload, rate limit, server error).
// Single fallback attempt per turn — tracks state via LoopContext metadata.
func ModelFallbackHook(fallbackModelID string) func(ctx api.LoopContext, err *api.LoopError) error {
	if fallbackModelID == "" {
		return nil // no fallback configured
	}

	return func(ctx api.LoopContext, loopErr *api.LoopError) error {
		// Only handle API call errors
		if loopErr.Phase != "api_call" {
			return nil
		}

		// Only retry on retryable errors
		if !isRetryableError(loopErr.Err) {
			return nil
		}

		// Single attempt per run
		if ctx.GetMeta(fallbackAttemptedKey) != nil {
			return nil
		}

		ctx.SetMeta(fallbackAttemptedKey, true)
		ctx.SetModelOverride(fallbackModelID)
		loopErr.Recovered = true
		return nil
	}
}

// isRetryableError checks if an error is retryable (overload, rate limit, server error).
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// HTTP status patterns in error messages
	retryablePatterns := []string{
		"529", "overloaded",
		"429", "rate limit", "rate_limit",
		"500", "502", "503", "504",
		"server error", "internal server error",
		"service unavailable",
	}
	lower := strings.ToLower(msg)
	for _, pattern := range retryablePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
