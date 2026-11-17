// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

func TestTheLauncherOpensTheCasesOwnGUI(t *testing.T) {
	src := t.TempDir()
	e := NewEngine(&api.Case{Name: "demo", Universe: api.CaseUniverse{Sidecars: []string{"ktw-agents"}}}, t.TempDir(), src, nil)
	for _, b := range []string{hubBinary, "ktw-agents", guiBinaryName} {
		if err := os.WriteFile(filepath.Join(src, exe(b)), []byte(b), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(e.RT.Bin(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.copyBinaries(); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(e.RT.Bin(), exe(guiBinaryName))
	if !fileExists(want) {
		t.Fatalf("the GUI was not copied with the members into %s", e.RT.Bin())
	}
	if got := e.guiBinary(); got != want {
		t.Errorf("the launcher opens %s, want the case's own build %s", got, want)
	}
}

func TestAFolderWithoutTheGUIStillInstalls(t *testing.T) {
	src := t.TempDir()
	e := NewEngine(&api.Case{Name: "demo"}, t.TempDir(), src, nil)
	if err := os.WriteFile(filepath.Join(src, exe(hubBinary)), []byte("hub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(e.RT.Bin(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.copyBinaries(); err != nil {
		t.Fatalf("the GUI is optional: %v", err)
	}
}
