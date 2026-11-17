// SPDX-License-Identifier: Apache-2.0

// Package display_ui_component implements the `display-ui-component`
// agent tool. When an LLM agent wants to render structured UI (a single
// composition like form/view/table/wizard, OR a tree-shaped page DSL
// with mixed atomic + schema-bound nodes), it calls this tool with the
// artifact's id, kind, and either schema+value (for single-composition
// shorthand) or a dslTree (for page-shaped artifacts).
//
// The companion stream plugin (stream_plugin.go) hooks the LLM stream
// and emits UIArtifactMount + UIArtifactPatch events progressively as
// the tool-call args stream in — giving the user live "building up"
// rendering. The tool itself just validates the final args and emits
// a success message back to the LLM.
//
// Per decision (modules/display/decisions), this is the
// LLM's primary UI-emission tool. Combined with `complete-ui-component`
// (terminal marker) and `patch-ui-component` (cross-turn updates).
//
// See display-dsl-framework (ms06) for the spec and prompt.md for the
// LLM-facing system message fragment.
package display_ui_component

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// validKinds is the set of kinds accepted by the tool: the ui-component
// types (form, view, table, tree-table, wizard — schema-bound; page — a DSL
// tree), displayed as application/vnd.ktw.ui+json; drawio, displayed as
// application/vnd.jgraph.mxfile; and deck, a markdown slide deck displayed as
// text/markdown; variant=deck (render.MediaTypeForKind).
var validKinds = map[string]bool{
	"form":       true,
	"view":       true,
	"table":      true,
	"tree-table": true,
	"wizard":     true,
	"page":       true,
	"drawio":     true,
	"deck":       true,
}

// DisplayUIComponentInput is the input schema for the tool. The LLM
// emits this JSON; the streaming plugin extracts the `artifactId`,
// `kind`, and either schema/value (single-composition) or dslTree
// (page) progressively as the args stream in.
type DisplayUIComponentInput struct {
	ArtifactID string `json:"artifactId" jsonschema:"Stable id for this artifact (LLM-supplied; must be unique within the session). Used by patch-ui-component to address this artifact later." required:"true"`

	// What to display — the ONE render definition every display surface
	// takes (render.Def, p7p): an object panel's content and a panel method's
	// result are this same struct, so a kind the LLM can display is a kind a
	// panel can show, and the other way round.
	render.Def

	// Declarative action shortcuts (Mode B of the CRUD action wiring,
	// US-S3). When an action id declared here fires on the artifact,
	// the HOST handles it locally (template-substituted) instead of
	// round-tripping to the agent. Undeclared actions still flow back
	// as ui_event (Mode A). v1 host support: tool
	// "display-ui-component" only (mount another artifact from the
	// substituted args — e.g. row-click → detail view).
	Actions map[string]ActionShortcut `json:"actions,omitempty" jsonschema:"Optional per-action declarative shortcuts: {\"click\": {tool: \"display-ui-component\", argsTemplate: {...}}}. Template strings use {{item.field}} / {{item}} against the action payload. Declared actions are handled by the host with NO agent round-trip; others arrive as ui_event."`

	// Persistence flag.
	Persistent bool `json:"persistent,omitempty" jsonschema:"When true, the artifact stays mountable for cross-turn patch-ui-component calls. Default: false (artifact auto-completes on tool_use_done)."`
}

// ActionShortcut declares one Mode-B action binding: which tool the
// host invokes and the argument template ({{path}} placeholders
// substituted from the action's payload). Sandbox: the host only
// honours tools it has a native handler for — no escalation surface.
type ActionShortcut struct {
	Tool         string         `json:"tool"                   jsonschema:"Tool the host invokes locally; v1: \"display-ui-component\""`
	ArgsTemplate map[string]any `json:"argsTemplate,omitempty" jsonschema:"Tool args with {{item.field}} / {{item}} placeholders substituted from the action payload"`
}

// DisplayUIComponentOutput is the tool's result.
type DisplayUIComponentOutput struct {
	Success    bool   `json:"success"`
	ArtifactID string `json:"artifactId"`
	Message    string `json:"message"`
	// Findings is what the deck lint reports on a kind=deck artifact, each at
	// its source line — the agent fixes them and displays the deck again.
	Findings []deckapi.Finding `json:"findings,omitempty"`
}

