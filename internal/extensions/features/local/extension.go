// SPDX-License-Identifier: Apache-2.0

// Package local wires the features service into a binary.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	featurescore "github.com/katyoti/katyoti-fwk/internal/extensions/features/internal/core"
)

// Extension publishes the FeaturesService. It needs a CWMService in the
// process (in-process or the bus client): every read is an Object API call.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.FeaturesService](
		func(context.Context) (api.FeaturesService, error) { return featurescore.New(), nil },
		publish.WithServiceDef(api.FeaturesServiceDef),
		publish.WithKTSPContainer(),
	)
}
