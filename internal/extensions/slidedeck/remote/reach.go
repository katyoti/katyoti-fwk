// SPDX-License-Identifier: Apache-2.0

// Package remote builds the query that reaches the universe's contributions
// from wherever the slide-deck engine runs. In ktw-agents the in-process
// catalog is the universe's; in a sidecar (ktw-docs) it holds only the
// sidecar's own entries, so a universe query read there saw only katyoti and
// called every other theme unknown. A sidecar reads the universe's catalog
// over the bus instead.
package remote

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
)

// Universe is a universe reach that reads the universe's catalog: over the
// bus when this binary has a bus-backed sender, else the catalog in process.
func Universe(ctx context.Context) mbapi.Q {
	q := mbapi.Q{Reach: mbapi.ReachUniverse}
	if p, err := application.UseService[busclient.SenderProvider](ctx); err == nil && p != nil {
		if s, err := p.Sender(ctx); err == nil && s != nil {
			q.Catalog = manifestremote.New(s)
		}
	}
	return q
}
