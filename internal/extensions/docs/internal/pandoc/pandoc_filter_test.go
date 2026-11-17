// SPDX-License-Identifier: Apache-2.0

package pandoc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/diagram"
)

func TestPandocFilter_ReplacesCodeBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<svg>mock</svg>"))
	}))
	defer srv.Close()

	ast := map[string]any{
		"pandoc-api-version": []any{1.0, 23.0},
		"meta":               map[string]any{},
		"blocks": []any{
			map[string]any{
				"t": "CodeBlock",
				"c": []any{
					[]any{"", []any{"mermaid"}, []any{}},
					"graph TD; A-->B",
				},
			},
		},
	}

	input, _ := json.Marshal(ast)
	var output bytes.Buffer

	filter := &PandocFilter{
		Kroki:     diagram.NewKrokiRenderer(srv.URL),
		OutputDir: t.TempDir(),
		Format:    "html",
		ImgFormat: "svg",
	}

	if err := filter.RunFilter(bytes.NewReader(input), &output); err != nil {
		t.Fatalf("RunFilter: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	blocks, ok := result["blocks"].([]any)
	if !ok || len(blocks) == 0 {
		t.Fatal("expected blocks in output")
	}

	para, ok := blocks[0].(map[string]any)
	if !ok {
		t.Fatal("expected map for first block")
	}
	if para["t"] != "Para" {
		t.Errorf("block type = %v, want Para", para["t"])
	}
}

func TestPandocFilter_PreservesNonDiagramBlocks(t *testing.T) {
	ast := map[string]any{
		"pandoc-api-version": []any{1.0, 23.0},
		"meta":               map[string]any{},
		"blocks": []any{
			map[string]any{
				"t": "CodeBlock",
				"c": []any{
					[]any{"", []any{"python"}, []any{}},
					"print('hello')",
				},
			},
		},
	}

	input, _ := json.Marshal(ast)
	var output bytes.Buffer

	filter := &PandocFilter{
		Kroki:     diagram.NewKrokiRenderer("http://unused.invalid"),
		OutputDir: t.TempDir(),
		Format:    "html",
		ImgFormat: "svg",
	}

	if err := filter.RunFilter(bytes.NewReader(input), &output); err != nil {
		t.Fatalf("RunFilter: %v", err)
	}

	var result map[string]any
	json.Unmarshal(output.Bytes(), &result)
	blocks := result["blocks"].([]any)
	block := blocks[0].(map[string]any)

	if block["t"] != "CodeBlock" {
		t.Errorf("block type = %v, want CodeBlock (non-diagram preserved)", block["t"])
	}
}

func TestPandocFilter_TypstForcePNG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/png") {
			t.Errorf("expected /png in path, got %s", r.URL.Path)
		}
		w.Write([]byte("PNG data"))
	}))
	defer srv.Close()

	ast := map[string]any{
		"pandoc-api-version": []any{1.0, 23.0},
		"meta":               map[string]any{},
		"blocks": []any{
			map[string]any{
				"t": "CodeBlock",
				"c": []any{
					[]any{"", []any{"mermaid"}, []any{}},
					"graph TD; A-->B",
				},
			},
		},
	}

	input, _ := json.Marshal(ast)
	var output bytes.Buffer

	filter := &PandocFilter{
		Kroki:     diagram.NewKrokiRenderer(srv.URL),
		OutputDir: t.TempDir(),
		Format:    "typst",
		ImgFormat: "svg",
	}

	if err := filter.RunFilter(bytes.NewReader(input), &output); err != nil {
		t.Fatalf("RunFilter: %v", err)
	}
}

func TestOutputDirFromInputFile(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/docs/report.md", "/docs/report-kroki-images"},
		{"/docs/report.tex.md", "/docs/report-kroki-images"},
		{"report.md", "report-kroki-images"},
	}
	for _, tt := range tests {
		got := OutputDirFromInputFile(tt.input)
		got = strings.ReplaceAll(got, "\\", "/")
		if got != tt.want {
			t.Errorf("OutputDirFromInputFile(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
