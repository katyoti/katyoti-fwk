// SPDX-License-Identifier: Apache-2.0

package console

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"
)

// RenderFunc renders a value in human-readable form (rich or raw mode).
// The Printer provides mode-aware helpers (Header, Dim, Table).
type RenderFunc func(p *Printer, v any) error

// Console manages multi-mode output for CLI commands.
//
// Commands call Render() for each named object:
//   - rich/raw mode: the renderer (function or template) is called immediately
//   - JSON mode: the value is collected; Flush() emits a single JSON document
//
// On Flush():
//   - single entry → emits just the value
//   - multiple entries → emits {"name1": v1, "name2": v2, ...}
//
// Unflushed consoles are automatically flushed at program exit via FlushAll().
type Console struct {
	mode    OutputMode
	w       io.Writer
	printer *Printer
	entries []consoleEntry
	flushed bool
}

type consoleEntry struct {
	name  string
	value any
}

// pendingConsoles tracks consoles that haven't been flushed yet.
// FlushAll() drains this list — call it before os.Exit or defer it in main.
var pendingConsoles []*Console

// NewConsole creates a Console for the given output mode.
// Writer defaults to os.Stdout if nil.
func NewConsole(mode OutputMode, w io.Writer) *Console {
	if w == nil {
		w = os.Stdout
	}
	c := &Console{
		mode:    mode,
		w:       w,
		printer: NewPrinter(mode),
	}
	pendingConsoles = append(pendingConsoles, c)
	return c
}

// FlushAll flushes all unflushed consoles. Intended to be called
// once before program exit (e.g. defer cmd.FlushAll() in main).
func FlushAll() {
	for _, c := range pendingConsoles {
		if !c.flushed {
			c.Flush() //nolint:errcheck // best-effort at exit
		}
	}
	pendingConsoles = nil
}

// Render outputs a named value.
//
// renderer can be:
//   - RenderFunc — called with the Printer and value for rich/raw modes
//   - string — Go text/template executed with the value for rich/raw modes
//   - nil — JSON-only entry (no rich/raw output)
//
// In JSON mode, the renderer is ignored and the value is collected for Flush().
//
// Template functions available when renderer is a string:
//
//	{{ header "Title" }}                                     section header
//	{{ dim "text" }}                                         dimmed text
//	{{ bold "text" }}                                        bold text
//	{{ warn "text" }}                                        warning text
//	{{ ok "text" }}                                          success text
//	{{ error "text" }}                                       error text
//	{{ row "label" "value" }}                                label-value pair
//	{{ table .Items "Name" "Type:FSType" "~Source" "Desc" }} auto-width table
//	{{ count .Items }}                                       len of slice
//	{{ join .Slice ", " }}                                   strings.Join
//	{{ nl }}                                                 blank line
//
// Table column specs (each argument after the data slice defines one column):
//
//	"Field"           — use struct field name as both header and data source
//	"Display:Field"   — custom header, data from struct field
//	"~Field"          — dim-styled column (struct field name as header)
//	"~Display:Field"  — dim-styled column with custom header
//
// The first non-dim column gets ValueStyle. Struct fields are accessed via
// reflection; string slices are joined with ", ". Data must be a slice of structs.
//
// In raw mode, all style functions output plain text.
func (c *Console) Render(name string, v any, renderer any) error {
	if c.mode == OutputJSON {
		c.entries = append(c.entries, consoleEntry{name: name, value: v})
		return nil // collected; emitted on Flush()
	}

	switch r := renderer.(type) {
	case RenderFunc:
		return r(c.printer, v)
	case func(*Printer, any) error:
		return r(c.printer, v)
	case string:
		if r == "" {
			// No template — fall back to pretty-JSON so rich/raw modes
			// produce SOMETHING readable instead of an empty stream.
			// Verbs that want a richer layout pass a Go text/template.
			return c.renderJSONFallback(v)
		}
		return c.renderTemplate(r, v)
	case nil:
		// Nil renderer means "JSON-only". In rich/raw mode that would
		// produce no output — fall back to pretty-JSON so the user
		// always sees the data. JSON mode already collects entries and
		// emits on Flush(), unchanged.
		return c.renderJSONFallback(v)
	default:
		return fmt.Errorf("console: renderer must be RenderFunc or string, got %T", renderer)
	}
}

