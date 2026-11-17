// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
)

// THE CASE'S OWN HUB — a case runs, and is looked at, only on the hub of its
// own bin folder: that is what "the same conditions" means. Another process can
// serve the case's universe — the GUI, activating it, spawns the installed
// control plane's hub — and that hub is someone else's binary. Start replaces
// it, Stop stops it.

// servingHub is the hub the case's lockfile names: its pid and executable path
// ("" when it cannot be told).
func (e *Engine) servingHub() (int, string) {
	lock, err := hubclient.ReadLockFile(e.RT.Data())
	if err != nil || lock == nil || lock.PID <= 0 {
		return 0, ""
	}
	return lock.PID, processPath(lock.PID)
}

// isOwnHub is whether a hub executable is the case's own: in its bin folder,
// or the copy in its sandbox's control plane (what a GUI opened in the
// sandbox spawns).
func (e *Engine) isOwnHub(path string) bool {
	if path == "" {
		return true // cannot tell: not replaced on a guess
	}
	for _, dir := range e.OwnFolders() {
		if within(dir, path) {
			return true
		}
	}
	return false
}

// OwnFolders are where the case's own programs run from: its bin folder and
// its sandbox's control plane.
func (e *Engine) OwnFolders() []string {
	return []string{e.RT.Bin(), e.Sandbox().ControlPlane()}
}

// within is whether path lies under dir.
func within(dir, path string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(abs, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// killTree stops a process and the processes it started (a hub's members).
func killTree(ctx context.Context, pid int) {
	if runtime.GOOS == "windows" {
		_ = exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
		return
	}
	_ = exec.CommandContext(ctx, "pkill", "-TERM", "-P", strconv.Itoa(pid)).Run()
	_ = exec.CommandContext(ctx, "kill", "-TERM", strconv.Itoa(pid)).Run()
}
