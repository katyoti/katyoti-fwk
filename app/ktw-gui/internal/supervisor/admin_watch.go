// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"log"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// UniversesEventHandler is invoked whenever the universe REGISTRY changes --
// created, deleted, renamed, attached, or sidecar membership edited -- no matter
// which process made the change. The App layer forwards this to the frontend so
// the universe ribbon refreshes.
//
// The payload is deliberately not passed through: the handler re-reads the
// registry through the service, so it can never render a stale view carried in
// an event.
type UniversesEventHandler func()

// ProfileEventHandler is invoked whenever the user's profile is updated -- from
// this GUI, another one, or `katyoti profile` in a terminal. Like the registry
// handler it carries no payload: the frontend re-reads the profile.
type ProfileEventHandler func()

// SetUniversesEventHandler installs the handler invoked on every registry
// change. Call once at App startup, BEFORE the first Activate -- like the
// workspace watch, a core activated earlier does not get the watch installed
// retroactively.
func (m *Master) SetUniversesEventHandler(h UniversesEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.universesHandler = h
}

// SetProfileEventHandler installs the handler invoked on every profile change.
// Same timing rule as SetUniversesEventHandler.
func (m *Master) SetProfileEventHandler(h ProfileEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profileHandler = h
}

// installAdminWatches subscribes the ADMIN core's bus to the control-plane
// change subjects (the universe registry, the profile). Only admin, because that
// is where control-plane state is announced (admin.PublishEvent): a client that
// just created "foo" is not connected to "foo" yet -- but it is always connected
// to admin.
//
// Subscription failure is non-fatal. Losing it costs live refresh, not
// correctness: every panel still re-reads on its next fetch.
func (m *Master) installAdminWatches(core *ActiveCore) {
	if core.Name != admin.UniverseName {
		return
	}
	watches := []struct {
		subject string
		handler func() func()
	}{
		{subjects.UniversesChangedSubject(activeTenantID, admin.UniverseName), func() func() { return m.universesHandler }},
		{subjects.ProfileChangedSubject(activeTenantID, admin.UniverseName), func() func() { return m.profileHandler }},
	}
	for _, w := range watches {
		handler := w.handler
		sub, err := core.MessageBus().Subscribe(context.Background(), w.subject,
			func(_ context.Context, _ mbapi.Message) {
				m.mu.RLock()
				h := handler()
				m.mu.RUnlock()
				if h != nil {
					h()
				}
			})
		if err != nil {
			log.Printf("[supervisor] admin watch subscribe failed (%s): %v -- it will refresh on the next read instead of live", w.subject, err)
			continue
		}
		core.mu.Lock()
		core.adminSubs = append(core.adminSubs, sub)
		core.mu.Unlock()
	}
}