// renderJSONFallback emits the value as pretty-printed JSON to the
// console's writer. Used as a "no template available" fallback so
// non-JSON modes always produce SOME readable output. JSON mode goes
// through the entries-collect path in Render instead — Flush composes
// the final document.
func (c *Console) renderJSONFallback(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("console: marshal JSON fallback: %w", err)
	}
	_, err = fmt.Fprintln(c.w, string(data))
	return err
}

// Flush completes the output. Idempotent — safe to call multiple times.
//
// In JSON mode: single entry emits the value directly,
// multiple entries emit a keyed object { "name1": v1, "name2": v2 }.
//
// In rich/raw mode: no-op (rendering already happened in Render calls).
func (c *Console) Flush() error {
	if c.flushed {
		return nil
	}
	c.flushed = true

	if c.mode != OutputJSON || len(c.entries) == 0 {
		return nil
	}

	var v any
	if len(c.entries) == 1 {
		v = c.entries[0].value
	} else {
		// Ordered JSON object: use json.RawMessage to preserve insertion order.
		buf := []byte("{")
		for i, e := range c.entries {
			if i > 0 {
				buf = append(buf, ',')
			}
			key, _ := json.Marshal(e.name)
			val, err := json.Marshal(e.value)
			if err != nil {
				return fmt.Errorf("console: marshal %q: %w", e.name, err)
			}
			buf = append(buf, '\n')
			buf = append(buf, key...)
			buf = append(buf, ':')
			buf = append(buf, val...)
		}
		buf = append(buf, '\n', '}')
		v = json.RawMessage(buf)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("console: marshal JSON: %w", err)
	}
	fmt.Fprintln(c.w, string(data))
	return nil
}

// Mode returns the current output mode.
func (c *Console) Mode() OutputMode { return c.mode }

// P returns the Printer for direct use in custom rendering logic.
func (c *Console) P() *Printer { return c.printer }

// renderTemplate executes a Go text/template with mode-aware functions.
func (c *Console) renderTemplate(tmpl string, data any) error {
	raw := c.mode == OutputRaw
	funcMap := buildConsoleFuncMap(raw)

	t, err := template.New("console").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("parse console template: %w", err)
	}
	return t.Execute(c.w, data)
}

// RenderConsole is a shortcut: NewConsole(mode) + Render(name, v, renderer) + Flush().
//
// renderer can be:
//   - RenderFunc — custom rendering logic
//   - string — Go text/template (with mode-aware functions: header, dim, bold, warn, ok, row, table, count, join, nl)
//   - nil — JSON-only (no rich/raw output)
func RenderConsole(mode OutputMode, name string, v any, renderer any) error {
	c := NewConsole(mode, os.Stdout)
	if err := c.Render(name, v, renderer); err != nil {
		return err
	}
	return c.Flush()
}

