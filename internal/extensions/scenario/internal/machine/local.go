// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// dropped are the variables a clean machine does not have: an installation's
// or a sandbox's locations, a workspace or universe, the installer's options,
// provider keys. The runner's own process may carry them (it runs in a
// case's sandbox); no stage inherits them.
var dropped = []string{"KTW_KATYOTI_ROOT", "KTW_DATA", "KTW_CACHE", "XDG_DATA_HOME", "KTW_HOME", "KTW_WORKSPACE",
	"KTW_WORKSPACE_DIR", "KTW_UNIVERSE", "KTW_INFRA_CHANNEL", "KTW_INSTALLATION", "KTW_INSTALLATION_DIR", "KTW_INSTANCE",
	"KTW_INSTANCES", "KTW_SHORTCUT_DIR", "KTW_SETUP_ARGS", "KTW_HUB", "XDG_CONFIG_HOME"}

// personal are the folders a local world moves into itself.
var personal = []string{"USERPROFILE", "HOME", "LOCALAPPDATA", "APPDATA"}

func droppedKey(k string) bool {
	u := strings.ToUpper(k)
	return slices.Contains(dropped, u) || strings.HasPrefix(u, "KTW_SCENARIO_") || strings.HasSuffix(u, "_API_KEY")
}

// Clean is base without what a clean machine does not have.
func Clean(base []string) []string {
	out := make([]string, 0, len(base))
	for _, kv := range base {
		if k, _, _ := strings.Cut(kv, "="); !droppedKey(k) {
			out = append(out, kv)
		}
	}
	return out
}

// MachineWorld is the machine the runner runs on, as it is (a Windows
// Sandbox): its home, its programs folder, nothing of the runner's own; each
// stage sees the PATH the programs installed before it wrote.
func MachineWorld(base []string) World {
	home, _ := os.UserHomeDir()
	return World{Env: Clean(base), Home: home, FreshPath: cenv.FreshPath}
}

// LocalWorld is this machine closed in dir: its home (and the AppData under
// it), its installation folder and its shortcuts folder, so nothing the
// installer writes lands outside dir. It runs each stage's local_cmd.
func LocalWorld(dir string, base []string) (World, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return World{}, err
	}
	home := filepath.Join(dir, "home")
	local, roaming := filepath.Join(home, "AppData", "Local"), filepath.Join(home, "AppData", "Roaming")
	for _, d := range []string{home, local, roaming, filepath.Join(home, "Documents")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return World{}, err
		}
	}
	env := []string{}
	for _, kv := range Clean(base) {
		k, _, _ := strings.Cut(kv, "=")
		if !slices.ContainsFunc(personal, func(p string) bool { return strings.EqualFold(p, k) }) {
			env = append(env, kv)
		}
	}
	// The registry of installations is named outright, in the world's own
	// config folder: an installer of the case never records itself in the
	// person's, whatever the platform's config folder is.
	env = append(env, "USERPROFILE="+home, "HOME="+home, "LOCALAPPDATA="+local, "APPDATA="+roaming,
		"KTW_SHORTCUT_DIR="+filepath.Join(dir, "shortcuts"),
		cenv.InstancesEnv+"="+filepath.Join(roaming, "katyoti-instances.yaml"))
	return World{Env: env, Home: home, Install: filepath.Join(dir, "install"), Local: true}, nil
}

// StopUnder stops every process running from an executable under dir: the
// hubs and members a local run left behind, so the next run can wipe it.
func StopUnder(ctx context.Context, dir string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		script := fmt.Sprintf("Get-Process | Where-Object { $_.Path -and $_.Path.StartsWith('%s', 'OrdinalIgnoreCase') } | Stop-Process -Force -ErrorAction SilentlyContinue", strings.ReplaceAll(dir, "'", "''"))
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	} else {
		cmd = exec.CommandContext(ctx, "pkill", "-f", dir)
	}
	_ = cmd.Run() // nothing to stop is not an error
}
