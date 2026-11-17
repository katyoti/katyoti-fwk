// SPDX-License-Identifier: Apache-2.0

package contribution_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
)

// A generated data-type ships as the file the Go type renders to — byte for
// byte. A stale file means the struct moved and nobody ran `go generate`:
// consumers would decode one shape while validation checked another, the
// exact drift a hand-written schema used to need a test for.
func TestGeneratedDataTypesAreCurrent(t *testing.T) {
	for _, name := range datatypegen.Names() {
		want, err := datatypegen.Render(name)
		if err != nil {
			t.Fatalf("%s: render: %v", name, err)
		}
		got, err := fs.ReadFile(contribution.SystemFS(), "embed/data-type/"+name+".yaml")
		if err != nil {
			t.Fatalf("%s: embedded file missing: %v (run go generate ./internal/core/contribution/)", name, err)
		}
		if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
			t.Errorf("embed/data-type/%s.yaml is stale — run `go generate ./internal/core/contribution/` (the Go type is the source)", name)
		}
	}
}

// The GUI's TypeScript for a data-type is rendered from the same AST: a stale
// file means the frontend reads a shape the Go side no longer sends.
func TestGeneratedTypeScriptIsCurrent(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "app", "ktw-gui", "frontend", "src", "generated")
	mt, err := os.ReadFile(filepath.Join(dir, datatypegen.MediaTypesTSName+".ts"))
	if err != nil || normalizeEOL(mt) != string(datatypegen.RenderMediaTypesTS()) {
		t.Errorf("frontend/src/generated/%s.ts is stale or missing — run `go generate ./internal/core/contribution/` (render's media-type table is the source)", datatypegen.MediaTypesTSName)
	}
	for _, name := range datatypegen.TSNames() {
		want, err := datatypegen.RenderTS(name)
		if err != nil {
			t.Fatalf("%s: render: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(dir, name+".ts"))
		if err != nil {
			t.Fatalf("%s: generated TypeScript missing: %v (run go generate ./internal/core/contribution/)", name, err)
		}
		if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
			t.Errorf("frontend/src/generated/%s.ts is stale — run `go generate ./internal/core/contribution/` (the Go type is the source)", name)
		}
	}
}

func normalizeEOL(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }
