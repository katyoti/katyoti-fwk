// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// : an installation whose folder is gone is forgotten - left
// out of the list and of the registry file - while every installation whose
// folder is there stays, and none of those is refused as sharing a folder
// with a vanished one.
func TestAVanishedInstallationIsForgottenAndTheOthersKept(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	for _, f := range []string{a, b} {
		if err := os.MkdirAll(f, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	goneA := filepath.Join(dir, "gone-rc1")
	goneB := filepath.Join(dir, "deleted", "rc2")
	reg := filepath.Join(dir, "katyoti-instances.yaml")
	t.Setenv(InstallationsEnv, reg)
	all := []KnownInstallation{{Name: "a", Folder: a}, {Name: "gone-rc1", Folder: goneA}, {Name: "b", Folder: b}, {Name: "rc2", Folder: goneB}}
	if err := writeInstallations(all); err != nil {
		t.Fatal(err)
	}

	known, err := KnownInstallations()
	if err != nil {
		t.Fatal(err)
	}
	if names(known) != "a,b" {
		t.Fatalf("listed %s, want only the installations whose folder exists: a,b", names(known))
	}
	data, err := os.ReadFile(reg)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(data); strings.Contains(s, "gone-rc1") || strings.Contains(s, "rc2") || !strings.Contains(s, "name: a") || !strings.Contains(s, "name: b") {
		t.Fatalf("the registry was not pruned of the vanished ones only:\n%s", s)
	}

	// The two existing ones are each deletable (no "shared" with a ghost).
	for _, n := range []string{"a", "b"} {
		if _, err := PlanDelete(known, n, "", true); err != nil {
			t.Fatalf("PlanDelete(%s) = %v, want a plan", n, err)
		}
	}

	// A vanished folder shares nothing: an entry once nested in a (its folder
	// since deleted) does not make a refused as shared.
	withGhost := append([]KnownInstallation{}, known...)
	withGhost = append(withGhost, KnownInstallation{Name: "inner", Folder: filepath.Join(a, "inner-rc")})
	if _, err := PlanDelete(withGhost, "a", "", true); err != nil {
		t.Fatalf("PlanDelete(a) beside a vanished nested entry = %v, want a plan", err)
	}

	// Saving prunes too.
	if err := SaveInstallations(append(known, KnownInstallation{Name: "ghost", Folder: filepath.Join(dir, "ghost")})); err != nil {
		t.Fatal(err)
	}
	again, _ := readInstallations()
	if names(again) != "a,b" {
		t.Fatalf("saved %s, want a,b", names(again))
	}
}

// A folder on a volume that is not there (a drive unplugged) cannot be told
// gone: it is kept.
func TestAnInstallationOnAnAbsentVolumeIsKept(t *testing.T) {
	if filepath.Separator != '\\' {
		t.Skip("volumes are Windows drives")
	}
	for _, d := range "QRSTUVWXYZ" {
		root := string(d) + `:\`
		if _, err := os.Stat(root); err != nil {
			if cenv.Vanished(root + `katyoti\rc1`) {
				t.Fatalf("a folder on the absent drive %s was said gone", root)
			}
			return
		}
	}
	t.Skip("no absent drive letter to test with")
}

func names(list []KnownInstallation) string {
	var out []string
	for _, k := range list {
		out = append(out, k.Name)
	}
	return strings.Join(out, ",")
}

// : a test never writes the person's registry of installations.
// The test process names its own file (cenv's init), and a test that clears
// KTW_INSTANCES or names the person's file still resolves to a file of its
// own - checked before anything is written, so a falsified guard fails here
// without touching the person's file.
func TestATestNeverWritesThePersonsRegistry(t *testing.T) {
	if personsRegistry == "" {
		t.Skip("no user config folder")
	}
	before, beforeErr := os.Stat(personsRegistry)
	if p := os.Getenv(InstallationsEnv); p == "" || strings.EqualFold(filepath.Clean(p), personsRegistry) {
		t.Fatalf("the test process starts with KTW_INSTANCES=%q: want a file of its own", p)
	}
	for _, env := range []string{"", personsRegistry} {
		t.Setenv(InstallationsEnv, env)
		p, err := InstallationsRegistryPath()
		if err != nil {
			t.Fatal(err)
		}
		if strings.EqualFold(filepath.Clean(p), personsRegistry) {
			t.Fatalf("with KTW_INSTANCES=%q a test resolves to the person's registry %s", env, p)
		}
		if err := SaveInstallations([]KnownInstallation{{Name: "t", Folder: t.TempDir()}}); err != nil {
			t.Fatal(err)
		}
	}
	after, afterErr := os.Stat(personsRegistry)
	if (beforeErr == nil) != (afterErr == nil) || (beforeErr == nil && (before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()))) {
		t.Fatalf("the person's registry %s changed during the test", personsRegistry)
	}
	_ = os.Remove(cenv.TestInstancesRegistry())
}
