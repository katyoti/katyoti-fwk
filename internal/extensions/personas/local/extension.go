// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	personasapi "github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
	personascore "github.com/katyoti/katyoti-fwk/internal/extensions/personas/internal/core"
)

// Extension publishes PersonaService (persona.Resolve / List / Spawn).
// Stateless: the catalog and the TaskService are resolved from ctx on
// every call, so registration order does not matter.
func Extension() application.Extension {
	return appcore.PublishSimpleService[personasapi.PersonaService](
		func(context.Context) (personasapi.PersonaService, error) {
			return personascore.New(), nil
		},
		appcore.WithServiceDef(personasapi.PersonaServiceDef),
		appcore.WithKTSPContainer(),
	)
}
