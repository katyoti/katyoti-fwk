// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// home writes a .katyoti config root with allow_overwrites=allow, and (when
// withDir) the per-sidecar CWM overlay dir. Returns the home path.
func cwmHome(t *testing.T, allow, withDir bool) string {
	t.Helper()
	cenv.ResetUniverseManifestCache()
	t.Cleanup(cenv.ResetUniverseManifestCache)
	home := t.TempDir()
	cfg := filepath.Join(home, "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	m := "name: tv\n"
	if allow {
		m += "allow_overwrites: true\n"
	}
	if err := os.WriteFile(filepath.Join(cfg, "Univers.yaml"), []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
	if withDir {
		d := filepath.Join(cenv.OverwritesDir(home, "ktw-foo"), "embed", "cwm", "node_types")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "x.yaml"), []byte("name: x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestCwmOverlayFS_GateOff(t *testing.T) {
	if fsys := OverlayFS(cwmHome(t, false, true), "ktw-foo"); fsys != nil {
		t.Fatal("gate off must yield nil overlay even when the dir exists")
	}
}

func TestCwmOverlayFS_GateOnNoDir(t *testing.T) {
	if fsys := OverlayFS(cwmHome(t, true, false), "ktw-foo"); fsys != nil {
		t.Fatal("no overlay dir must yield nil")
	}
}

func TestCwmOverlayFS_GateOnWithDir(t *testing.T) {
	fsys := OverlayFS(cwmHome(t, true, true), "ktw-foo")
	if fsys == nil {
		t.Fatal("gate on + dir present must yield a non-nil overlay FS")
	}
	// The FS is rooted at embed/cwm, so node_types/x.yaml is reachable.
	f, err := fsys.Open("node_types/x.yaml")
	if err != nil {
		t.Fatalf("overlay FS must expose node_types/x.yaml: %v", err)
	}
	f.Close() // release the handle so Windows TempDir cleanup can unlink it
}

func TestCwmOverlayFS_EmptyArgs(t *testing.T) {
	if OverlayFS("", "ktw-foo") != nil || OverlayFS("/x", "") != nil {
		t.Fatal("empty homeDir or provider must yield nil")
	}
}
