// SPDX-License-Identifier: Apache-2.0

// Package local wires the universe-view service into a binary
// (`<provider>:universe-view/...`) - ktw-agents, which serves every universe's
// GUI. It reads the universe's catalog and Univers.yaml in the same process.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/universeview/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/universeview/internal/core"
)

// Extension publishes the service over KTSP (and its CLI), as the
// `universe-view` slot.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.UniverseViewService](
		func(context.Context) (api.UniverseViewService, error) { return core.New(), nil },
		publish.WithServiceDef(api.ServiceDef),
		publish.WithKTSPContainer(),
	)
}
