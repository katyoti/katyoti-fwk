// SPDX-License-Identifier: Apache-2.0

// Package core holds the internal channel infrastructure for the notify extension.
package core

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/extensions/notify/internal/voice"
)

// Channel sends a notification through a specific medium.
type Channel interface {
	Send(ctx context.Context, title, message string, volume float64) error
}

// --- Voice channel ---

// voiceChannel implements Channel using the TTS engine.
type voiceChannel struct {
	engine    *voice.TTSClient
	voiceName string // voice name from channel config (resolved via TTSVoices)
	emotion   string
	volume    float64 // default volume (overridden by caller if > 0)
}

func (c *voiceChannel) Send(ctx context.Context, title, message string, volume float64) error {
	if !c.engine.HasAPIKey() {
		return fmt.Errorf("ELEVEN_LABS_API_KEY not set — cannot generate speech")
	}

	vol := c.volume
	if volume > 0 {
		vol = volume
	}

	resolved := c.engine.VoiceLib().Resolve(c.voiceName, c.emotion)
	return c.engine.Speak(message, resolved.VoiceID, resolved.Settings, vol)
}

// --- Desktop channel ---

// desktopChannel implements Channel using the host OS notification centre.
// The delivery mechanism is per-platform — see desktop_<goos>.go.
type desktopChannel struct{}

func (c *desktopChannel) Send(_ context.Context, title, message string, _ float64) error {
	if title == "" {
		title = "KTW Notification"
	}
	if message == "" {
		message = "Task completed"
	}
	if err := pushDesktopNotification(title, message); err != nil {
		slog.Warn("desktop notification failed", "error", err)
	}
	return nil
}

// --- Builder ---

// buildChannels creates all Channel instances from config.
func buildChannels(cfg *NotifyServiceConfig, voiceEngine *voice.TTSClient) map[string]Channel {
	channels := make(map[string]Channel, len(cfg.Channels))
	for name, chCfg := range cfg.Channels {
		ch, err := buildChannel(name, chCfg, voiceEngine)
		if err != nil {
			slog.Warn("skip channel", "name", name, "error", err)
			continue
		}
		channels[name] = ch
	}
	return channels
}

func buildChannel(name string, cfg ChannelConfig, voiceEngine *voice.TTSClient) (Channel, error) {
	switch cfg.Type {
	case "voice":
		if voiceEngine == nil {
			return nil, fmt.Errorf("voice engine not available")
		}
		return &voiceChannel{
			engine:    voiceEngine,
			voiceName: cfg.Voice,
			emotion:   cfg.Emotion,
			volume:    cfg.Volume,
		}, nil

	case "desktop":
		return &desktopChannel{}, nil

	default:
		return nil, fmt.Errorf("unknown channel type %q", cfg.Type)
	}
}
