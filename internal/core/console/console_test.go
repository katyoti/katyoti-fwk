// SPDX-License-Identifier: Apache-2.0

package console

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestConsole_JSONSingleEntry(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(OutputJSON, &buf)

	data := map[string]string{"name": "test", "version": "1.0"}
	if err := c.Render("info", data, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}

	// Single entry: should output just the value, not wrapped in {"info": ...}
	var decoded map[string]string
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}
	if decoded["name"] != "test" {
		t.Errorf("name = %q, want %q", decoded["name"], "test")
	}
}

func TestConsole_JSONMultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(OutputJSON, &buf)

	c.Render("version", map[string]string{"v": "1.0"}, nil)
	c.Render("status", map[string]string{"ok": "true"}, nil)
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}

	// Multiple entries: should output {"version": ..., "status": ...}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}
	if _, ok := decoded["version"]; !ok {
		t.Error("missing 'version' key")
	}
	if _, ok := decoded["status"]; !ok {
		t.Error("missing 'status' key")
	}
}

func TestConsole_RenderFunc(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(OutputRaw, &buf)

	err := c.Render("test", "hello", RenderFunc(func(p *Printer, v any) error {
		_, err := buf.WriteString("rendered: " + v.(string))
		return err
	}))
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "rendered: hello" {
		t.Errorf("output = %q, want %q", buf.String(), "rendered: hello")
	}
}

func TestConsole_TemplateRenderer(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(OutputRaw, &buf)

	data := struct{ Name string }{"test-app"}
	err := c.Render("info", data, `Name: {{ .Name }}`)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "Name: test-app" {
		t.Errorf("output = %q, want %q", buf.String(), "Name: test-app")
	}
}

func TestConsole_FlushNoopForNonJSON(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(OutputRaw, &buf)

	c.Render("test", "data", RenderFunc(func(p *Printer, v any) error {
		return nil
	}))
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	// Flush should not produce additional output in raw mode
	if buf.String() != "" {
		t.Errorf("expected empty output after flush in raw mode, got %q", buf.String())
	}
}

func TestConsole_JSONIgnoresRenderer(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(OutputJSON, &buf)

	rendered := false
	c.Render("test", map[string]string{"k": "v"}, RenderFunc(func(p *Printer, v any) error {
		rendered = true
		return nil
	}))
	c.Flush()

	if rendered {
		t.Error("renderer should not be called in JSON mode")
	}
}

func TestConsole_FlushIdempotent(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(OutputJSON, &buf)

	c.Render("test", map[string]string{"k": "v"}, nil)
	c.Flush()
	first := buf.String()

	// Second flush should be a no-op
	c.Flush()
	if buf.String() != first {
		t.Errorf("second Flush produced extra output: %q", buf.String()[len(first):])
	}
}

func TestConsole_FlushAll(t *testing.T) {
	// Reset global state
	pendingConsoles = nil

	var buf bytes.Buffer
	c := NewConsole(OutputJSON, &buf)
	c.Render("test", map[string]string{"auto": "flushed"}, nil)

	// FlushAll should flush the unflushed console
	FlushAll()

	var decoded map[string]string
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}
	if decoded["auto"] != "flushed" {
		t.Errorf("auto = %q, want %q", decoded["auto"], "flushed")
	}
}
