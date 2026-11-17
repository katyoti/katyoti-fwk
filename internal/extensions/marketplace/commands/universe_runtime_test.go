// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// hubBinary is the built ktw-hub, found from the module root; the test is
// skipped without it (`make ktw-hub`).
func hubBinary(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	bin := "ktw-hub"
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	for dir := wd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if p := filepath.Join(dir, "bin", bin); fileExists(p) {
			return p
		}
	}
	t.Skip("hub binary not built (run `make ktw-hub` first)")
	return ""
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// A UNIVERSE STARTED AND STOPPED WITHOUT THE GUI: start runs
// the hub detached and the command returns with it running; status says so,
// with its pid; a second start finds it instead of starting another; stop
// ends it and removes its lockfile; status then says stopped. Real hub,
// skipped in -short mode. The universe is a bare folder: it has no member to
// serve it, so start reports that ktw-agents did not answer — the hub runs
// regardless.
func TestUniverseStartStatusStop(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	binary := hubBinary(t)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".katyoti", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".katyoti", "config", "Univers.yaml"), []byte("name: start-test\nsidecars: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_DATA", t.TempDir())
	dataDir := application.EnvOf(home).DataDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t.Cleanup(func() { _, _ = hubclient.Stop(context.Background(), dataDir) })

	var out bytes.Buffer
	spawned, err := startUniverse(ctx, home, runtimeOptions{HubBinaryPath: binary, MemberWait: 3 * time.Second}, &out)
	if err != nil {
		t.Fatalf("start: %v\n%s", err, out.String())
	}
	if !spawned || !strings.Contains(out.String(), "universe started") {
		t.Fatalf("the first start did not start the hub:\n%s", out.String())
	}
	st := statusOf(ctx, home)
	if st.Hub == nil || !st.Reaching {
		t.Fatalf("after start, status = %+v, want a running, reachable hub", st)
	}
	pid := st.Hub.PID

	// A second start finds it.
	out.Reset()
	spawned, err = startUniverse(ctx, home, runtimeOptions{HubBinaryPath: binary, MemberWait: time.Second}, &out)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if spawned || !strings.Contains(out.String(), "already running") {
		t.Fatalf("the second start started another hub:\n%s", out.String())
	}
	if st = statusOf(ctx, home); st.Hub == nil || st.Hub.PID != pid {
		t.Fatalf("after the second start, status = %+v, want the same hub %d", st, pid)
	}

	// Stop ends it and its lockfile.
	out.Reset()
	if err := stopUniverse(ctx, home, &out); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !strings.Contains(out.String(), "universe stopped") {
		t.Fatalf("stop said:\n%s", out.String())
	}
	if hubclient.ProcessAlive(pid) {
		t.Fatalf("the hub (pid %d) survived stop", pid)
	}
	if lf, _ := hubclient.ReadLockFile(dataDir); lf != nil {
		t.Fatalf("stop left the lockfile: %+v", lf)
	}
	if st = statusOf(ctx, home); st.Hub != nil || st.Stale {
		t.Fatalf("after stop, status = %+v, want stopped", st)
	}
	out.Reset()
	if err := stopUniverse(ctx, home, &out); err != nil || !strings.Contains(out.String(), "not running") {
		t.Fatalf("a second stop: err=%v out=%s", err, out.String())
	}
}

// A UNIVERSE RESTARTED IN ONE ACTION: restart of a stopped
// universe starts it; restart of a running one ends its hub and starts
// another - nobody kills a process by hand. Real hub on a temporary
// universe, skipped in -short mode.
func TestUniverseRestartIsOneAction(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	binary := hubBinary(t)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".katyoti", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".katyoti", "config", "Univers.yaml"), []byte("name: restart-test\nsidecars: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_DATA", t.TempDir())
	dataDir := application.EnvOf(home).DataDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t.Cleanup(func() { _, _ = hubclient.Stop(context.Background(), dataDir) })
	opts := runtimeOptions{HubBinaryPath: binary, MemberWait: time.Second}

	var out bytes.Buffer
	if err := restartUniverse(ctx, home, opts, &out); err != nil {
		t.Fatalf("restart of a stopped universe: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "not running") || !strings.Contains(out.String(), "universe started") {
		t.Fatalf("restart of a stopped universe should start it:\n%s", out.String())
	}
	st := statusOf(ctx, home)
	if st.Hub == nil {
		t.Fatalf("after restart, status = %+v, want running", st)
	}
	first := st.Hub.PID

	out.Reset()
	if err := restartUniverse(ctx, home, opts, &out); err != nil {
		t.Fatalf("restart: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "universe stopped") || !strings.Contains(out.String(), "universe started") {
		t.Fatalf("restart should stop then start:\n%s", out.String())
	}
	if hubclient.ProcessAlive(first) {
		t.Fatalf("the first hub (pid %d) survived the restart", first)
	}
	if st = statusOf(ctx, home); st.Hub == nil || st.Hub.PID == first || !st.Reaching {
		t.Fatalf("after restart, status = %+v, want another reachable hub than %d", st, first)
	}
}
