// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

func hubExe() string {
	if runtime.GOOS == "windows" {
		return "ktw-hub.exe"
	}
	return "ktw-hub"
}

// TestResolveHubBinary_PrefersControlPlane: an installed control-plane ktw-hub
// wins over the dev/PATH fallbacks — the same first-choice rule as ResolveGUI.
func TestResolveHubBinary_PrefersControlPlane(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	want, err := controlplane.TargetPath("ktw-hub")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("hub"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveHubBinary("")
	if err != nil {
		t.Fatalf("resolveHubBinary: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Errorf("resolveHubBinary = %q, want the control-plane copy %q", got, want)
	}
}

// TestResolveHubBinary_ExplicitOverrideWins: an explicit HubBinaryPath beats the
// installed control-plane copy (the dev escape hatch).
func TestResolveHubBinary_ExplicitOverrideWins(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	cp, err := controlplane.TargetPath("ktw-hub")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cp, []byte("control-plane"), 0o755); err != nil {
		t.Fatal(err)
	}

	override := filepath.Join(t.TempDir(), hubExe())
	if err := os.WriteFile(override, []byte("override"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveHubBinary(override)
	if err != nil {
		t.Fatalf("resolveHubBinary(override): %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(override) {
		t.Errorf("resolveHubBinary = %q, want the override %q", got, override)
	}
}

// TestResolveHubBinary_NoControlPlane_FallsThrough: with no control-plane copy the
// resolver falls through to the existing dev/PATH chain (or a clear not-found).
func TestResolveHubBinary_NoControlPlane_FallsThrough(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv("PATH", t.TempDir()) // empty dir so LookPath finds nothing
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	got, err := resolveHubBinary("")
	if err == nil {
		// A repo-root cwd legitimately has ./bin/ktw-hub — that's the dev path.
		if filepath.Base(got) != hubExe() {
			t.Errorf("unexpected resolution %q", got)
		}
		return
	}
	if !errors.Is(err, ErrHubBinaryNotFound) {
		t.Errorf("expected ErrHubBinaryNotFound, got %v", err)
	}
}

// TestResolveHubBinary_EnvOverridesControlPlane: KTW_HUB_BINARY runs a freshly
// built hub over the installed control plane's — a session or a test proving
// a hub change on a machine that has one installed.
func TestResolveHubBinary_EnvOverridesControlPlane(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	installed, err := controlplane.TargetPath("ktw-hub")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("hub"), 0o755); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(t.TempDir(), hubExe())
	if err := os.WriteFile(fresh, []byte("hub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(HubBinaryEnv, fresh)
	got, err := resolveHubBinary("")
	if err != nil || filepath.Clean(got) != filepath.Clean(fresh) {
		t.Fatalf("resolveHubBinary = %q, %v; want the KTW_HUB_BINARY one %q", got, err, fresh)
	}
}
