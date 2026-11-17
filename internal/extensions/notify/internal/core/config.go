// SPDX-License-Identifier: Apache-2.0

package core

// NotifyServiceConfig is the main configuration for the notification service.
// Config file: NotifyService.yaml (TypeName strips "Config" suffix).
type NotifyServiceConfig struct {
	// Server
	ServerHost string `yaml:"server_host" default:"127.0.0.1"`
	ServerPort int    `yaml:"server_port" default:"8891"`

	// Kind → profile mapping (hierarchical, supports wildcards)
	// e.g., "task.complete": "celebrate", "agent.*": "default"
	Kinds map[string]string `yaml:"kinds"`

	// Channel definitions (strategy instances)
	// e.g., "albert": {type: "voice", voice: "chris", volume: 0.8}
	Channels map[string]ChannelConfig `yaml:"channels"`

	// Profile definitions
	Profiles map[string]ProfileConfig `yaml:"profiles"`

	// Default profile when no kind matches
	DefaultProfile string `yaml:"default_profile" default:"default"`
}

// ChannelConfig defines a channel instance.
type ChannelConfig struct {
	Type string `yaml:"type"` // strategy type: "voice", "desktop", "webhook", etc.

	// Voice-specific settings
	Voice   string  `yaml:"voice,omitempty"`
	Emotion string  `yaml:"emotion,omitempty"`
	Volume  float64 `yaml:"volume,omitempty"`

	// Desktop-specific settings
	MaxLength int `yaml:"max_length,omitempty"`

	// Webhook-specific settings (future)
	URL     string            `yaml:"url,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// ProfileConfig defines a notification profile.
type ProfileConfig struct {
	Channels []ProfileChannelRef `yaml:"channels"`
}

// ProfileChannelRef references a channel within a profile, with optional overrides.
type ProfileChannelRef struct {
	Name   string  `yaml:"name"`
	Volume float64 `yaml:"volume,omitempty"` // override channel's default volume
}
