// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// THE GUI LENDS A PANEL TO A CONTROL-PLANE CALL: the
// welcome screen's and the Marketplace's installs (install, update,
// set-version, install-solution, install-prereqs, doctor) are ktw-univers
// slots of the admin universe, which has no action surface. The GUI opens a progress panel itself on that universe's panel
// server (the GUI as its owner), lends it to the direct KTSP call
// (panel.WithHandle - the ktw.meta.panel header), so the method's
// progress.Emit reports its steps into it, then completes it from the call's
// answer or error. The screen draws it inline (DisplayPanel); its Cancel - the
// panel's own, or WorkspaceCancelCall(callID) - gives the call up. The call
// keeps its client budget (callBudget): no run owns it.

// callPanelOwner is the owner a panel the GUI opened for a call names.
const callPanelOwner = "ktw-gui"

// The ends a lent call's panel completes with (the method runs' own words).
const (
	callPanelOK        = "ok"
	callPanelFailed    = "failed"
	callPanelCancelled = "cancelled"
)

// callPanels holds the panels the GUI opened for a call, by the call's id,
// from OpenCallPanel until the call made under it ends.
type callPanels struct {
	mu   sync.Mutex
	open map[string]*panel.Handle
}

var lentPanels = &callPanels{open: map[string]*panel.Handle{}}

func (c *callPanels) put(callID string, h *panel.Handle) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.open[callID] = h
}

// take hands the panel opened for callID over to its call, once.
func (c *callPanels) take(callID string) (*panel.Handle, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.open[callID]
	delete(c.open, callID)
	return h, ok && h != nil
}

// openCallPanel opens a progress panel in universe, the GUI as its owner.
func openCallPanel(ctx context.Context, bus mbapi.MessageBus, universe, title string) (*panel.Handle, error) {
	if bus == nil {
		return nil, fmt.Errorf("call panel: no bus for universe %q", universe)
	}
	publish := func(ctx context.Context, subject string, payload []byte) error {
		return bus.Publish(ctx, subject, payload)
	}
	return panel.OpenPanel(ctx, panel.Scope{Tenant: panelTenant, Universe: universe}, publish,
		panel.Open{Type: panel.TypeProgress, Title: title, Owner: callPanelOwner})
}

// callInPanel makes one KTSP call lent the panel h: the method reports into
// it, the person's cancel on it gives the call up, and the panel completes
// with the call's end - its answer, its error, or its cancel.
func callInPanel(ctx context.Context, bus mbapi.MessageBus, sender ktsp.Sender, h *panel.Handle, universe, slotPath string, input any) KTSPCallResult {
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if bus != nil {
		// The person's « cancel » on the panel ends the call.
		if sub, err := bus.Subscribe(ctx, subjects.PanelInSubject(panelTenant, universe, h.ID()), func(_ context.Context, m mbapi.Message) {
			if in, err := panel.DecodeInput(m.Data()); err == nil && in.Kind == panel.InputCancel {
				cancel()
			}
		}); err == nil {
			defer func() { _ = sub.Unsubscribe() }()
		}
		// The events the method sends on the lent panel: the GUI's own go on
		// after them even when the call gives no answer back.
		if sub, err := bus.Subscribe(ctx, subjects.PanelOutSubject(panelTenant, universe, h.ID()), func(_ context.Context, m mbapi.Message) {
			if e, err := panel.DecodeEvent(m.Data()); err == nil {
				h.Advance(e.Seq)
			}
		}); err == nil {
			defer func() { _ = sub.Unsubscribe() }()
		}
	}
	respRaw, err := sender.Call(panel.WithHandle(callCtx, h), slotPath, input)
	switch {
	// Given up - the panel's cancel or WorkspaceCancelCall; a passed budget
	// is a failure.
	case errors.Is(callCtx.Err(), context.Canceled):
		completeCallPanel(h, callPanelCancelled, nil)
		return KTSPCallResult{Error: fmt.Sprintf("%s: cancelled", slotPath)}
	case err != nil:
		completeCallPanel(h, callPanelFailed, map[string]any{"error": err.Error()})
		return KTSPCallResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	var result any
	if len(respRaw) > 0 {
		result = json.RawMessage(respRaw)
	}
	completeCallPanel(h, callPanelOK, result)
	return KTSPCallResult{Response: respRaw}
}

// completeCallPanel sends the panel's end on a context of its own: the
// call's may be the one that was cancelled.
func completeCallPanel(h *panel.Handle, status string, result any) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.Complete(ctx, status, result)
}

// OpenCallPanel opens the progress panel a call of universe will be lent,
// under the call's id, and answers the panel's id - what the screen draws
// before the call starts (CallKTSPMethodInPanel).
//
// Wails-bound: window.go.main.App.OpenCallPanel(universeName, title, callID).
func (a *App) OpenCallPanel(universeName, title, callID string) (string, error) {
	if callID == "" {
		return "", fmt.Errorf("call panel: a call id is required")
	}
	// The admin universe is reached before anything is activated
	// (universeSender activates it); its bus then follows.
	if _, err := a.universeSender(universeName); err != nil {
		return "", err
	}
	bus, err := a.universeBus(universeName)
	if err != nil {
		return "", err
	}
	h, err := openCallPanel(a.ctx, bus, universeName, title)
	if err != nil {
		return "", err
	}
	lentPanels.put(callID, h)
	return h.ID(), nil
}

// CallKTSPMethodInPanel sends one KTSP call as CallKTSPMethod does, lent the
// panel OpenCallPanel opened under callID: the method reports its steps into
// it, and the panel completes with the call's end. WorkspaceCancelCall(callID)
// gives it up. The call keeps its client budget (callBudget).
//
// Wails-bound: window.go.main.App.CallKTSPMethodInPanel(universeName, slotPath, inputJSON, callID).
func (a *App) CallKTSPMethodInPanel(universeName, slotPath, inputJSON, callID string) KTSPCallResult {
	if slotPath == "" {
		return KTSPCallResult{Error: "slot path is required (e.g. `ktw-univers:marketplace/install`)"}
	}
	h, ok := lentPanels.take(callID)
	if !ok {
		return KTSPCallResult{Error: fmt.Sprintf("call %q has no panel open (OpenCallPanel first)", callID)}
	}
	var input any
	if inputJSON != "" {
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			completeCallPanel(h, callPanelFailed, map[string]any{"error": err.Error()})
			return KTSPCallResult{Error: fmt.Sprintf("Input is not valid JSON: %v", err)}
		}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		completeCallPanel(h, callPanelFailed, map[string]any{"error": err.Error()})
		return KTSPCallResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	bus, _ := a.universeBus(universeName)

	ctx := a.ctx
	var manifestOf mbapi.ManifestOf
	if !isLongRunningKTSP(slotPath) {
		if c, err := a.universeCatalog(universeName); err == nil {
			manifestOf = c.ManifestSlots
		}
	}
	if budget := callBudget(ctx, manifestOf, slotPath); budget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}
	ctx, done := guiCalls.begin(ctx, callID)
	defer done()
	return callInPanel(ctx, bus, sender, h, universeName, slotPath, input)
}
