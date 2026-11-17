// SPDX-License-Identifier: Apache-2.0

package api

// SpeakInput holds parameters for direct TTS speech (bypasses profiles/channels).
type SpeakInput struct {
	Text    string  `json:"text"              desc:"Text to speak" required:"true"`
	Voice   string  `json:"voice,omitempty"   desc:"Voice name or ID (empty = default)"`
	Emotion string  `json:"emotion,omitempty" desc:"Emotional preset (e.g., celebration, focused)"`
	Volume  float64 `json:"volume,omitempty"  desc:"Playback volume (0 = voice default, 1.0 = full)"`
}

// SpeakOutput is the result of direct TTS speech.
type SpeakOutput struct {
	Status string `json:"status" desc:"Speech status (ok, skipped, error)"`
}