// NewDisplayUIComponentTool creates the display-ui-component tool.
func NewDisplayUIComponentTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"display-ui-component",
		toolDescription,
		executeDisplayUIComponent,
		toolsapi.WithReadOnly(),
		toolsapi.WithDescriber(describeDisplayUIComponent),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{
			Icon: "🖼", Label: "Display UI", Color: "#A29BFE",
			Template: "{{.kind}} {{.artifactId}}",
		}),
	)
}

func executeDisplayUIComponent(ctx context.Context, in DisplayUIComponentInput) (DisplayUIComponentOutput, error) {
	// The stream plugin handles all UI emission (mount + progressive
	// patches) before this tool function fires. By the time we get
	// here, tool_use_done has fired and the artifact is fully mounted.
	// This function just validates inputs + returns a success message.

	if strings.TrimSpace(in.ArtifactID) == "" {
		return DisplayUIComponentOutput{}, fmt.Errorf("display-ui-component: artifactId is required")
	}
	if !validKinds[in.Kind] {
		return DisplayUIComponentOutput{}, fmt.Errorf(
			"display-ui-component: invalid kind %q (use one of: form, view, table, tree-table, wizard, page, drawio, deck)",
			in.Kind,
		)
	}

	if in.Kind == "page" {
		if in.DSLTree == nil {
			return DisplayUIComponentOutput{}, fmt.Errorf(
				"display-ui-component: kind='page' requires dslTree (the root UIPageNode)",
			)
		}
	} else if in.Kind == "drawio" {
		// Drawio takes EITHER a `file` path (host reads it — the stream plugin
		// inlines the XML before mount) OR an inline data { xml, filename? }.
		if strings.TrimSpace(in.File) != "" {
			if _, _, err := resolveDocumentFile(ctx, in.File); err != nil {
				return DisplayUIComponentOutput{}, fmt.Errorf(
					"display-ui-component: kind='drawio' file %q: %w", in.File, err,
				)
			}
		} else {
			v, ok := in.Data.(map[string]any)
			if !ok {
				return DisplayUIComponentOutput{}, fmt.Errorf(
					"display-ui-component: kind='drawio' requires `file` (path to a .drawio) OR data { xml, filename? }",
				)
			}
			xml, _ := v["xml"].(string)
			if strings.TrimSpace(xml) == "" {
				return DisplayUIComponentOutput{}, fmt.Errorf(
					"display-ui-component: kind='drawio' requires data.xml (mxGraph XML string) or a `file` path",
				)
			}
		}
	} else if in.Kind == "deck" {
		src, err := deckSource(ctx, in)
		if err != nil {
			return DisplayUIComponentOutput{}, err
		}
		findings := deckapi.Lint(src, deckGrammar(ctx))
		// A deck shown from its file: the files it names beside it.
		if f := strings.TrimSpace(in.File); f != "" && !isResourceURI(f) {
			dir := filepath.Dir(workspaceFile(f))
			findings = append(findings, deckapi.LintAssets(src, func(rel string) bool {
				_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
				return err == nil
			})...)
		}
		out := DisplayUIComponentOutput{
			Success:    true,
			ArtifactID: in.ArtifactID,
			Message:    fmt.Sprintf("slide deck %q displayed (%d slides)", in.ArtifactID, len(deckapi.Parse(src).Slides)),
			Findings:   findings,
		}
		if deckapi.HasErrors(findings) {
			out.Message += fmt.Sprintf(" with %d finding(s) — fix each at its line and display the deck again", len(findings))
		}
		return out, nil
	} else {
		// Schema-bound kinds need a schema: a data-type URI or the AST inline.
		switch sc := in.Schema.(type) {
		case string:
			if strings.TrimSpace(sc) == "" {
				return DisplayUIComponentOutput{}, fmt.Errorf("display-ui-component: kind=%q requires a schema (a data-type URI or the schema AST inline)", in.Kind)
			}
		case map[string]any:
		default:
			return DisplayUIComponentOutput{}, fmt.Errorf("display-ui-component: kind=%q requires a schema (a data-type URI or the schema AST inline)", in.Kind)
		}
	}

	msg := fmt.Sprintf("UI artifact %q displayed (kind=%s)", in.ArtifactID, in.Kind)
	if in.Persistent {
		msg += " — persistent (use patch-ui-component to update)"
	}

	return DisplayUIComponentOutput{
		Success:    true,
		ArtifactID: in.ArtifactID,
		Message:    msg,
	}, nil
}

