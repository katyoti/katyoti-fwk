// SPDX-License-Identifier: Apache-2.0

package voice

import (
	"regexp"
	"strings"
)

// EmotionManager manages emotional presets and marker extraction.
type EmotionManager struct {
	presets  map[string]EmotionalPreset
	emojiMap map[string]string
	pattern  *regexp.Regexp
}

// NewEmotionManager creates an EmotionManager with default presets and emoji mappings.
func NewEmotionManager() *EmotionManager {
	presets := map[string]EmotionalPreset{
		"excited":       {Stability: 0.7, SimilarityBoost: 0.9},
		"celebration":   {Stability: 0.65, SimilarityBoost: 0.85},
		"insight":       {Stability: 0.55, SimilarityBoost: 0.8},
		"creative":      {Stability: 0.5, SimilarityBoost: 0.75},
		"success":       {Stability: 0.6, SimilarityBoost: 0.8},
		"progress":      {Stability: 0.55, SimilarityBoost: 0.75},
		"investigating": {Stability: 0.6, SimilarityBoost: 0.85},
		"debugging":     {Stability: 0.55, SimilarityBoost: 0.8},
		"learning":      {Stability: 0.5, SimilarityBoost: 0.75},
		"pondering":     {Stability: 0.65, SimilarityBoost: 0.8},
		"focused":       {Stability: 0.7, SimilarityBoost: 0.85},
		"caution":       {Stability: 0.4, SimilarityBoost: 0.6},
		"urgent":        {Stability: 0.3, SimilarityBoost: 0.9},
	}

	emojiMap := map[string]string{
		"⚠️": "caution", "⚠": "caution",
		"✨": "success", "🎉": "celebration",
		"🎨": "creative", "🐛": "debugging",
		"💡": "insight", "💥": "excited",
		"📈": "progress", "📚": "learning",
		"🔍": "investigating", "🚨": "urgent",
		"🤔": "pondering", "🎯": "focused",
	}

	emojis := make([]string, 0, len(emojiMap))
	seen := make(map[string]bool)
	for e := range emojiMap {
		escaped := regexp.QuoteMeta(e)
		if !seen[escaped] {
			emojis = append(emojis, escaped)
			seen[escaped] = true
		}
	}

	patternStr := `\[(` + strings.Join(emojis, "|") + `)\s+(\w+)\]`
	pattern := regexp.MustCompile(patternStr)

	return &EmotionManager{
		presets:  presets,
		emojiMap: emojiMap,
		pattern:  pattern,
	}
}

// ApplyPreset returns a copy of settings with emotional overrides applied.
func (em *EmotionManager) ApplyPreset(settings VoiceSettings, emotion string) VoiceSettings {
	preset, ok := em.presets[emotion]
	if !ok {
		return settings
	}
	settings.Stability = preset.Stability
	settings.SimilarityBoost = preset.SimilarityBoost
	return settings
}

// ExtractMarker searches for an emotional marker in the message.
func (em *EmotionManager) ExtractMarker(message string) (string, string) {
	matches := em.pattern.FindStringSubmatchIndex(message)
	if matches == nil {
		return message, ""
	}

	emoji := message[matches[2]:matches[3]]
	emotionWord := message[matches[4]:matches[5]]

	mappedEmotion, ok := em.emojiMap[emoji]
	if !ok || mappedEmotion != emotionWord {
		return message, ""
	}

	cleaned := message[:matches[0]] + message[matches[1]:]
	cleaned = strings.TrimSpace(cleaned)

	return cleaned, emotionWord
}
