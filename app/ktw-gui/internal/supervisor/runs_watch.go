// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// RunsEventHandler receives a universe's run-lifecycle events: one of its runs
// changed state, the event as the task service told it (its JSON; nil when
// the message carries none). The App layer forwards them to the frontend: the
// scheduler console moves its tiles and its ticker from them, and re-reads
// (it polls slowly besides).
type RunsEventHandler func(universe string, event json.RawMessage)

// SetRunsEventHandler installs the handler invoked for every run transition of
// every active universe. Call once at App startup, before the first Activate.
func (m *Master) SetRunsEventHandler(h RunsEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runsHandler = h
}

// installRunsWatch subscribes the core's bus to the universe's run-lifecycle
// subject. Failure is non-fatal: the Runs pane still polls.
func (m *Master) installRunsWatch(core *ActiveCore) {
	sub, err := m.watchRuns(core.MessageBus(), core.Name)
	if err != nil {
		log.Printf("[supervisor] runs watch subscribe failed for %q: %v — the Runs pane only polls", core.Name, err)
		return
	}
	core.mu.Lock()
	core.runsSub = sub
	core.mu.Unlock()
}

// watchRuns subscribes bus to a universe's run-lifecycle subject, calling the
// runs handler on each transition, with the event.
func (m *Master) watchRuns(bus mbapi.MessageBus, universe string) (mbapi.Subscription, error) {
	if bus == nil {
		return nil, fmt.Errorf("no bus")
	}
	subject := subjects.RunLifecycleSubject(activeTenantID, universe)
	return bus.Subscribe(context.Background(), subject, func(_ context.Context, msg mbapi.Message) {
		m.mu.RLock()
		h := m.runsHandler
		m.mu.RUnlock()
		if h == nil {
			return
		}
		var event json.RawMessage
		if msg != nil && json.Valid(msg.Data()) {
			event = json.RawMessage(msg.Data())
		}
		h(universe, event)
	})
}
