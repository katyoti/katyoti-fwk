// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// An installation's projects folder is proposed in the person's documents
// until the person chooses one; one inside the installation (its folder or a
// root) is refused - an uninstall would take the projects with it - and the
// chosen one is read back from katyoti.yml.
func TestTheProjectsFolderIsChosenOutsideTheInstallation(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "inst"), 0o755); err != nil {
		t.Fatal(err)
	}
	inst := NewInstallation(filepath.Join(dir, "inst"), "demo 1.0", "demo@1.0")
	if err := SaveInstallation(inst); err != nil {
		t.Fatal(err)
	}
	if got, chosen := inst.ProjectsFolder(); chosen || got != filepath.Join(UserDocumentsDir(), "Katyoti") {
		t.Fatalf("projects folder %s (chosen %v), want <Documents>/Katyoti proposed", got, chosen)
	}
	for _, bad := range []string{
		filepath.Join(dir, "inst"),
		filepath.Join(dir, "inst", "projects"),
		filepath.Join(inst.DataRoot(), "projects"),
		"relative/projects",
	} {
		if err := inst.SetProjectsFolder(bad); err == nil {
			t.Errorf("%s accepted as the projects folder", bad)
		}
	}
	// The installation's folder written in other letter case is the same
	// folder only where the file system ignores case (Windows, as
	// filepath.Rel compares): refused there, another folder - accepted -
	// on a case-sensitive one (Linux).
	upper := strings.ToUpper(filepath.Join(dir, "inst", "data", "p"))
	if err := inst.SetProjectsFolder(upper); runtime.GOOS == "windows" && err == nil {
		t.Errorf("%s accepted as the projects folder: the same folder on a case-insensitive file system", upper)
	} else if runtime.GOOS != "windows" && err != nil {
		t.Errorf("%s refused on a case-sensitive file system, where it is another folder: %v", upper, err)
	}
	want := filepath.Join(dir, "my projects")
	if err := inst.SetProjectsFolder(want); err != nil {
		t.Fatal(err)
	}
	back, err := LoadInstallation(inst.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got, chosen := back.ProjectsFolder(); !chosen || got != want {
		t.Errorf("read back %s (chosen %v), want %s chosen", got, chosen, want)
	}
	// A program of no installation gets the proposed folder and sets none.
	var none *Installation
	if got, chosen := none.ProjectsFolder(); chosen || got != DefaultProjectsFolder() {
		t.Errorf("no installation: %s chosen %v", got, chosen)
	}
	if err := none.SetProjectsFolder(want); err == nil {
		t.Error("a projects folder set with no installation")
	}
}

// A new project goes to the projects folder under its name - the chosen one,
// else the one proposed - and a project folder inside the installation (its
// folder or a root) is refused: an uninstall or a reinstall would take it. A
// program of no installation refuses none and proposes <Documents>/Katyoti.
func TestANewProjectGoesToTheProjectsFolder(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstallation(filepath.Join(dir, "inst"), "demo 1.0", "demo@1.0")
	if got, want := inst.ProjectDir("archi"), filepath.Join(UserDocumentsDir(), "Katyoti", "archi"); got != want {
		t.Fatalf("project %s, want %s (the proposed projects folder)", got, want)
	}
	projects := filepath.Join(dir, "my projects")
	inst.Settings = map[string]any{ProjectsFolderSetting: projects}
	if got, want := inst.ProjectDir("archi"), filepath.Join(projects, "archi"); got != want {
		t.Fatalf("project %s, want %s (the chosen projects folder)", got, want)
	}
	for _, bad := range []string{
		filepath.Join(dir, "inst", "archi"),
		filepath.Join(inst.DataRoot(), "universes", "archi-project"),
		filepath.Join(inst.ConfigRoot(), "p"),
		filepath.Join(inst.CacheRoot(), "p"),
	} {
		if err := inst.CheckProjectDir(bad); err == nil || !strings.Contains(err.Error(), projects) {
			t.Errorf("%s accepted as a project folder (or the refusal does not name the projects folder): %v", bad, err)
		}
	}
	for _, ok := range []string{filepath.Join(projects, "archi"), filepath.Join(dir, "elsewhere", "p")} {
		if err := inst.CheckProjectDir(ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	// A relative folder is judged where it resolves: one that lands in the
	// installation's data is refused too.
	if err := os.MkdirAll(inst.DataRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(inst.DataRoot())
	if err := inst.CheckProjectDir("archi"); err == nil {
		t.Error("a relative project folder resolving in the installation's data accepted")
	}
	var none *Installation
	if err := none.CheckProjectDir(filepath.Join(dir, "inst", "p")); err != nil {
		t.Errorf("a program of no installation refuses no project folder: %v", err)
	}
	if got, want := none.ProjectDir("x"), filepath.Join(DefaultProjectsFolder(), "x"); got != want {
		t.Errorf("no installation: project %s, want %s", got, want)
	}
}