func buildConsoleFuncMap(raw bool) template.FuncMap {
	style := func(render func(string) string, text string) string {
		if raw {
			return text
		}
		return render(text)
	}

	return template.FuncMap{
		"header": func(title string) string {
			var sb strings.Builder
			sb.WriteString("\n")
			if raw {
				sb.WriteString("  " + title + "\n")
			} else {
				sb.WriteString(TitleStyle.Render("  "+title) + "\n")
			}
			sb.WriteString("\n")
			return sb.String()
		},
		"dim": func(text string) string {
			return style(func(s string) string { return DimStyle.Render(s) }, "  "+text) + "\n"
		},
		"bold": func(text string) string {
			return style(func(s string) string { return TitleStyle.Render(s) }, text)
		},
		"warn": func(text string) string {
			return style(func(s string) string { return WarnStyle.Render(s) }, "  "+text) + "\n"
		},
		"ok": func(text string) string {
			return style(func(s string) string { return OkStyle.Render(s) }, "  "+text) + "\n"
		},
		"error": func(text string) string {
			return style(func(s string) string { return ErrorStyle.Render(s) }, "  "+text) + "\n"
		},
		"row": func(label, value string) string {
			if raw {
				if value == "" {
					value = "(not set)"
				}
				return fmt.Sprintf("  %-22s%s\n", label, value)
			}
			if value == "" {
				value = DimStyle.Render("(not set)")
			} else {
				value = ValueStyle.Render(value)
			}
			return "  " + LabelStyle.Render(label) + value + "\n"
		},
		"nl": func() string {
			return "\n"
		},
		"count": func(data any) int {
			v := reflect.ValueOf(data)
			if v.Kind() == reflect.Slice || v.Kind() == reflect.Map {
				return v.Len()
			}
			return 0
		},
		"join": func(items []string, sep string) string {
			return strings.Join(items, sep)
		},
		"table": makeTableFunc(raw),
	}
}

// makeTableFunc returns a template function that renders a slice of structs
// as a formatted table. Column specs: "Field", "Display:Field", or "~Field"/"~Display:Field" for dim.
func makeTableFunc(raw bool) func(data any, cols ...string) string {
	type colDef struct {
		Display string
		Field   string
		Dim     bool
	}

	return func(data any, cols ...string) string {
		var defs []colDef
		for _, spec := range cols {
			dim := false
			if strings.HasPrefix(spec, "~") {
				dim = true
				spec = spec[1:]
			}
			parts := strings.SplitN(spec, ":", 2)
			if len(parts) == 2 {
				defs = append(defs, colDef{Display: parts[0], Field: parts[1], Dim: dim})
			} else {
				defs = append(defs, colDef{Display: spec, Field: spec, Dim: dim})
			}
		}

		headers := make([]string, len(defs))
		for i, d := range defs {
			headers[i] = d.Display
		}

		t := NewTable(headers...)
		t.raw = raw

		// Apply column styles: first non-dim column gets ValueStyle, ~cols get DimStyle
		if !raw {
			for i, d := range defs {
				if d.Dim {
					t.SetColumnStyle(i, DimStyle)
				} else if i == 0 {
					t.SetColumnStyle(0, ValueStyle)
				}
			}
		}

		// Extract rows via reflection
		v := reflect.ValueOf(data)
		if v.Kind() == reflect.Slice {
			for i := 0; i < v.Len(); i++ {
				elem := v.Index(i)
				if elem.Kind() == reflect.Ptr {
					elem = elem.Elem()
				}
				row := make([]string, len(defs))
				for j, d := range defs {
					row[j] = extractField(elem, d.Field)
				}
				t.AddRow(row...)
			}
		}

		return t.Render()
	}
}

// extractField gets a struct field or map key value as a string.
// Handles string slices by joining with ", ".
// Supports both struct fields (FieldByName) and map[string]any keys (MapIndex).
func extractField(v reflect.Value, name string) string {
	// Handle map[string]any (from JSON unmarshal)
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if v.Kind() == reflect.Map && v.Type().Key().Kind() == reflect.String {
		key := v.MapIndex(reflect.ValueOf(name))
		if !key.IsValid() {
			return ""
		}
		return fmt.Sprintf("%v", key.Interface())
	}

	f := v.FieldByName(name)
	if !f.IsValid() {
		return ""
	}

	switch f.Kind() {
	case reflect.Slice:
		if f.Type().Elem().Kind() == reflect.String {
			parts := make([]string, f.Len())
			for i := 0; i < f.Len(); i++ {
				parts[i] = f.Index(i).String()
			}
			return strings.Join(parts, ", ")
		}
		return fmt.Sprintf("%v", f.Interface())
	default:
		return fmt.Sprintf("%v", f.Interface())
	}
}
