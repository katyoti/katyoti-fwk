// SPDX-License-Identifier: Apache-2.0

// Package local wires the use-cases service into a binary.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/usecases/api"
	usecasescore "github.com/katyoti/katyoti-fwk/internal/extensions/usecases/internal/core"
)

// Extension publishes the UseCasesService. It needs a CWMService in the same
// process: every verb is an Object API call.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.UseCasesService](
		func(context.Context) (api.UseCasesService, error) { return usecasescore.New(), nil },
		publish.WithServiceDef(api.UseCasesServiceDef),
		publish.WithKTSPContainer(),
	)
}
