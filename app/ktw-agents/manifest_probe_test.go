// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/app/ktw-agents/cmd"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/proclease"
)

// probeChildEnv marks the re-executed test binary that plays the hub's probe.
const probeChildEnv = "KTW_TEST_MANIFEST_PROBE_CHILD"

// TestManifestProbeLeavesTheWorkspaceLock: the hub's probe of ktw-agents
// (`ktw-agents sc resources manifest`, appcore.RunResourcesList) describes the
// binary without opening its CWM workspace. While another live process holds
// the workspace lock — the serving ktw-agents — the probe used to fail (the
// CWM service, built at boot for its KTSP slot, was refused the lock and the
// boot with it); with nobody holding it, the probe took the lock itself and
// raced the member the hub was starting.
//
// The probe runs in a child process — the real command line, through
// cmd.Execute — because a process builds one application (TestCensus builds
// this package's).
func TestManifestProbeLeavesTheWorkspaceLock(t *testing.T) {
	if os.Getenv(probeChildEnv) == "1" {
		runManifestProbeChild()
		return
	}

	ws, data, cache := t.TempDir(), t.TempDir(), t.TempDir()
	home := application.EnvOf(ws).HomeDir()
	cfgDir := filepath.Join(home, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"Univers.yaml":  "name: probe-test\n",
		"Settings.yaml": "{}\n",
	} {
		if err := os.WriteFile(filepath.Join(cfgDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// This test process is the "other live process" holding the workspace.
	lockPath := filepath.Join(home, ".lock")
	lease := proclease.New(lockPath)
	if err := lease.Acquire(context.Background()); err != nil {
		t.Fatalf("hold the workspace lock: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })

	child := exec.Command(os.Args[0], "-test.run=^TestManifestProbeLeavesTheWorkspaceLock$")
	child.Dir = ws
	child.Env = append(os.Environ(),
		probeChildEnv+"=1",
		"KTW_WORKSPACE="+ws,
		"KTW_DATA="+data,
		"KTW_CACHE="+cache,
		"KTW_WORKSPACE_DIR=",
		"KTW_UNIVERSE=",
	)
	var stdout, stderr bytes.Buffer
	child.Stdout, child.Stderr = &stdout, &stderr
	if err := child.Run(); err != nil {
		t.Fatalf("the probe failed while another process held the workspace: %v\nstderr:\n%s", err, stderr.String())
	}

	out := stdout.Bytes()
	if i := bytes.IndexByte(out, '{'); i >= 0 {
		out = out[i:]
	}
	var m struct {
		Provider  string            `json:"provider"`
		SlotTypes []json.RawMessage `json:"slotTypes"`
	}
	if err := json.NewDecoder(bytes.NewReader(out)).Decode(&m); err != nil {
		t.Fatalf("probe output is not a manifest: %v\n%s", err, stdout.String())
	}
	if m.Provider != "ktw-agents" || len(m.SlotTypes) == 0 {
		t.Fatalf("probe manifest: provider %q with %d slot types; want ktw-agents with its slots", m.Provider, len(m.SlotTypes))
	}

	var rec struct {
		PID int `json:"pid"`
	}
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("the workspace lock is gone after the probe: %v", err)
	}
	if err := json.Unmarshal(raw, &rec); err != nil || rec.PID != os.Getpid() {
		t.Fatalf("the workspace lock changed hands during the probe: %s", raw)
	}
}

// runManifestProbeChild runs `ktw-agents sc resources manifest` exactly as the
// hub's probe does, then exits with the command's outcome.
func runManifestProbeChild() {
	os.Args = []string{os.Args[0], "sc", "resources", "manifest", "--include-system=false", "--include-local=false"}
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
