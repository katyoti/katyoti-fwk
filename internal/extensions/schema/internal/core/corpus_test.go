// SPDX-License-Identifier: Apache-2.0

// Package core_test runs the corpus snapshot tests for the parser+normalizer.
//
// Each entry in testdata/corpus/ is a (input.yaml, input.expected.json) pair.
// The test parses the YAML, marshals it through MarshalCanonicalJSONIndent,
// and asserts the bytes match the expected JSON. Set UPDATE_CORPUS=1 to
// regenerate the expected files after intentional AST changes.
package core_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
)

func TestCorpus(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.yaml"))
	if err != nil {
		t.Fatalf("glob corpus: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no corpus *.yaml files found")
	}
	for _, in := range matches {
		name := strings.TrimSuffix(filepath.Base(in), ".yaml")
		t.Run(name, func(t *testing.T) {
			runCorpusEntry(t, in)
		})
	}
}

func runCorpusEntry(t *testing.T, inputPath string) {
	t.Helper()
	expectedPath := strings.TrimSuffix(inputPath, ".yaml") + ".expected.json"

	out, err := parser.ParseFile(context.Background(), schemaapi.ParseFileInput{Path: inputPath})
	if err != nil {
		t.Fatalf("ParseFile %s: %v", inputPath, err)
	}
	got, err := out.Schema.MarshalCanonicalJSONIndent("  ")
	if err != nil {
		t.Fatalf("MarshalCanonicalJSONIndent: %v", err)
	}
	// always end with a single trailing newline for git friendliness.
	if len(got) == 0 || got[len(got)-1] != '\n' {
		got = append(got, '\n')
	}

	if os.Getenv("UPDATE_CORPUS") == "1" {
		if err := os.WriteFile(expectedPath, got, 0o644); err != nil {
			t.Fatalf("write expected: %v", err)
		}
		t.Logf("UPDATE_CORPUS: wrote %s (%d bytes)", expectedPath, len(got))
		return
	}

	want, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected %s: %v (run UPDATE_CORPUS=1 to generate)", expectedPath, err)
	}
	want = normalizeNewlines(want)
	got = normalizeNewlines(got)
	if !bytes.Equal(want, got) {
		t.Errorf("AST snapshot mismatch for %s\nwant:\n%s\ngot:\n%s",
			filepath.Base(inputPath), want, got)
	}
}

// normalizeNewlines converts CRLF → LF so Windows checkouts pass.
func normalizeNewlines(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}
