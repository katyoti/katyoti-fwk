// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/katyoti/katyoti-fwk/app/ktw-gui/internal/supervisor"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// DISPLAY PANELS IN THE GUI: the screen follows a display panel
// by its id - its snapshot read from the hub's panel server
// (`ktw-hub:panel-service/snapshot`), then its events as they
// go out - and sends the person's input back on the panel's input subject,
// where the run owning it waits. The frontend's DisplayPanel draws it.

// PanelEventName is the frontend event each followed panel's event arrives
// on: {universe, follow, panel, event}.
const PanelEventName = "panel.event"

// panelTenant is the tenant the desktop GUI's bus scope runs under: the
// supervisor's, one constant.
const panelTenant = supervisor.TenantID

// panelFollows holds the panels the screen follows, by follow id. A follow
// ends when the screen unfollows it, when the page that started it is gone
// (a webview reload: the new page names itself, keepPage), when its
// universe is deactivated or restarted (stopUniverse) and at the GUI's
// shutdown (stopAll) - never a subscription left open.
type panelFollows struct {
	mu   sync.Mutex
	subs map[string]panelFollow
	page string // the page now drawing the panels
	next atomic.Int64
}

// panelFollow is one follow: its subscription, its universe and the page
// that started it.
type panelFollow struct {
	sub      mbapi.Subscription
	universe string
	page     string
}

// follow subscribes bus to the panel's events in universe, handing each to
// emit with the follow id it answers; page names the page that follows it
// (empty: the current one).
func (f *panelFollows) follow(bus mbapi.MessageBus, universe, id, page string, emit func(payload map[string]any)) (string, error) {
	if bus == nil {
		return "", fmt.Errorf("display panel %s: no bus for universe %q", id, universe)
	}
	if !panel.ValidID(id) {
		return "", fmt.Errorf("display panel: %q is no panel id", id)
	}
	followID := "pf-" + strconv.FormatInt(f.next.Add(1), 10)
	sub, err := bus.Subscribe(context.Background(), subjects.PanelOutSubject(panelTenant, universe, id), func(_ context.Context, msg mbapi.Message) {
		if msg == nil || !json.Valid(msg.Data()) {
			return
		}
		emit(map[string]any{"universe": universe, "follow": followID, "panel": id, "event": json.RawMessage(msg.Data())})
	})
	if err != nil {
		return "", fmt.Errorf("display panel %s: follow: %w", id, err)
	}
	f.mu.Lock()
	if f.subs == nil {
		f.subs = map[string]panelFollow{}
	}
	if page == "" {
		page = f.page
	}
	f.subs[followID] = panelFollow{sub: sub, universe: universe, page: page}
	f.mu.Unlock()
	return followID, nil
}

// stop ends one follow; an unknown id is no error.
func (f *panelFollows) stop(followID string) {
	f.stopWhere(func(id string, _ panelFollow) bool { return id == followID })
}

// stopUniverse ends every follow of a universe: its bus is gone or about to
// be.
func (f *panelFollows) stopUniverse(universe string) {
	f.stopWhere(func(_ string, x panelFollow) bool { return x.universe == universe })
}

// stopAll ends every follow (the GUI's shutdown).
func (f *panelFollows) stopAll() { f.stopWhere(func(string, panelFollow) bool { return true }) }

// keepPage names the page now drawing the panels and ends the follows any
// other page started: a reloaded webview's are gone with it.
func (f *panelFollows) keepPage(page string) {
	f.mu.Lock()
	f.page = page
	f.mu.Unlock()
	f.stopWhere(func(_ string, x panelFollow) bool { return x.page != page })
}

// stopWhere ends the follows that match, outside the lock.
func (f *panelFollows) stopWhere(match func(id string, x panelFollow) bool) {
	f.mu.Lock()
	var subs []mbapi.Subscription
	for id, x := range f.subs {
		if match(id, x) {
			subs = append(subs, x.sub)
			delete(f.subs, id)
		}
	}
	f.mu.Unlock()
	for _, sub := range subs {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}
}

// size is how many follows are open.
func (f *panelFollows) size() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

// sendPanelInput validates the person's input and publishes it on the
// panel's input subject in universe.
func sendPanelInput(ctx context.Context, bus mbapi.MessageBus, universe, inputJSON string) error {
	if bus == nil {
		return fmt.Errorf("display panel: no bus for universe %q", universe)
	}
	in, err := panel.DecodeInput([]byte(inputJSON))
	if err != nil {
		return err
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return bus.Publish(ctx, subjects.PanelInSubject(panelTenant, universe, in.Panel), payload)
}

// universeBus is the bus of an active universe's core.
func (a *App) universeBus(universeName string) (mbapi.MessageBus, error) {
	a.mu.Lock()
	master := a.master
	a.mu.Unlock()
	if master == nil {
		return nil, fmt.Errorf("orchestrator master not initialized")
	}
	if universeName == "" || !master.IsActive(universeName) {
		return nil, fmt.Errorf("universe %q is not active — activate it first", universeName)
	}
	core, err := master.Activate(a.ctx, universeName)
	if err != nil {
		return nil, fmt.Errorf("activate %q: %w", universeName, err)
	}
	return core.MessageBus(), nil
}

// PanelFollow follows a display panel of a universe: each of its events
// arrives as the frontend event "panel.event" carrying the follow id it
// answers. The snapshot is read through the hub's panel-service.
//
// page names the page following it (PanelPage).
//
// Wails-bound: window.go.main.App.PanelFollow(universe, panelId, page).
func (a *App) PanelFollow(universeName, panelID, page string) (string, error) {
	bus, err := a.universeBus(universeName)
	if err != nil {
		return "", err
	}
	return a.panels.follow(bus, universeName, panelID, page, func(payload map[string]any) {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, PanelEventName, payload)
		}
	})
}

// PanelUnfollow ends a follow PanelFollow started.
//
// Wails-bound: window.go.main.App.PanelUnfollow(followId).
func (a *App) PanelUnfollow(followID string) { a.panels.stop(followID) }

// PanelPage names the page drawing the panels, once per load of the
// frontend: the follows an earlier page started (before a webview reload)
// end.
//
// Wails-bound: window.go.main.App.PanelPage(page).
func (a *App) PanelPage(page string) { a.panels.keepPage(page) }

// PanelSendInput sends the person's input to a display panel - a form's
// value, a button, a chat message, a cancel (panel.Input as JSON) - on its
// input subject, where the run owning the panel waits.
//
// Wails-bound: window.go.main.App.PanelSendInput(universe, inputJSON).
func (a *App) PanelSendInput(universeName, inputJSON string) error {
	bus, err := a.universeBus(universeName)
	if err != nil {
		return err
	}
	return sendPanelInput(a.ctx, bus, universeName, inputJSON)
}
