// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// UniverseUnreachable makes the universe reach fail - the hub down - until
// the returned func restores the catalog read.
func UniverseUnreachable() (restore func()) {
	prev := queryProfileSpecs
	queryProfileSpecs = func(ctx context.Context, q mbapi.Q) ([]mbapi.Attributed[permrules.ProfileSpec], error) {
		if q.Reach == mbapi.ReachUniverse {
			return nil, errors.New("hub down")
		}
		return prev(ctx, q)
	}
	return func() { queryProfileSpecs = prev }
}
