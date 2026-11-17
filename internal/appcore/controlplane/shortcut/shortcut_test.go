// SPDX-License-Identifier: Apache-2.0

package shortcut

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTheNameCarriesTheInstallation(t *testing.T) {
	for in, want := range map[string]string{
		"acme-demo 1.004": "KaTyoti acme-demo 1.004",
		"katyoti 1.002":   "KaTyoti 1.002",
		"KATYOTI next":    "KaTyoti next",
		`a/b:c?`:          "KaTyoti a-b-c-",
	} {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

// Two installations, two shortcuts; removing one leaves the other.
func TestEachInstallationHasItsShortcut(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("shortcuts are made on Windows")
	}
	dir := t.TempDir()
	t.Setenv(DirEnv, dir)
	target, _ := os.Executable()
	for _, n := range []string{"acme 1.004", "acme 1.005"} {
		if _, err := Make(n, target, target); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"KaTyoti acme 1.004.lnk", "KaTyoti acme 1.005.lnk"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	Remove("acme 1.004")
	if _, err := os.Stat(filepath.Join(dir, "KaTyoti acme 1.004.lnk")); err == nil {
		t.Fatal("the removed installation's shortcut is still there")
	}
	if _, err := os.Stat(filepath.Join(dir, "KaTyoti acme 1.005.lnk")); err != nil {
		t.Fatal("removing one installation's shortcut removed another's")
	}
}
