// SPDX-License-Identifier: Apache-2.0

package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// TTSClient manages ElevenLabs API calls and audio playback.
type TTSClient struct {
	model        string
	streaming    bool
	voiceLibrary *TTSVoices
	pronounce    *TTSPronunciations
	player       AudioPlayer
	apiKey       string
	httpClient   *http.Client
}

// TTSClientConfig holds configuration for creating a TTSClient.
// TTSClientConfig holds TTS engine configuration.
// Config file: TTSClient.yaml (TypeName strips nothing — exact match).
type TTSClientConfig struct {
	APIKey    string `yaml:"-"          envkey:"ELEVEN_LABS_API_KEY"`
	Model     string `yaml:"model"      default:"eleven_flash_v2_5"`
	Streaming bool   `yaml:"streaming"  default:"true"`
}

// TTSClientExtension registers TTSClient as a DI service (config: TTSClient.yaml).
var TTSClientExtension = application.DeclareService[*TTSClient, TTSClientConfig](
	func(ctx context.Context, cfg *TTSClientConfig) (*TTSClient, error) {
		voiceLib, err := application.UseService[*TTSVoices](ctx)
		if err != nil {
			return nil, fmt.Errorf("TTSClient: resolve TTSVoices: %w", err)
		}
		pronounce, err := application.UseService[*TTSPronunciations](ctx)
		if err != nil {
			return nil, fmt.Errorf("TTSClient: resolve TTSPronunciations: %w", err)
		}
		return NewTTSClient(*cfg, voiceLib, pronounce, NewPlayer()), nil
	},
)

// NewTTSClient creates a TTSClient with the given config, voice library, pronunciation dictionary, and player.
func NewTTSClient(cfg TTSClientConfig, voiceLib *TTSVoices, pronounce *TTSPronunciations, player AudioPlayer) *TTSClient {
	model := cfg.Model
	if model == "" {
		model = "eleven_multilingual_v2"
	}
	return &TTSClient{
		model:        model,
		streaming:    cfg.Streaming,
		voiceLibrary: voiceLib,
		pronounce:    pronounce,
		player:       player,
		apiKey:       cfg.APIKey,
		httpClient:   &http.Client{},
	}
}

// HasAPIKey returns whether an API key is configured.
func (c *TTSClient) HasAPIKey() bool {
	return c.apiKey != ""
}

// VoiceLib returns the underlying voice library.
func (c *TTSClient) VoiceLib() *TTSVoices {
	return c.voiceLibrary
}

type ttsRequest struct {
	Text          string         `json:"text"`
	ModelID       string         `json:"model_id"`
	LanguageCode  string         `json:"language_code,omitempty"`
	VoiceSettings ttsVoiceParams `json:"voice_settings"`
}

type ttsVoiceParams struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style"`
	Speed           float64 `json:"speed"`
	UseSpeakerBoost bool    `json:"use_speaker_boost"`
}

// Speak generates speech and plays it.
func (c *TTSClient) Speak(text string, voiceID string, settings VoiceSettings, volume float64) error {
	if c.streaming {
		return c.speakStream(text, voiceID, settings, volume)
	}
	return c.speakBuffered(text, voiceID, settings, volume)
}

func (c *TTSClient) speakBuffered(text string, voiceID string, settings VoiceSettings, volume float64) error {
	audio, err := c.GenerateSpeech(text, voiceID, settings)
	if err != nil {
		return err
	}
	return c.player.Play(audio, volume)
}

func (c *TTSClient) speakStream(text string, voiceID string, settings VoiceSettings, volume float64) error {
	resp, err := c.doTTSRequest(text, voiceID, settings)
	if err != nil {
		return err
	}
	slog.Debug("streaming speech", "voice_id", voiceID)
	return c.player.PlayStream(resp.Body, volume)
}

// GenerateSpeech calls the ElevenLabs TTS API and returns the MP3 audio bytes.
func (c *TTSClient) GenerateSpeech(text string, voiceID string, settings VoiceSettings) ([]byte, error) {
	resp, err := c.doTTSRequest(text, voiceID, settings)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read audio response: %w", err)
	}

	slog.Debug("speech generated", "voice_id", voiceID, "audio_bytes", len(audio))
	return audio, nil
}

func (c *TTSClient) doTTSRequest(text string, voiceID string, settings VoiceSettings) (*http.Response, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key not configured")
	}
	if voiceID == "" {
		return nil, fmt.Errorf("no voice ID provided")
	}

	if c.pronounce != nil {
		text = c.pronounce.Apply(text)
	}
	slog.Debug("generating speech", "voice_id", voiceID, "text_length", len(text))

	reqBody := ttsRequest{
		Text:         text,
		ModelID:      c.model,
		LanguageCode: settings.LanguageCode,
		VoiceSettings: ttsVoiceParams{
			Stability:       settings.Stability,
			SimilarityBoost: settings.SimilarityBoost,
			Style:           settings.Style,
			Speed:           settings.Speed,
			UseSpeakerBoost: settings.UseSpeakerBoost,
		},
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s?output_format=mp3_44100_128", voiceID)
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("xi-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ElevenLabs API error %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}
