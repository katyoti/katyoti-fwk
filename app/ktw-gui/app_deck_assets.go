// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	deckdisplay "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/display"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// DeckAssetsResult says where the deck page finds the files beside a deck,
// and which named files could not be read.
type DeckAssetsResult struct {
	Scope   string   `json:"scope,omitempty"`
	Version string   `json:"version,omitempty"`
	Missing []string `json:"missing,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// DeckAssetsPublish makes the files a deck names by a relative path (its
// images and backgrounds; deckapi.LocalAssets) available to the deck page:
// it reads them beside the deck and posts them to the display server's
// deck-assets store (modules/slidedeck/). Beside the deck is:
//
//   - for a slide-deck object (objectID): its type's folder in its module
//     (decks/), read through CWM — never outside the module;
//   - for a deck file (docPath, absolute): the file's folder.
//
// A path leaving that folder (`..`) is never read. A deck naming no file
// returns an empty result.
//
// Wails-bound: window.go.main.App.DeckAssetsPublish(universeName, objectID, docPath, markdown).
func (a *App) DeckAssetsPublish(universeName, objectID, docPath, markdown string) DeckAssetsResult {
	paths := deckapi.LocalAssets(markdown)
	if len(paths) == 0 {
		return DeckAssetsResult{}
	}
	var read func(rel string) ([]byte, error)
	var scope string
	switch {
	case objectID != "":
		sender, err := a.universeSender(universeName)
		if err != nil {
			return DeckAssetsResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
		}
		folder, err := objectFolder(a, sender, objectID)
		if err != nil {
			return DeckAssetsResult{Error: err.Error()}
		}
		read = func(rel string) ([]byte, error) {
			// A file beside the deck is in its folder; `module:/…` (listed
			// as _module/…) is a file of its module, from the root.
			p := path.Join(folder, rel)
			if m, ok := strings.CutPrefix(rel, deckapi.ModuleAssetPrefix); ok {
				p = m
			}
			raw, err := sender.Call(a.ctx, "ktw-agents:cwm/object-file-read", cwmapi.ObjectFileReadInput{ID: objectID, Path: p})
			if err != nil {
				return nil, err
			}
			var out cwmapi.ObjectFileReadOutput
			if err := json.Unmarshal(raw, &out); err != nil {
				return nil, err
			}
			return out.Content, nil
		}
		scope = deckdisplay.AssetsScope("object:" + universeName + ":" + objectID)
	case filepath.IsAbs(docPath):
		dir := filepath.Dir(docPath)
		read = func(rel string) ([]byte, error) {
			if strings.HasPrefix(rel, deckapi.ModuleAssetPrefix) {
				return nil, fmt.Errorf("%s names a file of a module: only a deck object has one", rel)
			}
			p := filepath.Join(dir, filepath.FromSlash(rel))
			if !strings.HasPrefix(p, dir+string(filepath.Separator)) {
				return nil, fmt.Errorf("%s leaves the deck's folder", rel)
			}
			return os.ReadFile(p)
		}
		scope = deckdisplay.AssetsScope("file:" + docPath)
	default:
		return DeckAssetsResult{Missing: paths}
	}
	files, missing := deckdisplay.ReadBeside(paths, read)
	if len(files) == 0 {
		return DeckAssetsResult{Missing: missing}
	}
	widget, err := a.GetWidgetURL(universeName, "deck", "", "", "")
	if err != nil {
		return DeckAssetsResult{Error: err.Error()}
	}
	origin, token, err := deckdisplay.Origin(widget)
	if err != nil {
		return DeckAssetsResult{Error: err.Error()}
	}
	ref, err := deckdisplay.PublishAssets(a.ctx, http.DefaultClient, origin, token, scope, files)
	if err != nil {
		return DeckAssetsResult{Error: err.Error()}
	}
	return DeckAssetsResult{Scope: ref.Scope, Version: ref.Version, Missing: missing}
}

// objectFolder is where an object's type writes its objects in its module —
// a deck object's folder, beside which its files are.
func objectFolder(a *App, sender ktsp.Sender, objectID string) (string, error) {
	obj, _, err := readObjectVia(a.ctx, sender, objectID)
	if err != nil {
		return "", err
	}
	raw, err := sender.Call(a.ctx, "ktw-agents:cwm/object-type-get", cwmapi.ObjectTypeGetInput{Name: obj.Type})
	if err != nil {
		return "", err
	}
	var out cwmapi.ObjectTypeGetOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.ObjectType.ObjectsFolder(), nil
}
