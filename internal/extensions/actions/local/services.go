// SPDX-License-Identifier: Apache-2.0

// Package local registers ActionService as a published service.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	actionscore "github.com/katyoti/katyoti-fwk/internal/extensions/actions/internal/core"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// Extension registers ActionService as a published service with the
// KTSP container. Wire it in the binary's bootstrap once per process.
func Extension() application.Extension {
	return appcore.PublishService[actionsapi.ActionService, actionscore.Config](
		func(ctx context.Context, cfg *actionscore.Config) (actionsapi.ActionService, error) {
			return actionscore.New(ctx, cfg)
		},
		appcore.WithServiceDef(actionsapi.ActionServiceDef),
		appcore.WithKTSPContainer(),
	)
}

// ExtensionDialing registers ActionService with its method calls sent through
// the sender dial builds instead of a connection to the running hub — for a
// process that already holds a bus, a test's in-memory one. Not published:
// the binary's own bootstrap wires Extension.
func ExtensionDialing(dial func(context.Context) (ktsapi.Sender, error)) application.Extension {
	return appcore.PublishService[actionsapi.ActionService, actionscore.Config](
		func(ctx context.Context, _ *actionscore.Config) (actionsapi.ActionService, error) {
			return actionscore.NewDialing(ctx, dial), nil
		},
		appcore.WithServiceDef(actionsapi.ActionServiceDef),
	)
}
