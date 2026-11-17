// SPDX-License-Identifier: Apache-2.0

// Package local provides the in-process DocService as a PublishedService.
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	docscore "github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/core"
)

// Extension registers DocService as a published service with KTSP + HTTP capabilities.
func Extension() application.Extension {
	return appcore.PublishService[api.DocService, docscore.DocServiceConfig](
		docscore.NewDocService,
		appcore.WithServiceDef(api.DocServiceDef),
		appcore.WithKTSPContainer(),
		appcore.WithHTTPServer(8890),
		appcore.WithConfigDirs("config"),
		appcore.WithTools(ToolDefs()...),
	)
}
