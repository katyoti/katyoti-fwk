// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// maxDocumentBytes caps what DocumentOpen reads inline; a larger source is
// refused rather than streamed through a Wails binding.
const maxDocumentBytes = 32 << 20

// DocumentOpenResult is a resolved document, or why it could not be read.
type DocumentOpenResult struct {
	Document render.Document `json:"document"`
	Error    string          `json:"error,omitempty"`
}

// DocumentOpen reads a document's source and fixes its media type — the one
// place the GUI turns a source into content (modules/display/document-display.md).
// The media type is, in order: the explicit one, the contribution layer's
// declared type, a contributed engine's file suffix (EngineDecl.Files), the
// file extension, the bytes. Text comes back as a string;
// anything else as base64. An inline document without a media type is typed
// the same way, its title standing for the file name.
//
//   - source.uri: a contribution, opened through the content lane (layer
//     empty = the kind's default);
//   - source.path with nodeID (an object id): a file of that object's module,
//     relative to the module folder (CWM ObjectFileRead);
//   - source.path, absolute, without nodeID: a local file.
//
// Wails-bound: window.go.main.App.DocumentOpen(universeName, nodeId, doc).
func (a *App) DocumentOpen(universeName, nodeID string, doc render.Document) DocumentOpenResult {
	out := DocumentOpenResult{Document: doc}
	if doc.Source == nil {
		// Inline content without a media type: its title is its file name.
		if doc.MediaType == "" {
			out.Document.MediaType = render.Resolve(a.contributedMediaType(universeName, doc.Title), "", doc.Title, []byte(doc.Text()))
		}
		return a.resolveUIDocument(universeName, out)
	}
	src := *doc.Source
	var (
		b         []byte
		layerType string
		name      string
		err       error
	)
	switch {
	case src.URI != "":
		b, layerType, err = a.readContributionLayer(universeName, src.URI, src.Layer)
		if parsed, perr := contribapi.ParseResourceURI(src.URI); perr == nil {
			name = parsed.ID
		}
	case src.Path != "" && nodeID != "":
		b, err = a.readObjectFile(universeName, nodeID, src.Path)
		name = src.Path
	case src.Path != "" && filepath.IsAbs(src.Path):
		b, err = readLocalFile(src.Path)
		name = src.Path
	case src.Path != "":
		err = fmt.Errorf("relative path %q needs the node it belongs to", src.Path)
	default:
		err = fmt.Errorf("source has neither a path nor a uri")
	}
	if err != nil {
		out.Error = err.Error()
		return out
	}
	explicit := doc.MediaType
	if explicit == "" && layerType == "" {
		explicit = a.contributedMediaType(universeName, name)
	}
	mt := render.Resolve(explicit, layerType, name, b)
	out.Document.MediaType = mt
	out.Document.Source = nil
	if out.Document.Title == "" {
		out.Document.Title = filepath.Base(filepath.FromSlash(name))
	}
	if render.IsTextual(mt) {
		out.Document.Content = string(b)
	} else {
		out.Document.Content = base64.StdEncoding.EncodeToString(b)
		out.Document.Encoding = "base64"
	}
	return a.resolveUIDocument(universeName, out)
}

// resolveUIDocument makes a ui-component document self-contained: every
// component naming its schema by id or by an action's input, or laying a ui
// block over it, is resolved by the object pane (object-pane/ResolvePanel —
// the one resolver every host uses). A document holding no schema-bound
// component, and any other document, is returned as is.
func (a *App) resolveUIDocument(universeName string, out DocumentOpenResult) DocumentOpenResult {
	if out.Error != "" || render.BaseMediaType(out.Document.MediaType) != render.MediaTypeUI {
		return out
	}
	content := out.Document.Content
	if s, ok := content.(string); ok {
		var parsed any
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			out.Error = fmt.Sprintf("ui-component document is not JSON: %v", err)
			return out
		}
		content = parsed
	}
	if !render.HasComponents(content) {
		out.Document.Content = content
		return out
	}
	resolved, err := a.resolvePanel(universeName, content)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Document.Content = resolved
	return out
}

// resolvePanel asks the object pane to make content self-contained.
func (a *App) resolvePanel(universeName string, content any) (any, error) {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return nil, fmt.Errorf("cannot reach universe %q: %v", universeName, err)
	}
	raw, err := sender.Call(a.ctx, "ktw-agents:object-pane/resolve-panel", paneapi.ResolvePanelInput{Def: content})
	if err != nil {
		return nil, err
	}
	var resp paneapi.ResolvePanelOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode ResolvePanel: %v", err)
	}
	return resp.Def, nil
}

func (a *App) readContributionLayer(universeName, uri, layer string) ([]byte, string, error) {
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return nil, "", fmt.Errorf("cannot reach universe %q: %v", universeName, err)
	}
	rc, ref, err := cat.OpenLayer(a.ctx, uri, layer)
	if err != nil {
		return nil, ref.MediaType, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, maxDocumentBytes+1))
	if err == nil && len(b) > maxDocumentBytes {
		err = fmt.Errorf("layer larger than %d bytes", maxDocumentBytes)
	}
	return b, ref.MediaType, err
}

