// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// The hub service the GUI reads and writes: the projects folder is proposed
// until chosen; a folder inside the installation is refused and nothing is
// written; a folder outside it is created and read back.
func TestTheProjectsFolderIsReadAndChosenThroughTheService(t *testing.T) {
	dir := t.TempDir()
	instDir := filepath.Join(dir, "inst")
	if err := os.MkdirAll(instDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inst := cenv.NewInstallation(instDir, "demo 1.0", "demo@1.0")
	if err := cenv.SaveInstallation(inst); err != nil {
		t.Fatal(err)
	}
	old := currentInstallation
	currentInstallation = func() *cenv.Installation { return inst }
	t.Cleanup(func() { currentInstallation = old })
	ctx := context.Background()
	s := &service{}

	out, err := s.ProjectsFolder(ctx)
	if err != nil || out.Chosen || out.Folder != cenv.DefaultProjectsFolder() || out.Installation != instDir {
		t.Fatalf("before: %+v %v, want the proposed folder, not chosen", out, err)
	}
	inside := filepath.Join(inst.DataRoot(), "projects")
	if _, err := s.SetProjectsFolder(ctx, mktapi.SetProjectsFolderInput{Folder: inside}); err == nil {
		t.Fatal("a projects folder inside the installation's data was accepted")
	}
	if _, err := os.Stat(inside); err == nil {
		t.Error("the refused folder was created")
	}
	want := filepath.Join(dir, "projects")
	out, err = s.SetProjectsFolder(ctx, mktapi.SetProjectsFolderInput{Folder: want})
	if err != nil || !out.Chosen || out.Folder != want {
		t.Fatalf("set: %+v %v", out, err)
	}
	if st, err := os.Stat(want); err != nil || !st.IsDir() {
		t.Errorf("the chosen folder was not created: %v", err)
	}
	back, err := cenv.LoadInstallation(inst.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got, chosen := back.ProjectsFolder(); !chosen || got != want {
		t.Errorf("katyoti.yml holds %s (chosen %v), want %s", got, chosen, want)
	}
}
