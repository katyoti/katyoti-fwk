// SPDX-License-Identifier: Apache-2.0

// Package api defines the public contract for the notification service.
package api

import (
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// NotifyServiceDef describes the NotifyService interface.
var NotifyServiceDef = meta.ServiceDef{
	Name:  "notify",
	Short: "Notification service with voice, desktop, and extensible channels",
	Long: `Dispatches notifications through configurable channels (voice TTS, desktop toast,
webhook, etc.). Notifications are routed by kind (hierarchical, dot-separated) to
profiles that define which channels to use. Channels are strategy instances with
per-channel configuration.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Notify": meta.MethodLong(
			"Send a notification through the resolved profile's channels",
			`Routes a notification by kind (e.g., "task.complete", "agent.error") to a profile.
The profile defines which channels to use. Each channel delivers the notification
in its own way (TTS voice, desktop toast, webhook, etc.). Kind matching supports
wildcards: "task.*" matches "task.start", "task.complete", etc.`,
		),
		"Speak": meta.MethodLong(
			"Generate and play TTS speech directly",
			`Bypasses the profile/channel system and speaks text directly using the TTS engine.
Useful for agent plugins that want voice feedback without going through notification routing.`,
		),
		"ListProfiles": {
			Short:          "List all configured notification profiles",
			RenderTemplate: listProfilesTmpl,
		},
		"ListChannels": {
			Short:          "List all configured notification channels",
			RenderTemplate: listChannelsTmpl,
		},
		"ListVoices": {
			Short:          "List all configured TTS voices",
			RenderTemplate: listVoicesTmpl,
		},
	},
}
