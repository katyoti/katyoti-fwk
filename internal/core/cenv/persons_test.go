// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"testing"
)

// TestATestNeverResolvesThePersonsPlaces -: a test process
// that resolves the global root, the fallback workspace or a path of the
// person's installations gets the same path under a root of its own, and a
// write there leaves the person's folder untouched. The person's places are
// a temporary HOME here, so the proof writes nothing of the real ones.
func TestATestNeverResolvesThePersonsPlaces(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config")
	programs := filepath.Join(home, "AppData", "Local", "Programs", "Katyoti")
	kept := personsPlaces
	personsPlaces = []string{config, programs}
	t.Cleanup(func() { personsPlaces = kept })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("KTW_KATYOTI_ROOT", "")
	t.Setenv("KTW_WORKSPACE", "")

	persons := filepath.Join(config, "katyoti")
	root := GlobalAgentsRoot("katyoti")
	if _, ok := under(root, persons); ok {
		t.Fatalf("the global root %s lies in the person's %s", root, persons)
	}
	if _, ok := under(root, TestRoot()); !ok {
		t.Fatalf("the global root %s is not under the test's own root %s", root, TestRoot())
	}
	if ws := ResolveWorkspaceDir("katyoti"); !isUnder(ws, TestRoot()) {
		t.Errorf("the fallback workspace %s is not under the test's own root", ws)
	}
	// A root named explicitly as the person's is moved too.
	t.Setenv("KTW_KATYOTI_ROOT", persons)
	if got := GlobalAgentsRoot("katyoti"); !isUnder(got, TestRoot()) {
		t.Errorf("KTW_KATYOTI_ROOT naming the person's root resolved to %s", got)
	}
	if got := KeepTestsOut(filepath.Join(programs, "dev")); !isUnder(got, TestRoot()) {
		t.Errorf("an installation folder under Programs/Katyoti resolved to %s", got)
	}

	// The write a careless test makes lands in the test's root, never the person's.
	reg, err := defaultRegistryPath("katyoti")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(TestRoot()) })
	if err := os.MkdirAll(filepath.Dir(reg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reg, []byte("universes: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(persons); !os.IsNotExist(err) {
		t.Fatalf("the person's %s was written (stat: %v)", persons, err)
	}

	// Any other folder is the test's own and stays as given.
	other := filepath.Join(t.TempDir(), "root")
	t.Setenv("KTW_KATYOTI_ROOT", other)
	if got := GlobalAgentsRoot("katyoti"); got != filepath.Clean(other) {
		t.Errorf("a test's own root %s resolved to %s", other, got)
	}
}

// TestATestNeverResolvesThePersonsDataRoot -: a test process
// resolves the data root of its own. Its XDG_DATA_HOME names TestDataHome
// (set at init, inherited by the programs it starts), and a data root named
// explicitly as the person's (KTW_DATA, XDG_DATA_HOME = %LOCALAPPDATA%) is
// moved under the test's root - so no test leaves universes/<u> folders in
// the person's %LOCALAPPDATA%\katyoti. The person's data home is a temporary
// folder here. Falsified: without KeepTestsOut in ResolveDataDir, the data
// root lies in the person's.
func TestATestNeverResolvesThePersonsDataRoot(t *testing.T) {
	if got := os.Getenv("XDG_DATA_HOME"); got != TestDataHome() && !isUnder(got, os.TempDir()) {
		t.Errorf("a test process's XDG_DATA_HOME is %q, want its own (%s)", got, TestDataHome())
	}
	local := filepath.Join(t.TempDir(), "AppData", "Local")
	persons := filepath.Join(local, "katyoti")
	keptPlaces, keptHomes := personsPlaces, personsDataHomes
	personsPlaces, personsDataHomes = []string{persons}, []string{local}
	t.Cleanup(func() { personsPlaces, personsDataHomes = keptPlaces, keptHomes })

	t.Setenv("KTW_DATA", "")
	t.Setenv("XDG_DATA_HOME", local)
	if got := ResolveDataDir("katyoti"); isUnder(got, persons) || !isUnder(got, TestRoot()) {
		t.Errorf("XDG_DATA_HOME naming the person's data home resolved the data root to %s", got)
	}
	t.Setenv("KTW_DATA", filepath.Join(persons, "universes", "plan-test"))
	if got := ResolveDataDir("katyoti"); isUnder(got, persons) || !isUnder(got, TestRoot()) {
		t.Errorf("KTW_DATA naming a folder of the person's data root resolved to %s", got)
	}
	if !isPersonsDataHome(local) || isPersonsDataHome(TestDataHome()) {
		t.Errorf("the person's data home is not told from the test's")
	}
}

// TestAReusedPidStartsOnAnEmptyTestRoot -: a test process
// clears the test root of its pid when it starts, so a pid the system reuses
// never inherits an earlier process's stale scheduler.db.
func TestAReusedPidStartsOnAnEmptyTestRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "katyoti-test-root", "4242")
	stale := filepath.Join(root, "data-home", "katyoti", "universes", "no-universe", "scheduler", "scheduler.db")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	clearTestRoot(root)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("the stale %s survived the clearing (stat: %v)", stale, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("the test root %s survived the clearing (stat: %v)", root, err)
	}
}
