// SPDX-License-Identifier: Apache-2.0

// Package local wires the reviews service into a binary
// (`<provider>:reviews/...`) — ktw-agents, whose reviews pack
// (internal/extensions/reviews/contrib) names its methods as its panels'
// sources. It needs a CWMService in the same process.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
	reviewscore "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/internal/core"
)

// Extension publishes the reviews service over KTSP (and its CLI), as the
// `reviews` slot: the sources of the Reviews and Review panels.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.ReviewsService](
		func(context.Context) (api.ReviewsService, error) { return reviewscore.New(), nil },
		publish.WithServiceDef(api.ReviewsServiceDef),
		publish.WithKTSPContainer(),
	)
}
