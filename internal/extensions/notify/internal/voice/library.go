// SPDX-License-Identifier: Apache-2.0

package voice

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// TTSVoicesExtension registers TTSVoices as a DI service (config: TTSVoices.yaml).
var TTSVoicesExtension = application.DeclareService[*TTSVoices, TTSVoicesConfig](
	func(_ context.Context, cfg *TTSVoicesConfig) (*TTSVoices, error) {
		return NewTTSVoices(*cfg), nil
	},
)

// TTSVoices manages voice lookup and resolution.
type TTSVoices struct {
	byName         map[string]VoiceEntry
	byID           map[string]VoiceEntry
	defaultVoiceID string
	emotionManager *EmotionManager
}

// NewTTSVoices creates a TTSVoices from pre-loaded config.
// The caller is responsible for loading TTSVoicesConfig (via DI or config service).
func NewTTSVoices(cfg TTSVoicesConfig) *TTSVoices {
	vl := &TTSVoices{
		byName:         make(map[string]VoiceEntry),
		byID:           make(map[string]VoiceEntry),
		emotionManager: NewEmotionManager(),
	}

	for name, entry := range cfg.Voices {
		vl.byName[name] = entry
		if entry.VoiceID != "" {
			vl.byID[entry.VoiceID] = entry
		}
	}

	vl.defaultVoiceID = vl.resolveDefault(cfg.DefaultVoice)
	return vl
}

func (vl *TTSVoices) resolveDefault(defaultName string) string {
	if entry, ok := vl.byName[defaultName]; ok {
		return entry.VoiceID
	}
	for _, entry := range vl.byID {
		return entry.VoiceID
	}
	return ""
}

// DefaultVoiceID returns the resolved default voice ID.
func (vl *TTSVoices) DefaultVoiceID() string {
	return vl.defaultVoiceID
}

// VoiceNames returns all configured voice names.
func (vl *TTSVoices) VoiceNames() []string {
	names := make([]string, 0, len(vl.byName))
	for name := range vl.byName {
		names = append(names, name)
	}
	return names
}

func (vl *TTSVoices) ListVoices() []VoiceEntry {
	voices := make([]VoiceEntry, 0, len(vl.byName))
	for _, entry := range vl.byName {
		voices = append(voices, entry)
	}
	return voices
}

// EmotionMgr returns the EmotionManager instance.
func (vl *TTSVoices) EmotionMgr() *EmotionManager {
	return vl.emotionManager
}

// Resolve resolves voice parameters with fallback chain.
func (vl *TTSVoices) Resolve(voiceID string, emotion string) ResolveResult {
	var settings VoiceSettings
	var volume float64 = 1.0

	// If voiceID is a name, resolve to actual ID
	if entry, ok := vl.byName[voiceID]; ok {
		voiceID = entry.VoiceID
	}

	// Look up by voice_id
	if entry, ok := vl.byID[voiceID]; ok {
		settings = entry.Settings
		volume = entry.Volume
	} else {
		// Try first available voice
		for _, e := range vl.byID {
			settings = e.Settings
			volume = e.Volume
			if voiceID == "" {
				voiceID = e.VoiceID
			}
			break
		}
		if voiceID == "" {
			settings = DefaultVoiceSettings()
		}
	}

	if voiceID == "" {
		voiceID = vl.defaultVoiceID
	}

	// Apply emotional preset
	if emotion != "" {
		settings = vl.emotionManager.ApplyPreset(settings, emotion)
	}

	return ResolveResult{
		VoiceID:  voiceID,
		Settings: settings,
		Volume:   volume,
	}
}
