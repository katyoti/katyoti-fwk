// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// WorkspaceObject is one row of the OBJECTS tree.
//
// It is a SEPARATE shape from WorkspaceItem on purpose. The two trees read
// different models — nodes and objects — and giving them one struct would
// invite exactly the confusion this parallel period exists to avoid: an id
// from one tree handed to a binding that speaks the other. A node id is a
// UUID, an object id is `<module>.<local>`, and nothing would catch the mix-up
// at compile time if they shared a type.
type WorkspaceObject struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	// Module is the id's module segment — the bundle this object belongs to.
	Module string `json:"module,omitempty"`
	Depth  int    `json:"depth"`
	// HasChildren lets the frontend render an expander without counting rows.
	HasChildren bool `json:"hasChildren"`
	// HasErrors draws the row red: CWM found the object in error when it read
	// it (its errors are in its Information tab).
	HasErrors bool `json:"hasErrors,omitempty"`
	// System marks a system module or an object in one (CWM reads it from
	// the system-module capability): shown to experts only.
	System bool `json:"system,omitempty"`
	// Level is project or module - its type's level capability:
	// the frontend asks it, never the type's name.
	Level string `json:"level,omitempty"`
	// Project is the key of the project holding the object when the universe
	// serves several (cwm's store set names it); empty with one.
	Project string `json:"project,omitempty"`
}

// WorkspaceObjectsTree returns the CWM OBJECT tree as a depth-first flat list,
// the same shape contract the node tree uses so the frontend's tree rendering
// is shared rather than duplicated.
//
// IT RUNS BESIDE THE NODE TREE, NOT INSTEAD OF IT. The node/item model is what
// the workspace is still authored through, and the object model is what the
// method, the grader and the panels already read. Cutting the tree over in one
// step would mean porting the detail pane, the create/delete flows and the
// per-type capability gating at the same moment — with no way to compare the
// two against each other while doing it. Two tabs cost one binding and make
// the migration observable.
//
// ONE CALL. ObjectTree nests children, so a subtree arrives whole (G5) — where
// the node tree spends an extra ItemList per node.
//
// A failure is RETURNED, so the promise rejects and the tree draws the
// error where the objects would be: an unreachable core is not "No objects".
//
// Wails-bound: window.go.main.App.WorkspaceObjectsTree(universeName).
func (a *App) WorkspaceObjectsTree(universeName string) ([]WorkspaceObject, error) {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return nil, fmt.Errorf("objects tree of %q: %w", universeName, err)
	}
	out, err := objectsTreeVia(a.ctx, sender)
	if err != nil {
		return nil, fmt.Errorf("objects tree of %q: %w", universeName, err)
	}
	return out, nil
}

// objectsTreeVia is the binding minus the App: one ObjectTree call over the
// given sender, decoded and flattened. Split out so the ONE JSON round trip
// — where `[]any` vs `[]string` and a renamed field bite — is testable with a
// stub sender (the binding-level parity pattern, phase 5d).
func objectsTreeVia(ctx context.Context, sender ktsp.Sender) ([]WorkspaceObject, error) {
	// DrawnOnly is answered by CWM, which owns the type registry: a type
	// declaring `tree: root | node` is drawn. Filtering here by type NAME would
	// be the type-name check the type's `tree:` exists to replace.
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-tree", cwmapi.ObjectTreeInput{DrawnOnly: true})
	if err != nil {
		return nil, fmt.Errorf("ktw-agents:cwm/object-tree: %w", err)
	}
	var resp cwmapi.ObjectTreeOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	out := make([]WorkspaceObject, 0, resp.Total)
	projects := treeProjects(raw)
	var walk func(nodes []cwmapi.ObjectTreeNode, depth int)
	walk = func(nodes []cwmapi.ObjectTreeNode, depth int) {
		for _, n := range nodes {
			out = append(out, WorkspaceObject{
				ID:          n.Object.ID,
				Type:        n.Object.Type,
				Title:       n.Object.DisplayTitle(),
				Module:      moduleOf(n.Object.ID),
				Depth:       depth,
				HasChildren: len(n.Children) > 0,
				HasErrors:   n.HasErrors,
				System:      n.System,
				Level:       n.Level,
			})
			if i := len(out) - 1; i < len(projects) {
				out[i].Project = projects[i]
			}
			walk(n.Children, depth+1)
		}
	}
	walk(resp.Roots, 0)
	return out, nil
}

// moduleOf returns an object id's module segment, or "" when it has none.
//
// Errors are swallowed deliberately: this feeds a LABEL. An id that cannot be
// split is still shown, with no module chip, rather than removed from the tree
// — a row missing a badge is a smaller problem than a row that is not there.
func moduleOf(id string) string {
	m, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return ""
	}
	return m
}
