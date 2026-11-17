// SPDX-License-Identifier: Apache-2.0

// Package testworld gives a test package of the marketplace a world of its
// own and guards the person's installation while its tests run
// (RQ-064): the global root, the data, the cache, the
// workspace and the registry of installations are temporary folders before
// any test runs, so a test that resolves the application's environment
// before setting its own root (the environment is cached) reaches that world,
// never ~/.config/katyoti - where a run of SetupMachine once replaced the
// control plane's katyoti.exe.
//
// The guard: the person's files are stat'ed (never read) before and after
// the run, and the run fails when one changed, or when the root the
// package's environment resolves at the end lies outside the temporary
// folder.
package testworld

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// Run is a test package's TestMain body: os.Exit(testworld.Run(m)).
func Run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "marketplace-test-world")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mktemp:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	watched := personFiles()
	before := stamps(watched)
	for k, v := range map[string]string{
		"KTW_KATYOTI_ROOT":            filepath.Join(dir, "root"),
		"KTW_DATA":                    filepath.Join(dir, "data"),
		"KTW_CACHE":                   filepath.Join(dir, "cache"),
		"KTW_WORKSPACE":               filepath.Join(dir, "workspace"),
		controlplane.InstallationsEnv: filepath.Join(dir, "katyoti-instances.yaml"),
	} {
		_ = os.Setenv(k, v)
	}
	application.ResetGlobals()
	code := m.Run()
	if root := application.Env().GlobalHomeDir(); root != "" && !within(root, os.TempDir()) {
		fmt.Fprintf(os.Stderr, "GUARD: the package's tests resolved the global root %s, outside a temporary folder\n", root)
		code = 1
	}
	for _, p := range watched {
		if after := stamp(p); after != before[p] {
			fmt.Fprintf(os.Stderr, "GUARD: a test of this package changed the person's %s (%s -> %s)\n", p, before[p], after)
			code = 1
		}
	}
	return code
}

// personFiles are the person's installation files a test of this package
// could write: the default global root's registries and control plane, and
// the registry of installations.
func personFiles() []string {
	var out []string
	root := ""
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		root = filepath.Join(x, "katyoti")
	} else if h, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(h, ".config", "katyoti")
	}
	if root != "" {
		for _, n := range []string{"marketplaces.yaml", "infra-channel.yaml", "sidecars.yaml", "universes.yaml", "control-plane", filepath.Join("control-plane", "katyoti.exe"), filepath.Join("control-plane", "katyoti"), filepath.Join("bin", "store")} {
			out = append(out, filepath.Join(root, n))
		}
	}
	if c, err := os.UserConfigDir(); err == nil {
		out = append(out, filepath.Join(c, "katyoti-instances.yaml"))
	}
	return out
}

func stamps(paths []string) map[string]string {
	out := map[string]string{}
	for _, p := range paths {
		out[p] = stamp(p)
	}
	return out
}

// stamp is a file's size and modification time, "absent" when there is none.
func stamp(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return "absent"
	}
	return fmt.Sprintf("%d@%s", fi.Size(), fi.ModTime().Format(time.RFC3339Nano))
}

func within(p, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
