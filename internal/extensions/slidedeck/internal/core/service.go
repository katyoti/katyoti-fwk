// SPDX-License-Identifier: Apache-2.0

// Package core implements the slide-deck service: the lint of a slide-deck
// object and the import of a PowerPoint template as a theme. local wires it.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/internal/pptx"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/remote"
)

// Service implements deckapi.DeckService.
type Service struct{}

var _ deckapi.DeckService = Service{}

// LintObject lints the hooked object's composed deck against the layouts and
// themes in the universe — its catalog, read over the bus from a sidecar —
// (the host's own when the universe cannot be read).
func (Service) LintObject(ctx context.Context, in deckapi.DeckObjectInput) (deckapi.ObjectLintOutput, error) {
	g, err := deckapi.GrammarAt(ctx, remote.Universe(ctx))
	if err != nil || len(g.Layouts) == 0 {
		g, _ = deckapi.GrammarAt(ctx, mbapi.Q{Reach: mbapi.ReachSelf})
	}
	return deckapi.LintObjectDeck(in, g), nil
}

// View composes a slide-deck object's deck from its name, its Deck fields and
// its slides, with the production mark written in (deckapi.MarkDeck).
func (Service) View(ctx context.Context, in deckapi.ViewInput) (render.Def, error) {
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return render.Def{}, err
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return render.Def{}, fmt.Errorf("deck view: %w", err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return render.Def{}, err
	}
	o := got.Object
	var fields deckapi.DeckObjectFields
	if raw, err := json.Marshal(o.Capabilities["deck"]); err == nil {
		_ = json.Unmarshal(raw, &fields)
	}
	md := deckapi.ComposeDeck(deckapi.DeckObjectInput{Title: o.Name, Deck: fields, Body: o.Body})
	return render.Def{Kind: "deck", Title: o.Name, Data: deckapi.MarkDeck(ctx, md)}, nil
}

// ImportTheme runs the importer on a template (a relative path is the
// workspace's), with the decisions applied, writing the theme folder when
// Folder is set (a relative Folder is the workspace's too).
func (Service) ImportTheme(ctx context.Context, in deckapi.ImportThemeInput) (deckapi.ImportThemeOutput, error) {
	if in.Template == "" || in.Name == "" {
		return deckapi.ImportThemeOutput{}, fmt.Errorf("import-theme: template and name are required")
	}
	data, err := os.ReadFile(inWorkspace(in.Template))
	if err != nil {
		return deckapi.ImportThemeOutput{}, err
	}
	// The sample deck is written from the layouts' examples, as the universe
	// serves them (a project's override included).
	layouts := deckapi.LayoutsInReach(ctx, remote.Universe(ctx))
	o := pptx.Options{Name: in.Name, Description: in.Description, Layouts: layouts, Decisions: in.Decisions}
	if in.Module == "" {
		out := ""
		if in.Folder != "" {
			out = inWorkspace(in.Folder)
		}
		return pptx.Import(data, o, filepath.Base(in.Template), out)
	}
	p, res, err := pptx.Prepare(data, o, filepath.Base(in.Template))
	if err != nil {
		return res, err
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return res, fmt.Errorf("import-theme: storing a theme in a module needs CWM: %w", err)
	}
	return storeTheme(ctx, cwm, in.Module, p, data, res)
}

// storeTheme stores a proposed theme as a deck-theme object of module: the
// declaration is its capability, its files and the template
// (reference.pptx) are module files in themes/<name>/. A theme of that name
// already in the module is refused: choose another name.
func storeTheme(ctx context.Context, cwm cwmapi.CWMService, module string, p *pptx.Proposal, template []byte, res deckapi.ImportThemeOutput) (deckapi.ImportThemeOutput, error) {
	raw, err := json.Marshal(p.Theme)
	if err != nil {
		return res, err
	}
	var theme map[string]any
	if err := json.Unmarshal(raw, &theme); err != nil {
		return res, err
	}
	obj := cwmapi.NewRecordObject(module, p.Theme.Name, deckapi.ObjectTypeDeckTheme, module, p.Theme.Name, nil)
	obj.Description = p.Theme.Description
	obj.Capabilities[deckapi.CapabilityDeckTheme] = theme
	// A theme of that name already there is refused before anything is
	// written: its files are not overwritten.
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: obj.ID}); err == nil {
		return res, fmt.Errorf("import-theme: module %s already has a theme %s: choose another name", module, p.Theme.Name)
	}
	// The files first, through the module's own object (the module is the
	// files' root), the object last: an import that fails half-way leaves no
	// theme without its files, and can be run again.
	files := map[string][]byte{"reference.pptx": template}
	for f, b := range p.Files {
		files[f] = b
	}
	names := make([]string, 0, len(files))
	for f := range files {
		names = append(names, f)
	}
	sort.Strings(names)
	var written []string
	for _, f := range names {
		w, err := writeInPieces(ctx, cwm, module, deckapi.ThemeFilePath(p.Theme.Name, f), files[f])
		if err != nil {
			return res, fmt.Errorf("import-theme: write %s in module %s: %w", f, module, err)
		}
		written = append(written, w)
	}
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj})
	if err != nil {
		return res, fmt.Errorf("import-theme: create the theme %s in module %s: %w", p.Theme.Name, module, err)
	}
	res.Object = created.Object.ID
	res.Written = append(append(res.Written, created.Path), written...)
	return res, nil
}

// inWorkspace resolves a relative path against the workspace folder.
func inWorkspace(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(application.Env().WorkspaceDir(), p)
}

// writePiece is the most one CWM write carries: a call to a CWM in another
// process is one bus message, and a template (reference.pptx) is larger than
// a message may be.
const writePiece = 256 << 10

// writeInPieces writes content to a module file, piece by piece (the first
// replaces the file, the others append), and returns its project path.
func writeInPieces(ctx context.Context, cwm cwmapi.CWMService, module, path string, content []byte) (string, error) {
	var projectPath string
	for off := 0; off == 0 || off < len(content); off += writePiece {
		end := min(off+writePiece, len(content))
		w, err := cwm.ObjectFileWrite(ctx, cwmapi.ObjectFileWriteInput{ID: module, Path: path, Content: content[off:end], Append: off > 0})
		if err != nil {
			return "", err
		}
		projectPath = w.ProjectPath
	}
	return projectPath, nil
}
