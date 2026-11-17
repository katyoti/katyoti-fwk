// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
)

// NotifyService is the public interface for the notification system.
type NotifyService interface {
	// Notify dispatches a notification through the resolved profile's channels.
	Notify(ctx context.Context, in NotifyRequest) (NotifyResponse, error)

	// Speak generates and plays TTS speech directly (bypasses profiles/channels).
	Speak(ctx context.Context, in SpeakInput) (SpeakOutput, error)

	// ListProfiles returns all configured notification profiles.
	ListProfiles(ctx context.Context) (ListProfilesOutput, error)

	// ListChannels returns all configured notification channels.
	ListChannels(ctx context.Context) (ListChannelsOutput, error)

	// ListVoices returns all configured TTS voices.
	ListVoices(ctx context.Context) (ListVoicesOutput, error)
}

// --- Notify ---

// NotifyRequest holds parameters for a notification dispatch.
type NotifyRequest struct {
	// Kind is a hierarchical dot-separated event type (e.g., "task.complete", "agent.error").
	// Used to resolve which profile to use. Supports wildcard matching in config.
	Kind string `json:"kind" desc:"Notification kind (e.g., task.complete, agent.error)"`

	// Profile overrides the kind→profile mapping. If set, kind is ignored for routing.
	Profile string `json:"profile,omitempty" desc:"Profile name (overrides kind→profile mapping)"`

	// Title is the notification title (optional).
	Title string `json:"title,omitempty" desc:"Notification title"`

	// Message is the notification body text.
	Message string `json:"message" desc:"Notification message text" required:"true"`

	// Meta holds extensible metadata (source, level, etc.).
	Meta NotifyMeta `json:"meta,omitempty" desc:"Extensible notification metadata"`
}

// NotifyMeta holds optional metadata for a notification.
// All fields are optional — extend freely without breaking existing callers.
type NotifyMeta struct {
	Source string `json:"source,omitempty" desc:"Notification source (e.g., Agent(Albert), ktw-agents)"`
	Level  string `json:"level,omitempty"  desc:"Severity level (info, warn, error)"`

	// Extra holds arbitrary key-value metadata for future extension.
	Extra map[string]string `json:"extra,omitempty" desc:"Arbitrary key-value metadata"`
}

// NotifyResponse holds the result of a notification dispatch.
type NotifyResponse struct {
	// Profile is the resolved profile name that was used.
	Profile string `json:"profile" desc:"Resolved profile name"`

	// Channels lists the result for each channel in the profile.
	Channels []ChannelResult `json:"channels" desc:"Per-channel delivery results"`
}

// ChannelResult holds the delivery result for a single channel.
type ChannelResult struct {
	Name   string `json:"name"             desc:"Channel name"`
	Status string `json:"status"           desc:"Delivery status (ok, error, skipped)"`
	Error  string `json:"error,omitempty"  desc:"Error message if delivery failed"`
}

// --- Speak (defined in voice.go) ---
// SpeakInput/SpeakOutput live in voice.go alongside other voice types.

// --- List ---

// ListProfilesOutput lists all configured notification profiles.
type ListProfilesOutput struct {
	Profiles []ProfileInfo `json:"profiles" desc:"Configured profiles"`
}

// ProfileInfo describes a notification profile.
type ProfileInfo struct {
	Name     string              `json:"name"     desc:"Profile name"`
	Channels []ProfileChannelRef `json:"channels" desc:"Channels used by this profile"`
}

// ProfileChannelRef is a reference to a channel within a profile, with optional overrides.
type ProfileChannelRef struct {
	Name   string  `json:"name"             desc:"Channel name"`
	Volume float64 `json:"volume,omitempty" desc:"Volume override for this channel in this profile"`
}

// ListChannelsOutput lists all configured notification channels.
type ListChannelsOutput struct {
	Channels []ChannelInfo `json:"channels" desc:"Configured channels"`
}

// ChannelInfo describes a configured notification channel.
type ChannelInfo struct {
	Name string `json:"name" desc:"Channel instance name (e.g., albert, desktop)"`
	Type string `json:"type" desc:"Channel strategy type (e.g., voice, desktop, webhook)"`
}

type ListVoicesOutput struct {
	Voices []VoiceInfo `json:"voices" desc:"Configured voices"`
}

type VoiceInfo struct {
	Name        string `json:"name" desc:"Voice name"`
	ID          string `json:"id" desc:"Voice ID"`
	Description string `json:"description" desc:"Voice description"`
}
