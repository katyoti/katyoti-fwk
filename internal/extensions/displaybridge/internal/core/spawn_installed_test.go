// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// In an installation every binary lives in its own store folder, so the
// display server is never ktw-agents' sibling: the bridge takes the ktw-display
// a marketplace installed, from the installation's record of what it installed.
func TestTheBridgeFindsTheInstalledDisplayServer(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv("KTW_DISPLAY_BIN", "")
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	name := "ktw-display"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(root, "bin", "store", "ktw-display", "v1", name)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("bun"), 0o755); err != nil {
		t.Fatal(err)
	}
	inv := "version: 1\nsidecars:\n  - name: ktw-display\n    binary_path: " + filepath.ToSlash(bin) + "\n    source: marketplace\n    kind: sidecar\n"
	if err := os.WriteFile(filepath.Join(root, "sidecars.yaml"), []byte(inv), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := findCompiledBinary()
	if !ok || filepath.Clean(got) != filepath.Clean(bin) {
		t.Fatalf("findCompiledBinary = %q, %v; want the installed %s", got, ok, bin)
	}
}
