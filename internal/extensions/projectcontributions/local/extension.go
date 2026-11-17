// SPDX-License-Identifier: Apache-2.0

// Package local wires the project-contributions service into a binary
// (`<provider>:project-contributions/...`) — ktw-agents, whose
// project-contributions pack names it as its panel's source. It needs a
// CWMService and the universe's catalog in the same process.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/internal/core"
)

// Extension publishes the service over KTSP (and its CLI), as the
// `project-contributions` slot: the source of the Contributions panel.
// It also pushes the project's object-backed entries to the hub
// (PushExtension): this is the process that hosts CWM.
func Extension() application.Extension {
	svc := publish.PublishSimpleService[api.ProjectContributionsService](
		func(context.Context) (api.ProjectContributionsService, error) { return core.New(), nil },
		publish.WithServiceDef(api.ServiceDef),
		publish.WithKTSPContainer(),
	)
	push := PushExtension()
	return func(init *application.ApplicationInit) {
		svc(init)
		push(init)
	}
}