// resolveDocumentFile reads an existing document referenced by the `file`
// field (a .drawio, a .deck.md) and returns its content + base filename. A
// resource URI (`<provider>:/<kind>/<path>`, e.g. a theme's sample deck) is
// opened through the catalog, in its kind's default layer. A relative path
// resolves against the workspace root (so the LLM can pass
// `<workspace_path>/media/x.drawio`), with a fallback to the path as given.
// Shared by the tool validation and the stream plugin (which inlines the
// content before the mount event).
func resolveDocumentFile(ctx context.Context, path string) (content, filename string, err error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", "", fmt.Errorf("empty file path")
	}
	if u, perr := contribapi.ParseResourceURI(p); perr == nil && !filepath.IsAbs(p) {
		layer, err := mbapi.Open(ctx, mbapi.Q{Reach: mbapi.ReachUniverse}, p, "")
		if err != nil {
			return "", "", err
		}
		b, err := layer.Bytes()
		if err != nil {
			return "", "", err
		}
		return string(b), filepath.Base(u.ID), nil
	}
	candidates := []string{p}
	if !filepath.IsAbs(p) {
		if ws := application.Env().WorkspaceDir(); ws != "" {
			candidates = []string{filepath.Join(ws, p), p}
		}
	}
	var lastErr error
	for _, c := range candidates {
		b, readErr := os.ReadFile(c)
		if readErr == nil {
			return string(b), filepath.Base(p), nil
		}
		lastErr = readErr
	}
	return "", "", fmt.Errorf("read: %w", lastErr)
}

// deckSource is a kind=deck artifact's markdown: the `file` it names, else its
// data — the markdown itself, or {markdown}.
func deckSource(ctx context.Context, in DisplayUIComponentInput) (string, error) {
	if strings.TrimSpace(in.File) != "" {
		src, _, err := resolveDocumentFile(ctx, in.File)
		if err != nil {
			return "", fmt.Errorf("display-ui-component: kind='deck' file %q: %w", in.File, err)
		}
		return src, nil
	}
	switch v := in.Data.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			return v, nil
		}
	case map[string]any:
		if s, _ := v["markdown"].(string); strings.TrimSpace(s) != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("display-ui-component: kind='deck' requires data (the deck's markdown) or a `file` path to a .deck.md")
}

// deckGrammar is the layouts and themes of the universe. When the catalog
// cannot be read the lint runs without them (it skips the layout, slot, theme,
// variant and icon checks rather than guessing).
func deckGrammar(ctx context.Context) deckapi.Grammar {
	g, err := deckapi.GrammarAt(ctx, mbapi.Q{Reach: mbapi.ReachUniverse})
	if err != nil {
		return deckapi.Grammar{}
	}
	return g
}

func describeDisplayUIComponent(raw json.RawMessage) string {
	var in DisplayUIComponentInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "display-ui-component <invalid>"
	}
	suffix := ""
	if id, ok := in.Schema.(string); ok && id != "" {
		suffix = fmt.Sprintf(" schema=%q", id)
	} else if in.Schema != nil {
		suffix = " (inline schema)"
	} else if in.Kind == "page" {
		suffix = " (page DSL)"
	}
	return fmt.Sprintf("display-ui-component %q kind=%s%s", in.ArtifactID, in.Kind, suffix)
}

//go:embed prompt.md
var toolDescription string

// isResourceURI reports whether a `file` value is a published document's URI
// rather than a path.
func isResourceURI(f string) bool {
	_, err := contribapi.ParseResourceURI(f)
	return err == nil && !filepath.IsAbs(f)
}

// workspaceFile resolves a `file` path as resolveDocumentFile does: relative
// to the workspace root when it is there.
func workspaceFile(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if ws := application.Env().WorkspaceDir(); ws != "" {
		if _, err := os.Stat(filepath.Join(ws, p)); err == nil {
			return filepath.Join(ws, p)
		}
	}
	return p
}
