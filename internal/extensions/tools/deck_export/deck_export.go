// SPDX-License-Identifier: Apache-2.0

// Package deck_export is the agent's `deck-export` tool: a slide deck saved
// as a PowerPoint (editable slides, or one picture per slide) or as one HTML
// page that presents offline (modules/slidedeck/). The display server makes
// the file — the same export the GUI's ⇩ Export menu runs — and the tool
// writes it into the workspace.
package deck_export

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	displayapi "github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	deckdisplay "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/display"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// DeckExportInput names the deck and what to make of it.
type DeckExportInput struct {
	File     string `json:"file,omitempty"     jsonschema:"The deck's .deck.md file (workspace-relative or absolute). Give this or markdown"`
	Markdown string `json:"markdown,omitempty" jsonschema:"The deck's markdown when it has no file. Give this or file"`
	Format   string `json:"format"             jsonschema:"description=pptx: a PowerPoint of editable slides in the theme; pptx-pictures: one picture per slide exactly as shown; html: one page that presents offline (double click; send by email),enum=pptx|pptx-pictures|html"`
	Output   string `json:"output,omitempty"   jsonschema:"Where to write the file (workspace-relative or absolute). Default: beside the deck file or else decks/<title> — with the format's extension"`
}

// DeckExportOutput says what was written.
type DeckExportOutput struct {
	Path    string `json:"path"    jsonschema:"The file written"`
	Bytes   int    `json:"bytes"   jsonschema:"Its size"`
	Slides  int    `json:"slides"  jsonschema:"The deck's slides"`
	Message string `json:"message" jsonschema:"What to tell the user"`
}

const toolDescription = `Save a slide deck as a file for people who do not have ArchiFactory: a PowerPoint of editable slides in the deck's theme (format pptx), a PowerPoint of one picture per slide exactly as shown (pptx-pictures), or one HTML page that presents offline and can be sent by email (html). Give the deck's .deck.md file, or its markdown. The speaker notes are kept. A PowerPoint takes a few seconds per slide.`

// NewDeckExportTool creates the deck-export tool.
func NewDeckExportTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"deck-export",
		toolDescription,
		executeDeckExport,
		toolsapi.WithMutating(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "⇩", Label: "Deck export", Color: "#82072A", Template: "{{.format}}"}),
	)
}

// The display server makes a PowerPoint slide by slide: give it time.
var exportClient = &http.Client{Timeout: 10 * time.Minute}

