// SPDX-License-Identifier: Apache-2.0

package chatpanel

import (
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Follow feeds the bridge from the agent's UI artifact events and the panels
// it places (agent.panel_shown) on bus, and
// hands each panel event it makes to sink, in order; it answers the
// unsubscribe. The session's translator feeds its bridge itself (it filters
// the events by run); Follow is the bridge for any other listener of a run's
// bus.
func Follow(bus eventbus.EventBus, b *Bridge, sink func(panel.Event)) func() {
	var mu sync.Mutex
	emit := func(events []panel.Event, _ error) {
		for _, e := range events {
			sink(e)
		}
	}
	unsubs := []func(){
		bus.Subscribe(agentapi.EventUIArtifactMount, func(_ string, p any) {
			if e, ok := p.(agentapi.UIArtifactMountEvent); ok {
				mu.Lock()
				defer mu.Unlock()
				emit(b.Mount(e))
			}
		}),
		bus.Subscribe(agentapi.EventUIArtifactPatch, func(_ string, p any) {
			if e, ok := p.(agentapi.UIArtifactPatchEvent); ok {
				mu.Lock()
				defer mu.Unlock()
				emit(b.Patch(e))
			}
		}),
		bus.Subscribe(agentapi.EventUIArtifactComplete, func(_ string, p any) {
			if e, ok := p.(agentapi.UIArtifactCompleteEvent); ok {
				mu.Lock()
				defer mu.Unlock()
				emit(b.Complete(e))
			}
		}),
		bus.Subscribe(agentapi.EventPanelShown, func(_ string, p any) {
			if e, ok := p.(agentapi.PanelShownEvent); ok {
				mu.Lock()
				defer mu.Unlock()
				emit(b.Panel(e))
			}
		}),
	}
	return func() {
		for _, u := range unsubs {
			u()
		}
	}
}
