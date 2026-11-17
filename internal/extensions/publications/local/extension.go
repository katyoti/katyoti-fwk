// SPDX-License-Identifier: Apache-2.0

// Package local wires the PublishersService into a binary
// (`<provider>:publishers/<Verb>`). The kind it reads is declared by
// contrib.DeclareKinds and the API's data types by contrib.Pack.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
	publicationscore "github.com/katyoti/katyoti-fwk/internal/extensions/publications/internal/core"
)

// Extension publishes the PublishersService over KTSP.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.PublishersService](
		func(context.Context) (api.PublishersService, error) { return publicationscore.New(), nil },
		publish.WithServiceDef(api.PublishersServiceDef),
		publish.WithKTSPContainer(),
	)
}
