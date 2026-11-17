// SPDX-License-Identifier: Apache-2.0

package api

// ThinkingMode represents different thinking/reasoning modes for LLMs.
type ThinkingMode string

const (
	// ThinkingModeNone disables thinking/reasoning.
	ThinkingModeNone ThinkingMode = "none"

	// ThinkingModeLow allocates minimal tokens for thinking (~20% of max tokens).
	ThinkingModeLow ThinkingMode = "low"

	// ThinkingModeMedium allocates moderate tokens for thinking (~50% of max tokens).
	ThinkingModeMedium ThinkingMode = "medium"

	// ThinkingModeHigh allocates maximum tokens for thinking (~80% of max tokens).
	ThinkingModeHigh ThinkingMode = "high"

	// ThinkingModeAuto lets the model decide how much thinking is needed.
	ThinkingModeAuto ThinkingMode = "auto"
)

// ThinkingConfig configures thinking/reasoning behavior for models that support it.
type ThinkingConfig struct {
	// Mode specifies the thinking mode (none, low, medium, high, auto).
	Mode ThinkingMode `json:"mode,omitempty"`

	// BudgetTokens sets explicit token budget for thinking.
	BudgetTokens int `json:"budget_tokens,omitempty"`

	// InterleaveThinking enables thinking between tool calls.
	InterleaveThinking bool `json:"interleave_thinking,omitempty"`
}

// WithThinkingMode sets the thinking mode for the request.
func WithThinkingMode(mode ThinkingMode) CallOption {
	return func(opts *CallOptions) {
		config := getOrCreateThinkingConfig(opts)
		config.Mode = mode
	}
}

// WithThinkingBudget sets explicit token budget for thinking.
func WithThinkingBudget(tokens int) CallOption {
	return func(opts *CallOptions) {
		config := getOrCreateThinkingConfig(opts)
		config.BudgetTokens = tokens
	}
}

// WithInterleaveThinking enables interleaved thinking between tool calls.
func WithInterleaveThinking(enabled bool) CallOption {
	return func(opts *CallOptions) {
		config := getOrCreateThinkingConfig(opts)
		config.InterleaveThinking = enabled
	}
}

// CalculateThinkingBudget calculates the token budget based on mode and max tokens.
func CalculateThinkingBudget(mode ThinkingMode, maxTokens int) int {
	switch mode {
	case ThinkingModeLow:
		return maxTokens * 20 / 100
	case ThinkingModeMedium:
		return maxTokens * 50 / 100
	case ThinkingModeHigh:
		return maxTokens * 80 / 100
	default:
		return 0
	}
}

// getOrCreateThinkingConfig is a helper to get or create thinking config from metadata.
func getOrCreateThinkingConfig(opts *CallOptions) *ThinkingConfig {
	if opts.Metadata == nil {
		opts.Metadata = make(map[string]interface{})
	}
	if existing, ok := opts.Metadata["thinking_config"].(*ThinkingConfig); ok {
		return existing
	}
	config := &ThinkingConfig{}
	opts.Metadata["thinking_config"] = config
	return config
}