func (a *App) readObjectFile(universeName, objectID, path string) ([]byte, error) {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return nil, fmt.Errorf("cannot reach universe %q: %v", universeName, err)
	}
	raw, err := sender.Call(a.ctx, "ktw-agents:cwm/object-file-read", cwmapi.ObjectFileReadInput{ID: objectID, Path: path})
	if err != nil {
		return nil, fmt.Errorf("ObjectFileRead: %v", err)
	}
	var fr cwmapi.ObjectFileReadOutput
	if err := json.Unmarshal(raw, &fr); err != nil {
		return nil, fmt.Errorf("decode ObjectFileRead: %v", err)
	}
	return fr.Content, nil
}

func readLocalFile(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > maxDocumentBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, maxDocumentBytes)
	}
	return os.ReadFile(path)
}

// ContributedEngine is a display engine a sidecar contributes, as the GUI
// registers it: the declaration and who published it. The declaration's
// fields are spelled out rather than embedded: the Wails binding generator
// flattens an embedded struct of another package but does not generate the
// types its fields name (render.EngineFile), and the frontend build fails.
type ContributedEngine struct {
	ID           string                    `json:"id"`
	Provider     string                    `json:"provider"`
	URI          string                    `json:"uri"`
	Label        string                    `json:"label,omitempty"`
	Accepts      []string                  `json:"accepts"`
	Capabilities render.EngineCapabilities `json:"capabilities,omitempty"`
	Files        []render.EngineFile       `json:"files,omitempty"`
}

// Decl is the engine's declaration.
func (e ContributedEngine) Decl() render.EngineDecl {
	return render.EngineDecl{Label: e.Label, Accepts: e.Accepts, Capabilities: e.Capabilities, Files: e.Files}
}

// DisplayEnginesResult lists the contributed engines of a universe.
type DisplayEnginesResult struct {
	Engines []ContributedEngine `json:"engines"`
	Error   string              `json:"error,omitempty"`
}

// DisplayEngines lists the `display-engine` contributions of a universe for
// the GUI's engine registry.
//
// Wails-bound: window.go.main.App.DisplayEngines(universeName).
func (a *App) DisplayEngines(universeName string) DisplayEnginesResult {
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return DisplayEnginesResult{Engines: []ContributedEngine{}, Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	items, err := mbapi.QueryTyped[render.EngineDecl](a.ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: displayEngineKind, Catalog: cat})
	if err != nil {
		return DisplayEnginesResult{Engines: []ContributedEngine{}, Error: fmt.Sprintf("list display engines: %v", err)}
	}
	return DisplayEnginesResult{Engines: contributedEngines(items)}
}

const displayEngineKind = "display-engine"

// contributedEngines keeps the declarations an engine registry can use — at
// least one accepted media type — in a stable order. The widget page is the
// contribution's `widget` layer, fetched by DisplayEngineWidget when first needed.
func contributedEngines(items []mbapi.Attributed[render.EngineDecl]) []ContributedEngine {
	out := make([]ContributedEngine, 0, len(items))
	for _, it := range items {
		if len(it.Body.Accepts) == 0 {
			continue
		}
		id := it.URI
		if parsed, err := contribapi.ParseResourceURI(it.URI); err == nil {
			id = parsed.ID
		}
		d := it.Body
		out = append(out, ContributedEngine{ID: id, Provider: it.Provider, URI: it.URI, Label: d.Label, Accepts: d.Accepts, Capabilities: d.Capabilities, Files: d.Files})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out
}

// contributedMediaType types a file name by the universe's contributed
// engines (render.EngineDecl.Files), or "" — so a sidecar's own files open
// with its engine. Nothing to consult without a universe or a name.
func (a *App) contributedMediaType(universeName, name string) string {
	if universeName == "" || name == "" {
		return ""
	}
	r := a.DisplayEngines(universeName)
	decls := make([]render.EngineDecl, 0, len(r.Engines))
	for _, e := range r.Engines {
		decls = append(decls, e.Decl())
	}
	return render.MediaTypeForName(decls, name)
}

// DisplayEngineWidgetResult is a contributed engine's widget page.
type DisplayEngineWidgetResult struct {
	HTML  string `json:"html"`
	Error string `json:"error,omitempty"`
}

// DisplayEngineWidget returns the widget page of a contributed display engine
// — its `widget` layer, opened through the contribution lane (the hub forwards
// to the sidecar that publishes it). The GUI loads it by srcdoc in a sandboxed
// iframe: no URL, no port.
//
// Wails-bound: window.go.main.App.DisplayEngineWidget(universeName, uri).
func (a *App) DisplayEngineWidget(universeName, uri string) DisplayEngineWidgetResult {
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return DisplayEngineWidgetResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	layer, err := mbapi.Open(a.ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: displayEngineKind, Catalog: cat}, uri, contribapi.LayerWidget)
	if err != nil {
		return DisplayEngineWidgetResult{Error: err.Error()}
	}
	b, err := layer.Bytes()
	if err != nil {
		return DisplayEngineWidgetResult{Error: err.Error()}
	}
	return DisplayEngineWidgetResult{HTML: string(b)}
}
