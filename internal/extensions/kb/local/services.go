// SPDX-License-Identifier: Apache-2.0

// Package local provides the in-process KBService as a PublishedService.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	kbcore "github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/embedding"
)

// Extension registers KBService as a published service with KTSP + HTTP, and
// the in-process EmbeddingProfiles service (EmbeddingProfiles.yaml) it embeds
// through.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishService[kbapi.EmbeddingProfiles, kbapi.EmbeddingProfilesConfig](
			newEmbeddingProfiles,
			appcore.WithServiceDef(kbapi.EmbeddingProfilesDef),
			appcore.WithLocal(),
			appcore.WithConfigDirs("config", "kb"),
		))
		init.AddExtension(appcore.PublishService[kbapi.KBService, kbcore.KBServiceConfig](
			newKBService,
			appcore.WithServiceDef(kbapi.KBServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithHTTPServer(8896),
			appcore.WithConfigDirs("config", "kb"),
			appcore.WithTools(ToolDefs()...),
			appcore.WithAPIKeys(APIKeyDefs()...),
		))
	}
}

// newKBService creates a KBService from config and DI-resolved deps.
func newKBService(ctx context.Context, cfg *kbcore.KBServiceConfig) (kbapi.KBService, error) {
	return kbcore.NewKBService(cfg)
}

// newEmbeddingProfiles validates EmbeddingProfiles.yaml and serves its profiles.
func newEmbeddingProfiles(ctx context.Context, cfg *kbapi.EmbeddingProfilesConfig) (kbapi.EmbeddingProfiles, error) {
	return embedding.NewEmbeddingProfiles(cfg)
}
