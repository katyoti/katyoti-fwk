// SPDX-License-Identifier: Apache-2.0

package api

// VoicePluginConfig holds TTS voice configuration loaded from voices.yaml.
// Used by the notify plugin to configure TTS behavior.
type VoicePluginConfig struct {
	Enabled          bool          `yaml:"enabled"`
	DefaultVoice     string        `yaml:"default_voice"`
	TTSModel         string        `yaml:"tts_model"`
	MaxMessageLength int           `yaml:"max_message_length"`
	Streaming        bool          `yaml:"streaming"`
	Voices           []SimpleVoice `yaml:"voices"`
	APIKey           string        `yaml:"-"` // From .env (ELEVEN_LABS_API_KEY), never in YAML
}

// SimpleVoice defines a named TTS voice in the plugin config.
type SimpleVoice struct {
	Name      string  `yaml:"name"`
	VoiceID   string  `yaml:"voice_id"`
	Language  string  `yaml:"language"`
	Stability float64 `yaml:"stability"`
	Clarity   float64 `yaml:"clarity"`
}
