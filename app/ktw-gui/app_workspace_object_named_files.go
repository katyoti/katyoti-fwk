// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// NAMED FILES in the GUI (2026-09-26): a capability's declared files, read by
// NAME — the pane never composes a path. The file overview is the first
// reader: on an object it reads `file-overview`'s `page` / `overview`.

// WorkspaceNamedFile is one named file as the GUI sees it. Content is text:
// the GUI renders markdown and page definitions, not bytes.
type WorkspaceNamedFile struct {
	Capability string `json:"capability"`
	Name       string `json:"name"`
	// Source is where the declaration in force came from: capability | type | object.
	Source  string `json:"source"`
	Path    string `json:"path"`
	Media   string `json:"media,omitempty"`
	Exists  bool   `json:"exists"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// WorkspaceObjectNamedFileRead reads one named file of an object.
//
// Wails-bound: window.go.main.App.WorkspaceObjectNamedFileRead(universeName, objectID, capability, name).
func (a *App) WorkspaceObjectNamedFileRead(universeName, objectID, capability, name string) WorkspaceNamedFile {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceNamedFile{Capability: capability, Name: name, Error: err.Error()}
	}
	return namedFileReadVia(a.ctx, sender, objectID, capability, name)
}

func namedFileReadVia(ctx context.Context, sender ktsp.Sender, objectID, capability, name string) WorkspaceNamedFile {
	out := WorkspaceNamedFile{Capability: capability, Name: name}
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-named-file-read",
		cwmapi.ObjectNamedFileReadInput{ID: objectID, Capability: capability, Name: name})
	if err != nil {
		out.Error = err.Error()
		return out
	}
	var resp cwmapi.ObjectNamedFileReadOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		out.Error = fmt.Sprintf("decode: %v", err)
		return out
	}
	out.Source, out.Path, out.Media, out.Exists, out.Content = resp.File.Source, resp.File.Path, resp.File.Media, resp.File.Exists, string(resp.Content)
	return out
}

// WorkspaceObjectNamedFileWrite writes one named file of an object; content is
// text (a deck's drawings, a page definition). The result carries the file as
// written, or its Error.
//
// Wails-bound: window.go.main.App.WorkspaceObjectNamedFileWrite(universeName, objectID, capability, name, content).
func (a *App) WorkspaceObjectNamedFileWrite(universeName, objectID, capability, name, content string) WorkspaceNamedFile {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceNamedFile{Capability: capability, Name: name, Error: err.Error()}
	}
	return namedFileWriteVia(a.ctx, sender, objectID, capability, name, content)
}

func namedFileWriteVia(ctx context.Context, sender ktsp.Sender, objectID, capability, name, content string) WorkspaceNamedFile {
	out := WorkspaceNamedFile{Capability: capability, Name: name}
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-named-file-write",
		cwmapi.ObjectNamedFileWriteInput{ID: objectID, Capability: capability, Name: name, Content: []byte(content)})
	if err != nil {
		out.Error = err.Error()
		return out
	}
	var resp cwmapi.ObjectNamedFileWriteOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		out.Error = fmt.Sprintf("decode: %v", err)
		return out
	}
	out.Source, out.Path, out.Media, out.Exists = resp.File.Source, resp.File.Path, resp.File.Media, true
	return out
}

// WorkspaceObjectNamedFiles lists the named files every capability in force
// on an object declares, resolved for it (content not included).
//
// Wails-bound: window.go.main.App.WorkspaceObjectNamedFiles(universeName, objectID).
func (a *App) WorkspaceObjectNamedFiles(universeName, objectID string) []WorkspaceNamedFile {
	sender, err := a.universeSender(universeName)
	if err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectNamedFiles %q: %v", universeName, err))
		return nil
	}
	raw, err := sender.Call(a.ctx, "ktw-agents:cwm/object-named-file-list", cwmapi.ObjectNamedFileListInput{ID: objectID})
	if err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectNamedFiles %q: %v", universeName, err))
		return nil
	}
	var resp cwmapi.ObjectNamedFileListOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectNamedFiles %q: decode: %v", universeName, err))
		return nil
	}
	out := make([]WorkspaceNamedFile, 0, len(resp.Files))
	for _, f := range resp.Files {
		out = append(out, WorkspaceNamedFile{Capability: f.Capability, Name: f.Name, Source: f.Source, Path: f.Path, Media: f.Media, Exists: f.Exists, Error: f.Error})
	}
	return out
}
