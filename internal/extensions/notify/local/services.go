// SPDX-License-Identifier: Apache-2.0

// Package local provides the in-process NotifyService as a PublishedService.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	notifyapi "github.com/katyoti/katyoti-fwk/internal/extensions/notify/api"
	notifycore "github.com/katyoti/katyoti-fwk/internal/extensions/notify/internal/core"
	voiceengine "github.com/katyoti/katyoti-fwk/internal/extensions/notify/internal/voice"
)

// Extension registers NotifyService and its internal dependencies as DI services.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		// Register internal DI services (each declares its own config)
		init.AddExtension(voiceengine.TTSVoicesExtension)
		init.AddExtension(voiceengine.TTSPronunciationsExtension)
		init.AddExtension(voiceengine.TTSClientExtension)

		// Register the published NotifyService
		init.AddExtension(appcore.PublishService[notifyapi.NotifyService, notifycore.NotifyServiceConfig](
			NewNotifyService,
			appcore.WithServiceDef(notifyapi.NotifyServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithHTTPServer(8891),
			// NotifyService.yaml is derived and advertised by the declaration
			// above; these are the FURTHER files this service reads.
			appcore.WithConfigFiles("TTSClient.yaml", "TTSVoices.yaml", "TTSPronunciations.yaml"),
			appcore.WithConfigDirs("config"),
			appcore.WithAPIKeys(appcore.APIKeyDef{
				EnvName:     "ELEVEN_LABS_API_KEY",
				Label:       "ElevenLabs API key",
				Description: "ElevenLabs API key for text-to-speech voice synthesis",
			}),
		))
	}
}

// NewNotifyService creates a NotifyService from DI-resolved config and deps.
func NewNotifyService(ctx context.Context, cfg *notifycore.NotifyServiceConfig) (notifyapi.NotifyService, error) {
	ttsClient, err := application.UseService[*voiceengine.TTSClient](ctx)
	if err != nil {
		return nil, err
	}
	return notifycore.NewNotifyService(cfg, ttsClient)
}
