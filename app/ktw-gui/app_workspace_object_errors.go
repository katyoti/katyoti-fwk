// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// WorkspaceObjectInError is one object CWM found in error, as the Global tab's
// Errors list shows it.
type WorkspaceObjectInError struct {
	ID     string                    `json:"id"`
	Type   string                    `json:"type,omitempty"`
	Title  string                    `json:"title,omitempty"`
	Module string                    `json:"module,omitempty"`
	Path   string                    `json:"path"`
	Errors []WorkspaceNodeDiagnostic `json:"errors"`
}

// WorkspaceUnreadableFile is a file CWM could not read as an object: it has no
// id to be listed under, so it is listed by path.
type WorkspaceUnreadableFile struct {
	Path     string   `json:"path"`
	Problems []string `json:"problems"`
}

// WorkspaceObjectErrors is every error of the universe's project.
type WorkspaceObjectErrors struct {
	Objects    []WorkspaceObjectInError  `json:"objects"`
	Unreadable []WorkspaceUnreadableFile `json:"unreadable,omitempty"`
	Total      int                       `json:"total"`
	Error      string                    `json:"error,omitempty"`
}

// WorkspaceObjectErrorList lists the objects CWM found in error and the files
// it could not read — the Global tab's Errors entry.
//
// Wails-bound: window.go.main.App.WorkspaceObjectErrorList(universeName).
func (a *App) WorkspaceObjectErrorList(universeName string) WorkspaceObjectErrors {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectErrors{Error: err.Error()}
	}
	out, err := objectErrorListVia(a.ctx, sender)
	if err != nil {
		return WorkspaceObjectErrors{Error: err.Error()}
	}
	return out
}

func objectErrorListVia(ctx context.Context, sender ktsp.Sender) (WorkspaceObjectErrors, error) {
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-error-list", cwmapi.ObjectErrorListInput{})
	if err != nil {
		return WorkspaceObjectErrors{}, fmt.Errorf("ktw-agents:cwm/object-error-list: %w", err)
	}
	var resp cwmapi.ObjectErrorListOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return WorkspaceObjectErrors{}, fmt.Errorf("decode response: %w", err)
	}
	out := WorkspaceObjectErrors{Objects: make([]WorkspaceObjectInError, 0, len(resp.Objects)), Total: resp.Total}
	for _, o := range resp.Objects {
		row := WorkspaceObjectInError{ID: o.ID, Type: o.Type, Title: o.Name, Module: o.Module, Path: o.Path}
		for _, d := range o.Errors {
			row.Errors = append(row.Errors, WorkspaceNodeDiagnostic{Code: d.Code, Label: d.Label, Description: d.Description})
		}
		out.Objects = append(out.Objects, row)
	}
	for _, u := range resp.Unreadable {
		out.Unreadable = append(out.Unreadable, WorkspaceUnreadableFile{Path: u.Path, Problems: u.Problems})
	}
	return out, nil
}
