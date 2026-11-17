// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// DraftsModule is where a document saved without a place goes: the module
// « Brouillons », created the first time, from which the user moves it.
const DraftsModule = "drafts"

// WorkspaceDocumentSave keeps a document shown in a chat as an object of the
// workspace (th25): a deck becomes a slide-deck — the name given (else its title), its
// frontmatter the Deck fields, its slides the body — and markdown a document.
// parentID is the object it goes under; empty puts it in the drafts module.
//
// Wails-bound: window.go.main.App.WorkspaceDocumentSave(universeName,
//
//	parentID, mediaType, title, content).
func (a *App) WorkspaceDocumentSave(universeName, parentID, mediaType, title, content string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	out, err := documentSaveVia(a.ctx, sender, parentID, mediaType, title, content)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return out
}

func documentSaveVia(ctx context.Context, sender ktsp.Sender, parentID, mediaType, title, content string) (WorkspaceObjectMutationResult, error) {
	obj, err := documentObject(mediaType, title, content)
	if err != nil {
		return WorkspaceObjectMutationResult{}, err
	}
	if parentID == "" {
		if err := ensureDrafts(ctx, sender); err != nil {
			return WorkspaceObjectMutationResult{}, err
		}
		parentID = DraftsModule
	}
	obj.Parent = parentID
	module := moduleOf(parentID)
	if module == "" {
		module = parentID // a module object's id is its name
	}
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-create", cwmapi.ObjectCreateInput{Module: module, Object: obj})
	if err != nil {
		return WorkspaceObjectMutationResult{}, err
	}
	var resp cwmapi.ObjectCreateOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return WorkspaceObjectMutationResult{}, fmt.Errorf("decode response: %w", err)
	}
	return WorkspaceObjectMutationResult{Object: WorkspaceObject{
		ID: resp.Object.ID, Type: resp.Object.Type, Title: resp.Object.DisplayTitle(), Module: moduleOf(resp.Object.ID),
	}}, nil
}

// documentObject is the object a document becomes, by its media type.
func documentObject(mediaType, title, content string) (cwmapi.Object, error) {
	mt := strings.TrimSpace(mediaType)
	switch {
	case mt == render.MediaTypeDeck:
		parts := deckapi.DeckObjectOf(content)
		obj := cwmapi.Object{Type: "slide-deck"}
		obj.Name = firstNonEmpty(title, parts.Title, "Presentation")
		fields := map[string]any{}
		for k, v := range map[string]string{
			"aspect": parts.Deck.Aspect, "theme": parts.Deck.Theme, "transition": parts.Deck.Transition,
			"author": parts.Deck.Author, "watermark": parts.Deck.Watermark, "mark": parts.Deck.Mark,
		} {
			if v != "" {
				fields[k] = v
			}
		}
		if len(fields) > 0 {
			obj.Capabilities = map[string]map[string]any{"deck": fields}
		}
		obj.Body = parts.Body
		return obj, nil
	case mt == render.MediaTypeMarkdown:
		obj := cwmapi.Object{Type: "document"}
		obj.Name = firstNonEmpty(title, markdownTitle(content), "Document")
		obj.Body = content
		return obj, nil
	}
	return cwmapi.Object{}, fmt.Errorf("a %s document is not kept as an object; save it as a file", mediaType)
}

// ensureDrafts gives the project its drafts module when it has none.
func ensureDrafts(ctx context.Context, sender ktsp.Sender) error {
	if _, err := sender.Call(ctx, "ktw-agents:cwm/object-get", cwmapi.ObjectGetInput{Module: DraftsModule, ID: DraftsModule}); err == nil {
		return nil
	}
	mod := cwmapi.Object{ID: DraftsModule, Type: "documentation"}
	mod.Name = "Brouillons"
	mod.Description = "Documents not yet filed: move each where it belongs."
	if _, err := sender.Call(ctx, "ktw-agents:cwm/object-create", cwmapi.ObjectCreateInput{Module: DraftsModule, Object: mod}); err != nil {
		return fmt.Errorf("no drafts module, and creating it failed (%v): choose where to save the document", err)
	}
	return nil
}

// markdownTitle is the text of a markdown document's first heading.
func markdownTitle(md string) string {
	for _, l := range strings.Split(md, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "#") {
			if h := strings.TrimSpace(strings.TrimLeft(t, "#")); h != "" {
				return h
			}
		}
	}
	return ""
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
