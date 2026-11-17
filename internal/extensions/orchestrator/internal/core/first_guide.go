// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// guideKind is the kind a first guide is.
const guideKind = "guide-recipe"

// firstGuideOf refuses a first guide that is not a guide of one of the
// universe's sidecars or of the hub: a guide-recipe URI whose provider is a
// chosen sidecar, or ktw-hub.
// What that sidecar publishes is its manifest's to say once the universe
// runs; the hub, creating it, holds the membership.
func firstGuideOf(in api.CreateUniverseInput) error {
	if in.FirstGuide == "" {
		return nil
	}
	u, err := contribapi.ParseResourceURI(in.FirstGuide)
	if err != nil || u.Type != guideKind || u.ID == "" {
		return fmt.Errorf("first guide %q is no guide: want <sidecar>:/%s/<id>", in.FirstGuide, guideKind)
	}
	// The hub's own guides - the shell recipes every universe has - may be
	// one too: an ArchiFactory universe starts on « Utiliser KaTyoti »
	//
	if u.Provider == projectfile.HubToken {
		return nil
	}
	for _, s := range in.Sidecars {
		if s.Name == u.Provider {
			return nil
		}
	}
	return fmt.Errorf("first guide %q is not among the universe's sidecars' guides: %s is not one of its sidecars", in.FirstGuide, u.Provider)
}
