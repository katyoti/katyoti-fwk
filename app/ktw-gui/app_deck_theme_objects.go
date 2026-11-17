// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// senderThemeObjects is where the deck theme resolver reads the project's
// deck-theme objects from the GUI: the universe's CWM, over KTSP (the GUI
// holds no CWM of its own).
type senderThemeObjects struct{ sender ktsp.Sender }

func (s senderThemeObjects) ObjectList(ctx context.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	var out cwmapi.ObjectListOutput
	raw, err := s.sender.Call(ctx, "ktw-agents:cwm/object-list", in)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

// ObjectGet reads one object in full (a listed object carries no body): a
// slide group's slides are its body.
func (s senderThemeObjects) ObjectGet(ctx context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	var out cwmapi.ObjectGetOutput
	raw, err := s.sender.Call(ctx, "ktw-agents:cwm/object-get", in)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

func (s senderThemeObjects) ObjectFileRead(ctx context.Context, in cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error) {
	var out cwmapi.ObjectFileReadOutput
	raw, err := s.sender.Call(ctx, "ktw-agents:cwm/object-file-read", in)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

// deckThemeCtx is the context a deck theme is resolved in for a universe:
// its project themes (deck-theme objects) come before the sidecars'.
func (a *App) deckThemeCtx(universeName string) context.Context {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return a.ctx
	}
	return deckapi.WithThemeObjects(a.ctx, senderThemeObjects{sender: sender})
}

// DeckThemeObjectResult is a deck-theme object as its pane edits it: the
// declaration (JSON) and the theme's sample deck.
type DeckThemeObjectResult struct {
	Name   string `json:"name"`
	Theme  string `json:"theme"`
	Sample string `json:"sample"`
	Error  string `json:"error,omitempty"`
}

// DeckThemeObject reads a deck-theme object for its pane: the theme's
// declaration and its sample deck (from the module's themes/<name>/).
//
// Wails-bound: window.go.main.App.DeckThemeObject(universeName, objectID).
func (a *App) DeckThemeObject(universeName, objectID string) DeckThemeObjectResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return DeckThemeObjectResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	obj, _, err := readObjectVia(a.ctx, sender, objectID)
	if err != nil {
		return DeckThemeObjectResult{Error: err.Error()}
	}
	theme, err := deckapi.ThemeOfObject(obj)
	if err != nil {
		return DeckThemeObjectResult{Error: err.Error()}
	}
	raw, err := json.Marshal(theme)
	if err != nil {
		return DeckThemeObjectResult{Error: err.Error()}
	}
	out := DeckThemeObjectResult{Name: theme.Name, Theme: string(raw)}
	if theme.Sample != "" {
		f, err := senderThemeObjects{sender: sender}.ObjectFileRead(a.ctx, cwmapi.ObjectFileReadInput{ID: objectID, Path: deckapi.ThemeFilePath(theme.Name, theme.Sample)})
		if err == nil {
			out.Sample = string(f.Content)
		}
	}
	return out
}
