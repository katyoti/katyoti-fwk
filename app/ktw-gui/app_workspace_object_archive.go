// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// « Retirer »: an object is taken out of the work by archiving
// it - its file kept, unchanged, in its module's history, its links read as
// ended -; the hard delete sits in the « ⋯ » menu, offered only when nothing
// points to the object.

// WorkspaceObjectArchive ends one object: cwm's ObjectArchive moves its file
// into its module's .ktw/_history/<yyyy-mm>/.
//
// Wails-bound: window.go.main.App.WorkspaceObjectArchive(universeName, objectID).
func (a *App) WorkspaceObjectArchive(universeName, objectID string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return objectArchiveVia(a.ctx, sender, objectID)
}

func objectArchiveVia(ctx context.Context, sender ktsp.Sender, objectID string) WorkspaceObjectMutationResult {
	if objectID == "" {
		return WorkspaceObjectMutationResult{Error: "Object ID is required."}
	}
	if _, err := sender.Call(ctx, "ktw-agents:cwm/object-archive", cwmapi.ObjectArchiveInput{ID: objectID}); err != nil {
		return WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return WorkspaceObjectMutationResult{Object: WorkspaceObject{ID: objectID}}
}

// WorkspaceObjectInboundResult names the objects whose links point to one
// object: the hard delete is offered only when it names none.
type WorkspaceObjectInboundResult struct {
	From  []string `json:"from"`
	Error string   `json:"error,omitempty"`
}

// WorkspaceObjectInbound lists the objects linking to one object.
//
// Wails-bound: window.go.main.App.WorkspaceObjectInbound(universeName, objectID).
func (a *App) WorkspaceObjectInbound(universeName, objectID string) WorkspaceObjectInboundResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectInboundResult{From: []string{}, Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return objectInboundVia(a.ctx, sender, objectID)
}

func objectInboundVia(ctx context.Context, sender ktsp.Sender, objectID string) WorkspaceObjectInboundResult {
	out := WorkspaceObjectInboundResult{From: []string{}}
	if objectID == "" {
		out.Error = "Object ID is required."
		return out
	}
	raw, err := sender.Call(ctx, "ktw-agents:cwm/edge-list", cwmapi.EdgeListInput{ObjectID: objectID, Direction: cwmapi.DirectionIn})
	if err != nil {
		out.Error = err.Error()
		return out
	}
	var edges cwmapi.EdgeListOutput
	if err := json.Unmarshal(raw, &edges); err != nil {
		out.Error = err.Error()
		return out
	}
	seen := map[string]bool{}
	for _, e := range edges.Edges {
		if e.From == "" || e.From == objectID || seen[e.From] {
			continue
		}
		seen[e.From] = true
		out.From = append(out.From, e.From)
	}
	sort.Strings(out.From)
	return out
}
