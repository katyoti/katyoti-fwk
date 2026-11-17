// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"log"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// CatalogEventHandler receives a universe's `contributions-changed` events:
// its catalog reloaded (a sidecar wired, rebuilt or edited). The App layer
// forwards them to the frontend, which re-reads what it derived from the
// catalog — the contribution translation bundle first (i18n/contrib.ts).
type CatalogEventHandler func(universe string)

// SetCatalogEventHandler installs the handler invoked for every catalog change
// of every active universe. Call once at App startup, before the first
// Activate (as SetWorkspaceEventHandler).
func (m *Master) SetCatalogEventHandler(h CatalogEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.catalogHandler = h
}

// installCatalogWatch subscribes the core's bus to the universe's
// contributions-changed subject. Failure is non-fatal: the frontend still
// reads the catalog on activation; only live refresh is lost.
func (m *Master) installCatalogWatch(core *ActiveCore) {
	subject := subjects.ContributionsChangedSubject(activeTenantID, core.Name)
	sub, err := core.MessageBus().Subscribe(context.Background(), subject,
		func(_ context.Context, _ mbapi.Message) {
			m.mu.RLock()
			h := m.catalogHandler
			m.mu.RUnlock()
			if h != nil {
				h(core.Name)
			}
		})
	if err != nil {
		log.Printf("[supervisor] catalog watch subscribe failed for %q (%s): %v — live catalog refresh disabled",
			core.Name, subject, err)
		return
	}
	core.mu.Lock()
	core.catalogSub = sub
	core.mu.Unlock()
}
