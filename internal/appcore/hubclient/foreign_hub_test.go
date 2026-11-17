// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// : a program outside an installation (bin/ktw-gui in the dev
// repository) that would spawn that installation's hub is refused at once,
// with the installation named and how to launch from it - never a 90 s wait
// on a hub that drops the universe it is given.
func TestAHubOfAnotherInstallationIsRefusedAtOnce(t *testing.T) {
	root := t.TempDir()
	inst := cenv.NewInstallation(root, "katyoti-dev", "")
	if err := cenv.SaveInstallation(inst); err != nil {
		t.Fatal(err)
	}
	hub := filepath.Join(root, "root", "control-plane", "ktw-hub")
	if runtime.GOOS == "windows" {
		hub += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(hub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hub, []byte("not run"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := connectSpawn(context.Background(), ConnectOptions{DataDir: t.TempDir(), HubBinaryPath: hub})
	if !errors.Is(err, ErrForeignHub) {
		t.Fatalf("spawn = %v; want ErrForeignHub", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the refusal took %s: it must come at once", took)
	}
	for _, want := range []string{"katyoti-dev", "katyoti gui"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
}
