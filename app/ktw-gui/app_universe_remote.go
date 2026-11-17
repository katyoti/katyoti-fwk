// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	orchremote "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/remote"
)

// orchSvc returns a KTSP-backed OrchestratorService: a PURE CLIENT of the universe-
// registry service served in the ADMIN universe (ktw-univers's orchestrator slot —
// (modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)). The GUI writes NO registry state itself; the
// admin sidecar is the sole writer. The admin universe is auto-activated (it always
// exists), so registry ops work regardless of which work universe is displayed.
func (a *App) orchSvc() (orchapi.OrchestratorService, error) {
	a.mu.Lock()
	master := a.master
	a.mu.Unlock()
	if master == nil {
		return nil, fmt.Errorf("orchestrator master not initialized")
	}
	// Activate is idempotent — returns the existing admin core if already active.
	core, err := master.Activate(a.ctx, admin.UniverseName)
	if err != nil {
		return nil, fmt.Errorf("activate admin universe (registry host): %w", err)
	}
	return orchremote.NewRemote(core.Sender()), nil
}
