// SPDX-License-Identifier: Apache-2.0

package profile

import (
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// AgentProfile is a named bundle of model selection + agent behavior.
// Defined in Profiles.yaml. The model field references a model type name
// from LLMProviders.yaml, with optional provider-specific overrides.
type AgentProfile struct {
	Name string `yaml:"name,omitempty"`

	// Model references a model type name (e.g., "claude-opus").
	// Can be a plain string or a map with "name" + provider-specific overrides.
	Model ProfileModel `yaml:"model,omitempty"`

	// Agent behavior
	MaxTurns           *int                 `yaml:"max_turns,omitempty"`
	Plugins            []agentapi.PluginRef `yaml:"plugins,omitempty"`
	SystemPromptPrefix string               `yaml:"system_prompt_prefix,omitempty"`
	FallbackModel      string               `yaml:"fallback_model,omitempty"`

	// LogModelCalls overrides the config-level default for this
	// profile: when effective-true, every model call of a session made
	// under this profile is dumped at HTTP level (auth scrubbed) into
	// the session's llm-model-calls.log. nil = inherit the global
	// `log_model_calls` default.
	LogModelCalls *bool `yaml:"log_model_calls,omitempty"`

	// Compaction tunes auto-compaction for runs on this profile. Omitted →
	// enabled with default ratios of the model's context window
	// (LLMProviders.yaml context_window). Set `enabled: false` to turn it off.
	Compaction *CompactionConfig `yaml:"compaction,omitempty"`
}

// CompactionConfig is the per-profile auto-compaction policy. All sizing is
// expressed as FRACTIONS of the model's configured context window — never
// absolute token counts — so one policy behaves correctly across a 128K and a
// 1M model.
type CompactionConfig struct {
	// Enabled toggles compaction. nil → enabled (the default). false → off.
	Enabled *bool `yaml:"enabled,omitempty"`
	// TriggerRatio: compact once the prompt reaches this fraction of the
	// context window. 0 → built-in default.
	TriggerRatio float64 `yaml:"trigger_ratio,omitempty"`
	// KeepRecentRatio: fraction of the window kept verbatim as the recent
	// tail. 0 → built-in default.
	KeepRecentRatio float64 `yaml:"keep_recent_ratio,omitempty"`
	// SummaryMaxTokens caps the summarizer's output. 0 → built-in default.
	SummaryMaxTokens int `yaml:"summary_max_tokens,omitempty"`
}

// ProfileModel holds a model type reference with optional overrides.
// In YAML, can be a plain string ("claude-opus") or a map ({name: claude-opus, thinking: ...}).
type ProfileModel struct {
	// Name is the model type name from LLMProviders.yaml.
	Name string `yaml:"name" json:"name"`

	// Overrides holds provider-specific config overrides (decoded as raw map).
	// Passed to ModelType.CreateModel(overrides).
	Overrides map[string]any `yaml:",inline" json:"overrides,omitempty"`
}

// UnmarshalYAML handles both string and map forms:
//
//	model: claude-opus          → ProfileModel{Name: "claude-opus"}
//	model:
//	  name: claude-opus
//	  thinking:
//	    budget_tokens: 50000    → ProfileModel{Name: "claude-opus", Overrides: {thinking: ...}}
func (m *ProfileModel) UnmarshalYAML(unmarshal func(any) error) error {
	// Try string first
	var s string
	if err := unmarshal(&s); err == nil && s != "" {
		m.Name = s
		return nil
	}

	// Try map
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}

	if name, ok := raw["name"].(string); ok {
		m.Name = name
		delete(raw, "name")
	}
	m.Overrides = raw
	return nil
}

// Merge returns a new AgentProfile where non-nil/non-empty fields from override win.
func (p AgentProfile) Merge(override AgentProfile) AgentProfile {
	out := p
	if override.Model.Name != "" {
		out.Model = override.Model
	}
	if override.MaxTurns != nil {
		out.MaxTurns = override.MaxTurns
	}
	if len(override.Plugins) > 0 {
		out.Plugins = override.Plugins
	}
	if override.SystemPromptPrefix != "" {
		out.SystemPromptPrefix = override.SystemPromptPrefix
	}
	if override.LogModelCalls != nil {
		out.LogModelCalls = override.LogModelCalls
	}
	if override.Compaction != nil {
		out.Compaction = override.Compaction
	}
	return out
}

// MaxTurnsOrDefault returns MaxTurns or the given default if nil.
func (p AgentProfile) MaxTurnsOrDefault(def int) int {
	if p.MaxTurns != nil {
		return *p.MaxTurns
	}
	return def
}

// LLMProfilesConfig is loaded from LLMProfiles.yaml.
// ConfigName strips "Config" → "LLMProfiles" → file LLMProfiles.yaml.
type LLMProfilesConfig struct {
	Default  string                  `yaml:"default"`
	Profiles map[string]AgentProfile `yaml:"profiles"`

	// LogModelCalls is the global default for per-session HTTP-level
	// model-call logging (llm-model-calls.log in the session folder).
	// Individual profiles override it via their own log_model_calls.
	LogModelCalls bool `yaml:"log_model_calls"`
}
