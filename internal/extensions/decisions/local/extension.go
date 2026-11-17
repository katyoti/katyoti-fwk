// SPDX-License-Identifier: Apache-2.0

// Package local wires the decisions service into a binary.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	decisionscore "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/internal/core"
)

// Extension publishes the DecisionsService. It needs a CWMService in the same
// process: every verb is an Object API call.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.DecisionsService](
		func(context.Context) (api.DecisionsService, error) { return decisionscore.New(), nil },
		publish.WithServiceDef(api.DecisionsServiceDef),
		publish.WithKTSPContainer(),
	)
}
