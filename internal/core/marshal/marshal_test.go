// SPDX-License-Identifier: Apache-2.0

package marshal_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// --- FormatFor ---

func TestFormatFor(t *testing.T) {
	tests := []struct {
		input string
		want  marshal.Format
		ok    bool
	}{
		{"yaml", marshal.YAML, true},
		{".yaml", marshal.YAML, true},
		{"config/settings.yaml", marshal.YAML, true},
		{"docs/README.md", marshal.Frontmatter, true},
		{"data.json", marshal.JSON, true},
		{"notes.txt", marshal.Text, true},
		{"YML", marshal.YAML, true},
		{"Makefile", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		f, ok := marshal.FormatFor(tt.input)
		if ok != tt.ok {
			t.Errorf("FormatFor(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && f != tt.want {
			t.Errorf("FormatFor(%q) = %q, want %q", tt.input, f, tt.want)
		}
	}
}

// --- YAML ---

func TestUnmarshal_YAML(t *testing.T) {
	data := []byte("name: test\nport: 8080\n")
	var target struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
	}
	if err := marshal.Unmarshal(data, marshal.YAML, &target); err != nil {
		t.Fatal(err)
	}
	if target.Name != "test" || target.Port != 8080 {
		t.Errorf("got %+v", target)
	}
}

func TestUnmarshalMap_YAML(t *testing.T) {
	data := []byte("key: value\n")
	m, err := marshal.UnmarshalMap(data, marshal.YAML)
	if err != nil {
		t.Fatal(err)
	}
	if m["key"] != "value" {
		t.Errorf("got %v", m)
	}
}

func TestMarshal_YAML(t *testing.T) {
	data := map[string]any{"key": "value"}
	b, err := marshal.Marshal(data, marshal.YAML)
	if err != nil {
		t.Fatal(err)
	}
	// Round-trip check.
	m, err := marshal.UnmarshalMap(b, marshal.YAML)
	if err != nil {
		t.Fatal(err)
	}
	if m["key"] != "value" {
		t.Errorf("round-trip failed: %v", m)
	}
}

// --- JSON ---

func TestUnmarshal_JSON(t *testing.T) {
	data := []byte(`{"name":"test","port":8080}`)
	var target struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}
	if err := marshal.Unmarshal(data, marshal.JSON, &target); err != nil {
		t.Fatal(err)
	}
	if target.Name != "test" || target.Port != 8080 {
		t.Errorf("got %+v", target)
	}
}

func TestUnmarshalMap_JSON(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	m, err := marshal.UnmarshalMap(data, marshal.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if m["key"] != "value" {
		t.Errorf("got %v", m)
	}
}

func TestMarshal_JSON(t *testing.T) {
	data := map[string]any{"key": "value"}
	b, err := marshal.Marshal(data, marshal.JSON)
	if err != nil {
		t.Fatal(err)
	}
	m, err := marshal.UnmarshalMap(b, marshal.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if m["key"] != "value" {
		t.Errorf("round-trip failed: %v", m)
	}
}

// --- Frontmatter ---

func TestUnmarshal_Frontmatter(t *testing.T) {
	data := []byte("---\ntitle: Hello\n---\nBody content here")
	m := make(map[string]any)
	if err := marshal.Unmarshal(data, marshal.Frontmatter, &m); err != nil {
		t.Fatal(err)
	}
	if m["title"] != "Hello" {
		t.Errorf("title = %v, want Hello", m["title"])
	}
	if m["content"] != "\nBody content here" {
		t.Errorf("content = %q", m["content"])
	}
}

func TestUnmarshal_Frontmatter_NoFrontmatter(t *testing.T) {
	data := []byte("Just plain text")
	m := make(map[string]any)
	if err := marshal.Unmarshal(data, marshal.Frontmatter, &m); err != nil {
		t.Fatal(err)
	}
	if m["content"] != "Just plain text" {
		t.Errorf("content = %q", m["content"])
	}
}

func TestMarshal_Frontmatter(t *testing.T) {
	data := map[string]any{"title": "Test", "content": "\nHello world"}
	b, err := marshal.Marshal(data, marshal.Frontmatter)
	if err != nil {
		t.Fatal(err)
	}
	// Round-trip.
	m := make(map[string]any)
	if err := marshal.Unmarshal(b, marshal.Frontmatter, &m); err != nil {
		t.Fatal(err)
	}
	if m["title"] != "Test" {
		t.Errorf("title = %v", m["title"])
	}
}

// --- Text ---

func TestUnmarshal_Text(t *testing.T) {
	data := []byte("Hello world")
	m := make(map[string]any)
	if err := marshal.Unmarshal(data, marshal.Text, &m); err != nil {
		t.Fatal(err)
	}
	if m["content"] != "Hello world" {
		t.Errorf("content = %q", m["content"])
	}
}

func TestMarshal_Text(t *testing.T) {
	data := map[string]any{"content": "Hello world"}
	b, err := marshal.Marshal(data, marshal.Text)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "Hello world" {
		t.Errorf("got %q", string(b))
	}
}

// --- CodecFor / flexible format ---

func TestCodecFor(t *testing.T) {
	// Direct format constant.
	c, err := marshal.CodecFor(marshal.YAML)
	if err != nil || c == nil {
		t.Fatalf("CodecFor(YAML) failed: %v", err)
	}

	// Extension with dot.
	c, err = marshal.CodecFor(".json")
	if err != nil || c == nil {
		t.Fatalf("CodecFor(.json) failed: %v", err)
	}

	// Full path.
	c, err = marshal.CodecFor("config/settings.yml")
	if err != nil || c == nil {
		t.Fatalf("CodecFor(config/settings.yml) failed: %v", err)
	}

	// Unknown.
	_, err = marshal.CodecFor("xml")
	if err == nil {
		t.Fatal("CodecFor(xml) should fail")
	}
}

func TestUnmarshal_WithExtension(t *testing.T) {
	data := []byte(`{"name":"test"}`)
	var target struct {
		Name string `json:"name"`
	}
	if err := marshal.Unmarshal(data, ".json", &target); err != nil {
		t.Fatal(err)
	}
	if target.Name != "test" {
		t.Errorf("got %q", target.Name)
	}
}

func TestUnmarshal_WithPath(t *testing.T) {
	data := []byte("key: value\n")
	var m map[string]any
	if err := marshal.Unmarshal(data, "config/data.yaml", &m); err != nil {
		t.Fatal(err)
	}
	if m["key"] != "value" {
		t.Errorf("got %v", m)
	}
}

// --- Unsupported format ---

func TestUnmarshal_UnsupportedFormat(t *testing.T) {
	err := marshal.Unmarshal([]byte("data"), "xml", nil)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestMarshal_UnsupportedFormat(t *testing.T) {
	_, err := marshal.Marshal("data", "xml")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestKnownFormats(t *testing.T) {
	m := marshal.KnownFormats()
	if len(m) == 0 {
		t.Fatal("expected non-empty map")
	}
	if m["yaml"] != marshal.YAML {
		t.Error("yaml not in KnownFormats")
	}
}
