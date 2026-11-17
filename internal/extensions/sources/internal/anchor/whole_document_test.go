// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
)

// A sync anchors a whole document - no section path - and CWM accepts its
// body: the shipped source-document data-type does not require section_path
// (a whole Confluence page or LeanIX fact sheet was refused
// with « missing properties: section_path »).
func TestAWholeDocumentItemIsAnchored(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	dsl, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "contrib", "embed", "cwm", "data-types", "source-document.dsl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := parser.CompileJSONSchema(context.Background(), dsl)
	if err != nil {
		t.Fatalf("compile source-document: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("source-document.json", bytes.NewReader(compiled)); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("source-document.json")
	if err != nil {
		t.Fatal(err)
	}

	out, err := New().Ingest(context.Background(), nil, "confluence://wiki.example.com/pages/101", nil, "# Facturation\n\nLe SI de facturation.\n", "sources/source-sync")
	if err != nil {
		t.Fatalf("ingest a whole document: %v", err)
	}
	var body any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatal(err)
	}
	if m, _ := body.(map[string]any); m["section_path"] != nil {
		t.Fatalf("a whole document carries a section path: %v", m["section_path"])
	}
	if err := sch.Validate(body); err != nil {
		t.Fatalf("CWM would refuse the anchor of a whole document: %v", err)
	}
}
