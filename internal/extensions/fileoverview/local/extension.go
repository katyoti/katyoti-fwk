// SPDX-License-Identifier: Apache-2.0

// Package local wires the file-overview service into a binary.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/fileoverview/api"
	focore "github.com/katyoti/katyoti-fwk/internal/extensions/fileoverview/internal/core"
)

// Extension publishes the FileOverviewService (ktw-agents:file-overview/read),
// the source of the file-overview panel. It needs a CWMService in the same
// process.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.FileOverviewService](
		func(context.Context) (api.FileOverviewService, error) { return focore.New(), nil },
		publish.WithServiceDef(api.FileOverviewServiceDef),
		publish.WithKTSPContainer(),
	)
}
