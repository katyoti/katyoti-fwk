// SPDX-License-Identifier: Apache-2.0

// Package voice contains the TTS engine internals for the notify extension.
package voice

import "gopkg.in/yaml.v3"

// VoiceSettings holds the parameters sent to the ElevenLabs API.
type VoiceSettings struct {
	Stability       float64 `json:"stability" yaml:"stability"`
	SimilarityBoost float64 `json:"similarity_boost" yaml:"similarity_boost"`
	Style           float64 `json:"style" yaml:"style"`
	Speed           float64 `json:"speed" yaml:"speed"`
	UseSpeakerBoost bool    `json:"use_speaker_boost" yaml:"use_speaker_boost"`
	LanguageCode    string  `json:"language_code" yaml:"language_code"`
}

// DefaultVoiceSettings returns VoiceSettings with sensible defaults.
func DefaultVoiceSettings() VoiceSettings {
	return VoiceSettings{
		Stability:       0.5,
		SimilarityBoost: 0.75,
		Style:           0.0,
		Speed:           1.0,
		UseSpeakerBoost: true,
		LanguageCode:    "fr",
	}
}

// VoiceEntry represents a fully configured voice from voices.yaml.
type VoiceEntry struct {
	VoiceID     string        `yaml:"-"`
	VoiceName   string        `yaml:"-"`
	Settings    VoiceSettings `yaml:"-"`
	Volume      float64       `yaml:"volume"`
	Description string        `yaml:"description"`
	Language    string        `yaml:"language"`
	Type        string        `yaml:"type"`
}

// voiceEntryRaw is the intermediate representation for YAML unmarshaling.
type voiceEntryRaw struct {
	VoiceID         string  `yaml:"voice_id"`
	VoiceName       string  `yaml:"voice_name"`
	Stability       float64 `yaml:"stability"`
	SimilarityBoost float64 `yaml:"similarity_boost"`
	Style           float64 `yaml:"style"`
	Speed           float64 `yaml:"speed"`
	UseSpeakerBoost *bool   `yaml:"use_speaker_boost"`
	Volume          float64 `yaml:"volume"`
	Description     string  `yaml:"description"`
	Language        string  `yaml:"language"`
	Type            string  `yaml:"type"`
}

// UnmarshalYAML implements custom YAML parsing.
func (v *VoiceEntry) UnmarshalYAML(value *yaml.Node) error {
	var raw voiceEntryRaw
	if err := value.Decode(&raw); err != nil {
		return err
	}

	v.VoiceID = raw.VoiceID
	v.VoiceName = raw.VoiceName
	v.Volume = raw.Volume
	v.Description = raw.Description
	v.Language = raw.Language
	v.Type = raw.Type

	if v.Language == "" {
		v.Language = "en"
	}
	if v.Volume == 0 {
		v.Volume = 1.0
	}

	useSpeakerBoost := true
	if raw.UseSpeakerBoost != nil {
		useSpeakerBoost = *raw.UseSpeakerBoost
	}

	v.Settings = VoiceSettings{
		Stability:       raw.Stability,
		SimilarityBoost: raw.SimilarityBoost,
		Style:           raw.Style,
		Speed:           raw.Speed,
		UseSpeakerBoost: useSpeakerBoost,
	}

	if v.Settings.Stability == 0 {
		v.Settings.Stability = 0.5
	}
	if v.Settings.SimilarityBoost == 0 {
		v.Settings.SimilarityBoost = 0.75
	}
	if v.Settings.Speed == 0 {
		v.Settings.Speed = 1.0
	}

	return nil
}

// TTSVoicesConfig is the top-level structure of voices.yaml.
type TTSVoicesConfig struct {
	DefaultVoice string                `yaml:"default_voice"`
	Voices       map[string]VoiceEntry `yaml:"voices"`
}

// EmotionalPreset overrides stability and similarity_boost for a given emotion.
type EmotionalPreset struct {
	Stability       float64
	SimilarityBoost float64
}

// TTSPronunciationsConfig holds pronunciation rules loaded from TTSPronunciations.yaml.
type TTSPronunciationsConfig struct {
	Rules []PronunciationRule `yaml:"rules"`
}

// PronunciationRule maps a display term to its phonetic TTS spelling.
type PronunciationRule struct {
	Term     string `yaml:"term"`
	Phonetic string `yaml:"phonetic"`
	Note     string `yaml:"note"`
}

// ResolveResult holds the output of voice resolution.
type ResolveResult struct {
	VoiceID  string
	Settings VoiceSettings
	Volume   float64
}
