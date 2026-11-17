// SPDX-License-Identifier: Apache-2.0

package room

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// Extension registers the Room as a singleton application-scoped service.
// The data directory is taken from Env().DataDir(); the KTSP sender is nil
// by default (bridge disabled until a transport task wires it in).
var Extension = application.DeclareSimpleService[*Room](
	func(ctx context.Context) (*Room, error) {
		dataDir := application.Env().DataDir()
		return NewRoom(dataDir, nil), nil
	},
)
