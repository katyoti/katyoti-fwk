// SPDX-License-Identifier: Apache-2.0

package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
)

// thinkingOpts builds CallOptions carrying a thinking_config, mirroring what
// WithThinkingBudget / WithThinkingMode record in metadata.
func thinkingOpts(cfg *api.ThinkingConfig) *api.CallOptions {
	return &api.CallOptions{
		MaxTokens: 16384,
		Metadata:  map[string]any{"thinking_config": cfg},
	}
}

// TestBuildThinkingConfig_AdaptiveModel pins the fix: modern models (Opus 4.7/4.8,
// Fable 5, …) MUST receive adaptive thinking + output_config.effort. The legacy
// enabled + budget_tokens shape is rejected by those models with a 400
// ("thinking.type.enabled is not supported for this model").
func TestBuildThinkingConfig_AdaptiveModel(t *testing.T) {
	o := &LLM{model: "claude-opus-4-8", reasoning: true}
	opts := thinkingOpts(&api.ThinkingConfig{BudgetTokens: 10000})

	thinking, effort := buildThinkingConfig(o, opts)
	if thinking == nil {
		t.Fatal("expected a thinking config for a reasoning model")
	}
	if thinking.OfAdaptive == nil {
		t.Errorf("opus-4-8 must use adaptive thinking, got %+v", thinking)
	}
	if thinking.OfEnabled != nil {
		t.Error("opus-4-8 must NOT use enabled thinking (rejected with 400)")
	}
	if effort == "" {
		t.Error("adaptive models must carry an output_config.effort level")
	}

	// And the legacy enabled shape must not appear in the serialized request.
	raw, _ := json.Marshal(thinking)
	if strings.Contains(string(raw), `"enabled"`) {
		t.Errorf("serialized thinking still contains enabled: %s", raw)
	}
}

// TestBuildThinkingConfig_LegacyModel confirms older models keep the
// enabled + budget_tokens shape (effort would 400 there).
func TestBuildThinkingConfig_LegacyModel(t *testing.T) {
	o := &LLM{model: "claude-sonnet-4-5", reasoning: true}
	opts := thinkingOpts(&api.ThinkingConfig{BudgetTokens: 8000})

	thinking, effort := buildThinkingConfig(o, opts)
	if thinking == nil || thinking.OfEnabled == nil {
		t.Fatalf("sonnet-4-5 must use enabled thinking, got %+v", thinking)
	}
	if thinking.OfEnabled.BudgetTokens != 8000 {
		t.Errorf("budget_tokens = %d, want 8000", thinking.OfEnabled.BudgetTokens)
	}
	if effort != "" {
		t.Errorf("legacy models must not set effort, got %q", effort)
	}
}

// TestBuildThinkingConfig_NonReasoning verifies a non-reasoning model gets no
// thinking config regardless of metadata.
func TestBuildThinkingConfig_NonReasoning(t *testing.T) {
	o := &LLM{model: "claude-opus-4-8", reasoning: false}
	thinking, effort := buildThinkingConfig(o, thinkingOpts(&api.ThinkingConfig{BudgetTokens: 10000}))
	if thinking != nil || effort != "" {
		t.Errorf("non-reasoning model must get no thinking config, got %+v / %q", thinking, effort)
	}
}

// TestEffortForThinking_ModeMapping checks mode → effort translation.
func TestEffortForThinking_ModeMapping(t *testing.T) {
	cases := map[api.ThinkingMode]string{
		api.ThinkingModeLow:    "low",
		api.ThinkingModeMedium: "medium",
		api.ThinkingModeHigh:   "high",
		api.ThinkingModeNone:   "high", // unset/none defaults to the API default
	}
	for mode, want := range cases {
		if got := string(effortForThinking(&api.ThinkingConfig{Mode: mode})); got != want {
			t.Errorf("effortForThinking(%q) = %q, want %q", mode, got, want)
		}
	}
}
