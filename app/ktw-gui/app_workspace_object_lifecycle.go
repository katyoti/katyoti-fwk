// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// WorkspaceObjectLifecycles is where an object may move, as the Lifecycle
// panel shows it: per lifecycle, the state, the history and the transitions
// from it, each with its guard verdict and its named gate's checklist.
type WorkspaceObjectLifecycles struct {
	Lifecycles []paneapi.LifecycleOptions `json:"lifecycles"`
	// Message reports the last move or confirmation that was refused, while
	// Lifecycles still says where the object is.
	Message string `json:"message,omitempty"`
	// MessageMsg is Message by key, in the person's language (errors namespace).
	MessageMsg *usermsg.Msg `json:"messageMsg,omitempty"`
	Error      string       `json:"error,omitempty"`
	ErrorMsg   *usermsg.Msg `json:"errorMsg,omitempty"`
}

func guiPerson(personID string) cwmapi.Actor {
	if personID == "" {
		personID = "user"
	}
	return cwmapi.Actor{ID: personID, Kind: cwmapi.ActorPerson}
}

// WorkspaceObjectTransitionOptions reads where an object may move, judged for
// the person using the GUI.
//
// Wails-bound: window.go.main.App.WorkspaceObjectTransitionOptions(universeName, objectID, personID).
func (a *App) WorkspaceObjectTransitionOptions(universeName, objectID, personID string) WorkspaceObjectLifecycles {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectLifecycles{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return transitionOptionsVia(a.ctx, sender, objectID, personID)
}

func transitionOptionsVia(ctx context.Context, sender ktsp.Sender, objectID, personID string) WorkspaceObjectLifecycles {
	raw, err := sender.Call(ctx, "ktw-agents:object-pane/transition-options", paneapi.TransitionOptionsInput{ID: objectID, By: guiPerson(personID)})
	if err != nil {
		return WorkspaceObjectLifecycles{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	var out paneapi.TransitionOptionsOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return WorkspaceObjectLifecycles{Error: fmt.Sprintf("decode response: %v", err)}
	}
	if out.Lifecycles == nil {
		out.Lifecycles = []paneapi.LifecycleOptions{}
	}
	return WorkspaceObjectLifecycles{Lifecycles: out.Lifecycles}
}

// WorkspaceObjectTransition moves an object, as the person using the GUI, and
// answers the options after it. A refused move (a guard, a named gate) comes
// back as Message, with the options unchanged.
//
// Wails-bound: window.go.main.App.WorkspaceObjectTransition(universeName, objectID, capability, to, transition, note, force, personID).
func (a *App) WorkspaceObjectTransition(universeName, objectID, capability, to, transition, note string, force bool, personID string) WorkspaceObjectLifecycles {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectLifecycles{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return objectTransitionVia(a.ctx, sender, objectID, capability, to, transition, note, force, personID)
}

func objectTransitionVia(ctx context.Context, sender ktsp.Sender, objectID, capability, to, transition, note string, force bool, personID string) WorkspaceObjectLifecycles {
	in := cwmapi.ObjectTransitionInput{ID: objectID, Capability: capability, To: to, Transition: transition, Note: note, Force: force, By: guiPerson(personID)}
	_, moveErr := sender.Call(ctx, "ktw-agents:cwm/object-transition", in)
	out := transitionOptionsVia(ctx, sender, objectID, personID)
	if moveErr != nil {
		out.Message, out.MessageMsg = moveErr.Error(), errorMsgOf(moveErr)
	}
	return out
}

// WorkspaceObjectGateConfirm records the person's confirmation of one
// requirement of a named gate, and answers the options after it.
//
// Wails-bound: window.go.main.App.WorkspaceObjectGateConfirm(universeName, objectID, gate, requirement, note, personID).
func (a *App) WorkspaceObjectGateConfirm(universeName, objectID, gate, requirement, note, personID string) WorkspaceObjectLifecycles {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectLifecycles{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return gateConfirmVia(a.ctx, sender, objectID, gate, requirement, note, personID)
}

func gateConfirmVia(ctx context.Context, sender ktsp.Sender, objectID, gate, requirement, note, personID string) WorkspaceObjectLifecycles {
	in := gatesapi.ConfirmInput{ID: objectID, Gate: gate, Requirement: requirement, Note: note, By: guiPerson(personID)}
	_, confirmErr := sender.Call(ctx, "ktw-agents:gates/confirm", in)
	out := transitionOptionsVia(ctx, sender, objectID, personID)
	if confirmErr != nil {
		out.Message, out.MessageMsg = confirmErr.Error(), errorMsgOf(confirmErr)
	}
	return out
}
