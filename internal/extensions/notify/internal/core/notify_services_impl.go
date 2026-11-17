// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"

	notifyapi "github.com/katyoti/katyoti-fwk/internal/extensions/notify/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/notify/internal/voice"
)

// notifyService implements api.NotifyService.
type notifyService struct {
	cfg         *NotifyServiceConfig
	channels    map[string]Channel
	voiceEngine *voice.TTSClient
}

// NewNotifyService creates a NotifyService from config and voice engine.
func NewNotifyService(cfg *NotifyServiceConfig, voiceEngine *voice.TTSClient) (notifyapi.NotifyService, error) {
	channels := buildChannels(cfg, voiceEngine)
	return &notifyService{
		cfg:         cfg,
		channels:    channels,
		voiceEngine: voiceEngine,
	}, nil
}

// Notify dispatches a notification through the resolved profile's channels.
func (s *notifyService) Notify(ctx context.Context, in notifyapi.NotifyRequest) (notifyapi.NotifyResponse, error) {
	// Resolve profile: explicit > kind-based > default
	profileName := in.Profile
	if profileName == "" {
		profileName = ResolveProfile(in.Kind, s.cfg.Kinds, s.cfg.DefaultProfile)
	}

	profile, ok := s.cfg.Profiles[profileName]
	if !ok {
		return notifyapi.NotifyResponse{}, fmt.Errorf("profile %q not found", profileName)
	}

	// Dispatch to each channel in the profile
	results := make([]notifyapi.ChannelResult, 0, len(profile.Channels))
	for _, chRef := range profile.Channels {
		ch, exists := s.channels[chRef.Name]
		if !exists {
			slog.Warn("profile references unknown channel", "profile", profileName, "channel", chRef.Name)
			results = append(results, notifyapi.ChannelResult{
				Name:   chRef.Name,
				Status: "skipped",
				Error:  "channel not found",
			})
			continue
		}

		err := ch.Send(ctx, in.Title, in.Message, chRef.Volume)
		if err != nil {
			slog.Error("channel send failed", "channel", chRef.Name, "error", err)
			results = append(results, notifyapi.ChannelResult{
				Name:   chRef.Name,
				Status: "error",
				Error:  err.Error(),
			})
		} else {
			results = append(results, notifyapi.ChannelResult{
				Name:   chRef.Name,
				Status: "ok",
			})
		}
	}

	return notifyapi.NotifyResponse{
		Profile:  profileName,
		Channels: results,
	}, nil
}

// Speak generates and plays TTS speech directly, bypassing profiles/channels.
func (s *notifyService) Speak(ctx context.Context, in notifyapi.SpeakInput) (notifyapi.SpeakOutput, error) {
	if s.voiceEngine == nil {
		return notifyapi.SpeakOutput{Status: "skipped"}, fmt.Errorf("voice engine not available")
	}
	if !s.voiceEngine.HasAPIKey() {
		return notifyapi.SpeakOutput{Status: "skipped"}, fmt.Errorf("ELEVEN_LABS_API_KEY not set")
	}

	resolved := s.voiceEngine.VoiceLib().Resolve(in.Voice, in.Emotion)

	vol := resolved.Volume
	if in.Volume > 0 {
		vol = in.Volume
	}

	if err := s.voiceEngine.Speak(in.Text, resolved.VoiceID, resolved.Settings, vol); err != nil {
		return notifyapi.SpeakOutput{Status: "error"}, err
	}
	return notifyapi.SpeakOutput{Status: "ok"}, nil
}

// ListProfiles returns all configured notification profiles.
func (s *notifyService) ListProfiles(_ context.Context) (notifyapi.ListProfilesOutput, error) {
	profiles := make([]notifyapi.ProfileInfo, 0, len(s.cfg.Profiles))
	for name, p := range s.cfg.Profiles {
		channels := make([]notifyapi.ProfileChannelRef, len(p.Channels))
		for i, ch := range p.Channels {
			channels[i] = notifyapi.ProfileChannelRef{
				Name:   ch.Name,
				Volume: ch.Volume,
			}
		}
		profiles = append(profiles, notifyapi.ProfileInfo{
			Name:     name,
			Channels: channels,
		})
	}
	return notifyapi.ListProfilesOutput{Profiles: profiles}, nil
}

// ListChannels returns all configured notification channels.
func (s *notifyService) ListChannels(_ context.Context) (notifyapi.ListChannelsOutput, error) {
	channels := make([]notifyapi.ChannelInfo, 0, len(s.cfg.Channels))
	for name, ch := range s.cfg.Channels {
		channels = append(channels, notifyapi.ChannelInfo{
			Name: name,
			Type: ch.Type,
		})
	}
	return notifyapi.ListChannelsOutput{Channels: channels}, nil
}

// ListVoices returns all configured TTS voices.
func (s *notifyService) ListVoices(_ context.Context) (notifyapi.ListVoicesOutput, error) {

	voiceLib := s.voiceEngine.VoiceLib()
	voices := make([]notifyapi.VoiceInfo, 0, len(voiceLib.VoiceNames()))
	for _, entry := range voiceLib.ListVoices() {
		voices = append(voices, notifyapi.VoiceInfo{
			Name:        entry.VoiceName,
			ID:          entry.VoiceID,
			Description: entry.Description,
		})
	}
	return notifyapi.ListVoicesOutput{Voices: voices}, nil
}
