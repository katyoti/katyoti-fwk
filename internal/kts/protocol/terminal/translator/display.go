// SPDX-License-Identifier: Apache-2.0

package translator

import (
	"bytes"
	"encoding/json"
	"text/template"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

// resolveDisplay converts a ToolCallEvent's ToolDisplay metadata into
// a terminal DisplayData struct. Uses the template field to resolve a
// dynamic title from the tool input, falling back to DefaultDisplayResolver.
func resolveDisplay(e agentapi.ToolCallEvent) *terminal.DisplayData {
	if e.Display == nil {
		return defaultDisplay(e.Tool, e.Input)
	}

	d := &terminal.DisplayData{
		Icon:  e.Display.Icon,
		Label: e.Display.Label,
		Color: e.Display.Color,
	}

	// Resolve title from template + input.
	if e.Display.Template != "" && len(e.Input) > 0 {
		d.Title = executeTemplate(e.Display.Template, e.Input)
	}

	// Fallback title from input fields.
	if d.Title == "" {
		fallback := terminal.DefaultDisplayResolver(e.Tool, e.Input)
		d.Title = fallback.Title
	}

	// Fallback label.
	if d.Label == "" {
		d.Label = e.Tool
	}

	return d
}

// defaultDisplay produces a DisplayData when no ToolDisplay is available.
func defaultDisplay(toolName string, input json.RawMessage) *terminal.DisplayData {
	d := terminal.DefaultDisplayResolver(toolName, input)
	return &d
}

// executeTemplate runs a Go text/template against the tool input JSON.
// Returns the rendered string, or empty on any error.
func executeTemplate(tmpl string, input json.RawMessage) string {
	t, err := template.New("display").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return ""
	}

	var data map[string]any
	if err := json.Unmarshal(input, &data); err != nil {
		return ""
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "" // missing key → caller falls back to DefaultDisplayResolver
	}
	result := buf.String()
	if result == "<no value>" || result == "<nil>" {
		return ""
	}
	return result
}
