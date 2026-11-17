// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

func TestDirAndTargetPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if filepath.Base(dir) != "control-plane" {
		t.Errorf("Dir = %q, want it to end in control-plane", dir)
	}

	target, err := TargetPath("ktw-gui")
	if err != nil {
		t.Fatalf("TargetPath: %v", err)
	}
	if filepath.Dir(target) != dir {
		t.Errorf("TargetPath dir = %q, want %q", filepath.Dir(target), dir)
	}
	wantBase := "ktw-gui"
	if runtime.GOOS == "windows" {
		wantBase += ".exe"
	}
	if filepath.Base(target) != wantBase {
		t.Errorf("TargetPath base = %q, want %q", filepath.Base(target), wantBase)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(target, ".exe") {
		t.Errorf("windows target should end in .exe: %q", target)
	}
}

func TestIsInfra(t *testing.T) {
	// ktw-univers is infra: it serves admin's management slots, moves as a set
	// with the hub it shares a KTSP version with, and cannot come from the
	// catalog because it is the service that serves the catalog.
	for _, n := range []string{"katyoti", "ktw-hub", "ktw-gui", "ktw-univers"} {
		if !IsInfra(n) {
			t.Errorf("IsInfra(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"ktw-demo", "ktw-example", ""} {
		if IsInfra(n) {
			t.Errorf("IsInfra(%q) = true, want false", n)
		}
	}
}

// TestSweep reaps `<binary>.old-*` files beside every control-plane binary while
// leaving the binaries themselves (and unrelated files) untouched.
func TestSweep(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(p string) {
		t.Helper()
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// one live binary + two stale asides for each infra name
	var targets []string
	for _, n := range InfraBinaries {
		tp, err := TargetPath(n)
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, tp)
		write(tp)
		write(tp + ".old-111")
		write(tp + ".old-222")
	}
	unrelated := filepath.Join(dir, "notes.txt")
	write(unrelated)

	if err := Sweep(); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	for _, tp := range targets {
		if m, _ := filepath.Glob(tp + ".old-*"); len(m) != 0 {
			t.Errorf("stale asides left for %s: %v", filepath.Base(tp), m)
		}
		if _, err := os.Stat(tp); err != nil {
			t.Errorf("Sweep removed the live binary %s: %v", filepath.Base(tp), err)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("Sweep removed an unrelated file: %v", err)
	}
}
