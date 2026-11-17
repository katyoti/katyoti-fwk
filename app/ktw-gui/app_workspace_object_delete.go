// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// WorkspaceObjectDeleteDetach removes one object AND the links other objects
// have to it — the answer to a delete CWM refused because the object is
// linked. Children still refuse: they are moved or deleted first.
//
// Wails-bound: window.go.main.App.WorkspaceObjectDeleteDetach(universeName, objectID).
func (a *App) WorkspaceObjectDeleteDetach(universeName, objectID string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return objectDeleteVia(a.ctx, sender, objectID, true)
}

// objectDeleteVia removes one object; detach also removes the links other
// objects have to it (WorkspaceObjectDelete passes false, ...DeleteDetach true).
func objectDeleteVia(ctx context.Context, sender ktsp.Sender, objectID string, detach bool) WorkspaceObjectMutationResult {
	if objectID == "" {
		return WorkspaceObjectMutationResult{Error: "Object ID is required."}
	}
	if _, err := sender.Call(ctx, "ktw-agents:cwm/object-delete",
		cwmapi.ObjectDeleteInput{Module: moduleOf(objectID), ID: objectID, Detach: detach}); err != nil {
		return WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return WorkspaceObjectMutationResult{Object: WorkspaceObject{ID: objectID}}
}
