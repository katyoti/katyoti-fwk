// SPDX-License-Identifier: Apache-2.0

// Package local registers EventService as a published service.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	eventscore "github.com/katyoti/katyoti-fwk/internal/extensions/events/internal/core"
)

// Extension registers EventService as a published service with the
// KTSP container. The facade is bus-agnostic at registration time —
// MessageBus is resolved lazily on each Subscribe / Publish call so
// the registration order between EventService and the bus binding
// doesn't matter.
func Extension() application.Extension {
	return appcore.PublishService[eventsapi.EventService, eventscore.Config](
		func(ctx context.Context, cfg *eventscore.Config) (eventsapi.EventService, error) {
			return eventscore.New(ctx, cfg)
		},
		appcore.WithServiceDef(eventsapi.EventServiceDef),
		appcore.WithKTSPContainer(),
	)
}