func executeDeckExport(ctx context.Context, in DeckExportInput) (DeckExportOutput, error) {
	format := deckdisplay.Format(strings.TrimSpace(in.Format))
	if !format.Valid() {
		return DeckExportOutput{}, fmt.Errorf("deck-export: format must be pptx, pptx-pictures or html, not %q", in.Format)
	}
	markdown, source, err := deckSource(in)
	if err != nil {
		return DeckExportOutput{}, err
	}
	// The slide groups it includes, written in (th22): the export needs none.
	if groups, err := deckapi.GroupsFor(ctx, mbapi.Q{Reach: mbapi.ReachUniverse}, markdown); err == nil {
		markdown = deckapi.InlineGroups(markdown, groups)
	}
	deck := deckapi.Parse(markdown)
	if len(deck.Slides) == 0 {
		return DeckExportOutput{}, fmt.Errorf("deck-export: the deck has no slide")
	}

	bridge, err := application.UseService[displayapi.SchemaBridge](ctx)
	if err != nil {
		return DeckExportOutput{}, fmt.Errorf("deck-export: the display server is not available here: %w", err)
	}
	w, err := bridge.GetWidgetURL(ctx, displayapi.GetWidgetURLInput{Kind: "deck"})
	if err != nil {
		return DeckExportOutput{}, fmt.Errorf("deck-export: display server: %w", err)
	}
	origin, token, err := deckdisplay.Origin(w.URL)
	if err != nil {
		return DeckExportOutput{}, err
	}
	if token == "" {
		token = w.Token
	}
	// The deck's theme goes to the display server first, as to present.
	if name, _ := deck.Settings["theme"].(string); name != "" && name != deckapi.DefaultTheme {
		theme, err := deckapi.ResolveTheme(ctx, mbapi.Q{Reach: mbapi.ReachUniverse}, name)
		if err != nil {
			return DeckExportOutput{}, fmt.Errorf("deck-export: theme %q: %w", name, err)
		}
		if _, err := deckdisplay.PublishTheme(ctx, exportClient, origin, token, theme); err != nil {
			return DeckExportOutput{}, fmt.Errorf("deck-export: %w", err)
		}
	}
	// The files beside the deck file (its images and backgrounds) go with it.
	var assets *deckdisplay.AssetsRef
	if source != "" {
		dir := filepath.Dir(source)
		files, _ := deckdisplay.ReadBeside(deckapi.LocalAssets(markdown), func(rel string) ([]byte, error) {
			return os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		})
		if len(files) > 0 {
			ref, err := deckdisplay.PublishAssets(ctx, exportClient, origin, token, deckdisplay.AssetsScope("file:"+source), files)
			if err != nil {
				return DeckExportOutput{}, fmt.Errorf("deck-export: %w", err)
			}
			assets = &ref
		}
	}
	// The production mark (docs ProductionMark) is written in, as the Deck
	// panel shows it: the exported file says it was produced by ArchiFactory.
	markdown = deckapi.MarkDeck(ctx, markdown)
	data, err := deckdisplay.Export(ctx, exportClient, origin, token, markdown, format, assets)
	if err != nil {
		return DeckExportOutput{}, err
	}

	path := outputPath(in.Output, source, deck, format)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return DeckExportOutput{}, fmt.Errorf("deck-export: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return DeckExportOutput{}, fmt.Errorf("deck-export: %w", err)
	}
	what := map[deckdisplay.Format]string{
		deckdisplay.FormatPptx:         "a PowerPoint of editable slides",
		deckdisplay.FormatPptxPictures: "a PowerPoint of one picture per slide",
		deckdisplay.FormatHTML:         "an HTML page that presents offline (double click; it can be sent by email)",
	}[format]
	return DeckExportOutput{
		Path:    path,
		Bytes:   len(data),
		Slides:  len(deck.Slides),
		Message: fmt.Sprintf("%s written: %s (%d slides, %d KB)", what, path, len(deck.Slides), len(data)/1024),
	}, nil
}

// deckSource reads the deck: its file (returned too, to write beside it) or
// the markdown given.
func deckSource(in DeckExportInput) (markdown, file string, err error) {
	if f := strings.TrimSpace(in.File); f != "" {
		p := workspacePath(f)
		b, err := os.ReadFile(p)
		if err != nil {
			return "", "", fmt.Errorf("deck-export: read %s: %w", f, err)
		}
		return string(b), p, nil
	}
	if strings.TrimSpace(in.Markdown) == "" {
		return "", "", fmt.Errorf("deck-export: give the deck's file or its markdown")
	}
	return in.Markdown, "", nil
}

// workspacePath resolves a relative path against the workspace root.
func workspacePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if ws := application.Env().WorkspaceDir(); ws != "" {
		return filepath.Join(ws, p)
	}
	return p
}

var unsafeName = regexp.MustCompile(`[\\/:*?"<>|]+`)

// outputPath is where the export goes: as asked, else beside the deck's
// file, else decks/<title> in the workspace — with the format's extension.
func outputPath(asked, source string, deck deckapi.Deck, format deckdisplay.Format) string {
	ext := format.Extension()
	if a := strings.TrimSpace(asked); a != "" {
		p := workspacePath(a)
		if filepath.Ext(p) == "" {
			p += ext
		}
		return p
	}
	if source != "" {
		base := strings.TrimSuffix(strings.TrimSuffix(source, ".md"), ".deck")
		return base + ext
	}
	title, _ := deck.Settings["title"].(string)
	name := strings.TrimSpace(unsafeName.ReplaceAllString(title, "-"))
	if name == "" {
		name = "presentation"
	}
	return workspacePath(filepath.Join("decks", name+ext))
}

// StrategyDef registers the tool as the agent plugin `deck-export`.
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"deck-export", "Save a slide deck as PowerPoint or as a standalone HTML page",
	func() (agentapi.PluginFactory, error) {
		return toolsapi.NewToolPluginFactory(NewDeckExportTool()), nil
	},
)

// Extension contributes the plugin; a task lists `deck-export` in its plugins.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("deck-export-tool", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
