// SPDX-License-Identifier: Apache-2.0

// Package local wires the goals service into a binary.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/goals/api"
	goalscore "github.com/katyoti/katyoti-fwk/internal/extensions/goals/internal/core"
)

// Extension publishes the GoalsService. It needs a CWMService in the same
// process: every verb is an Object API call.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.GoalsService](
		func(context.Context) (api.GoalsService, error) { return goalscore.New(), nil },
		publish.WithServiceDef(api.GoalsServiceDef),
		publish.WithKTSPContainer(),
	)
}
