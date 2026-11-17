// SPDX-License-Identifier: Apache-2.0

// Package local wires the challenges service into a binary.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/challenges/api"
	challengescore "github.com/katyoti/katyoti-fwk/internal/extensions/challenges/internal/core"
)

// Extension publishes the ChallengesService. It needs a CWMService in the
// process (in-process or the bus client): every read is an Object API call.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.ChallengesService](
		func(context.Context) (api.ChallengesService, error) { return challengescore.New(), nil },
		publish.WithServiceDef(api.ChallengesServiceDef),
		publish.WithKTSPContainer(),
	)
}
