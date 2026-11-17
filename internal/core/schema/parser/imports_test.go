// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// TestImports_TransitiveChain verifies a 3-file chain: parent imports child
// which imports grandchild. All types end up in the parent's AST.
func TestImports_TransitiveChain(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "grandchild.yaml", `$schema: ktw-schema/v1
name: grandchild
types:
  Z:
    type: object
`)
	writeFile(t, dir, "child.yaml", `$schema: ktw-schema/v1
name: child
imports:
  - ./grandchild.yaml
types:
  Y:
    type: object
`)
	parentPath := writeFile(t, dir, "parent.yaml", `$schema: ktw-schema/v1
name: parent
imports:
  - ./child.yaml
types:
  X:
    type: object
`)

	out, err := ParseFile(context.Background(), schemaapi.ParseFileInput{Path: parentPath})
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	for _, name := range []string{"X", "Y", "Z"} {
		if _, ok := out.Schema.Types[name]; !ok {
			t.Errorf("type %q missing after import resolution; types=%v", name, keys(out.Schema.Types))
		}
	}
	if len(out.Schema.Imports) != 0 {
		t.Errorf("Imports should be cleared after resolution, got %v", out.Schema.Imports)
	}
}

// TestImports_CycleDetection verifies a 3-file cycle (A→B→C→A) is caught
// with a useful error message naming the path.
func TestImports_CycleDetection(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "a.yaml", `$schema: ktw-schema/v1
name: a
imports: [./b.yaml]
types: {A: {type: object}}
`)
	writeFile(t, dir, "b.yaml", `$schema: ktw-schema/v1
name: b
imports: [./c.yaml]
types: {B: {type: object}}
`)
	writeFile(t, dir, "c.yaml", `$schema: ktw-schema/v1
name: c
imports: [./a.yaml]
types: {C: {type: object}}
`)

	_, err := ParseFile(context.Background(), schemaapi.ParseFileInput{Path: filepath.Join(dir, "a.yaml")})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "cycle") {
		t.Errorf("error message should mention 'cycle': %v", err)
	}
}

// TestImports_DuplicateTypeConflict verifies merging two imports that both
// declare a type with the same name returns an explicit error.
func TestImports_DuplicateTypeConflict(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "x1.yaml", `$schema: ktw-schema/v1
name: x1
types: {Conflict: {type: object}}
`)
	writeFile(t, dir, "x2.yaml", `$schema: ktw-schema/v1
name: x2
types: {Conflict: {type: object}}
`)
	parent := writeFile(t, dir, "parent.yaml", `$schema: ktw-schema/v1
name: parent
imports:
  - ./x1.yaml
  - ./x2.yaml
types: {}
`)
	_, err := ParseFile(context.Background(), schemaapi.ParseFileInput{Path: parent})
	if err == nil {
		t.Fatal("expected duplicate-type error, got nil")
	}
	if !strings.Contains(err.Error(), "Conflict") {
		t.Errorf("error should mention 'Conflict': %v", err)
	}
}

// TestImports_AbsoluteAndRelative verifies both relative and absolute import
// paths resolve correctly.
func TestImports_AbsoluteAndRelative(t *testing.T) {
	dir := t.TempDir()
	libPath := writeFile(t, dir, "lib.yaml", `$schema: ktw-schema/v1
name: lib
types: {Lib: {type: object}}
`)
	absLib, _ := filepath.Abs(libPath)

	parent := writeFile(t, dir, "parent.yaml", `$schema: ktw-schema/v1
name: parent
imports:
  - `+absLib+`
types: {App: {type: object}}
`)
	out, err := ParseFile(context.Background(), schemaapi.ParseFileInput{Path: parent})
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if _, ok := out.Schema.Types["Lib"]; !ok {
		t.Errorf("absolute import not merged; types=%v", keys(out.Schema.Types))
	}
}

// helpers

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
