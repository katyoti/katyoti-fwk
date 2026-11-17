// SPDX-License-Identifier: Apache-2.0

// Package local wires the acceptance service into a binary
// (`<provider>:acceptance/verify`). It needs a CWMService — local or the bus
// client — because every read and write is an Object API call.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/api"
	acceptancecore "github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/internal/core"
)

// Extension publishes the AcceptanceService over KTSP.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.AcceptanceService](
		func(context.Context) (api.AcceptanceService, error) { return acceptancecore.New(), nil },
		publish.WithServiceDef(api.AcceptanceServiceDef),
		publish.WithKTSPContainer(),
	)
}
