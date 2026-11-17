// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
)

// Every `.dsl.yaml` a sidecar SHIPS must parse.
//
// Until this existed, nothing in the suite read them: they are registered at
// ktw-hub boot by schemalocal.RegisterDataTypes, which reports a failed one as
// a count in a log line. So a malformed shipped schema was a green test run and
// a data-type silently missing from the catalog at runtime — the same shape as
// the inert object-type declarations this test's neighbours were written for.
//
// The corpus test next door proves the PARSER against fixtures. This proves the
// CORPUS WE SHIP against the parser, which is the direction that catches an
// authoring mistake.
func TestShippedSchemasParse(t *testing.T) {
	up := filepath.Join("..", "..", "..", "..", "..")
	appDir := filepath.Join(up, "app")
	if _, err := os.Stat(appDir); err != nil {
		t.Fatalf("app dir not reachable from test cwd: %v", err)
	}

	var files []string
	err := filepath.WalkDir(appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".dsl.yaml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", appDir, err)
	}
	// A guard that finds nothing passes for the wrong reason. The corpus is
	// dozens of files; zero means the walk broke, not that the schemas are fine.
	if len(files) < 10 {
		t.Fatalf("found only %d shipped .dsl.yaml files — the walk is wrong, not the corpus", len(files))
	}

	for _, f := range files {
		rel, _ := filepath.Rel(appDir, f)
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			out, err := parser.ParseFile(context.Background(), schemaapi.ParseFileInput{Path: f})
			if err != nil {
				t.Fatalf("does not parse: %v", err)
			}
			if out.Schema == nil {
				t.Fatal("parsed to a nil schema")
			}
			if out.Schema.Name == "" {
				t.Error("declares no name — nothing can reference it")
			}
			// The root names the type the rest of the file hangs off. Without it
			// a consumer has no entry point, and the file registers as an empty
			// data-type rather than failing.
			if out.Schema.Root == "" {
				t.Error("declares no root type")
			} else if _, ok := out.Schema.Types[out.Schema.Root]; !ok {
				t.Errorf("root %q is not among the declared types", out.Schema.Root)
			}
		})
	}
}
