// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcore "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// init installs the KTSP slot introspector the manifest builder asks for.
//
// In init rather than in Extension(): the builder is reachable without going
// through Extension (the resource service constructs its own, and the census
// builds one directly), and a seam filled in only some of those paths would
// make a binary's manifest depend on how it was constructed.
//
// Importing this package is what declares "this binary speaks KTSP" — which is
// already true of every caller, since local/ is the wiring layer.
func init() { manifestcore.SlotIntrospector = introspectSlots }

// introspectSlots reads the local KTSP endpoint and maps it onto the manifest's
// slot surface. Returns nothing when no endpoint is registered — not every
// binary has a KTSP container (ktw-hub itself doesn't), and that is not an
// error.
//
// ktsp.GenerateManifest rather than Endpoint.Info directly, so SlotTypes are
// auto-derived from the typed slots, internal slots are filtered out, and typed
// slots' inline Methods are dropped (the SlotType contract is the single source
// of truth for that). This matches what `<binary> ktsp describe --json` exposes.
func introspectSlots(ctx context.Context) ([]mbapi.SlotType, []mbapi.Slot) {
	endpoint, err := application.UseNamedService[*ktsp.Endpoint](ctx, "default")
	if err != nil || endpoint == nil {
		return nil, nil
	}
	return mapKTSPInfo(ktsp.GenerateManifest(endpoint))
}
