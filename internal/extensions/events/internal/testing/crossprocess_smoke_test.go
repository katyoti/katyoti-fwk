// SPDX-License-Identifier: Apache-2.0

// Package testing holds the cross-process event-delivery smoke: it
// proves a CWM lifecycle event published by a hub-spawned ktw-agents
// core reaches a subscriber in ANOTHER process (this test) over the
// hub's NATS bus. This is the cross-sidecar leg every out-of-process
// consumer (other sidecars' EventService subscriptions, the GUI)
// depends on; the in-process legs are covered by unit tests.
//
// Self-contained: its own temp workspace + dataDir + hub + core —
// never touches the repo's live workspace and takes no CWM lock the
// developer might be holding.
package testing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

const smokeUniverse = "evtsmoke"

func TestCrossProcessObjectEventDelivery(t *testing.T) {
	agentsBin := findBinary(t, "ktw-agents")
	hubBin := findBinary(t, "ktw-hub")
	if agentsBin == "" || hubBin == "" {
		t.Skip("ktw-agents / ktw-hub binaries not built; skipping cross-process smoke")
	}

	// Temp workspace with its own Univers.yaml: universe name +
	// the ktw-agents sidecar in NATS transport so the hub's connmgr
	// can lazily spawn the serving core on the first slot call.
	ws := t.TempDir()
	cfgDir := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	manifest := fmt.Sprintf(`version: 1
name: %s
display_name: Event Smoke
icon: E
color: gray
sidecars:
    - name: ktw-agents
      transport: nats
      binary: %s
      args:
        - sc
        - ktsp
        - nats
`, smokeUniverse, filepath.ToSlash(agentsBin))
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write Univers.yaml: %v", err)
	}
	// ktw-agents refuses to serve from a workspace without Settings.yaml
	// (the "not configured yet" gate in root.go). A minimal file passes;
	// this smoke only exercises CWM slots, never LLM profiles.
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write Settings.yaml: %v", err)
	}
	dataDir := t.TempDir()

	// Spawn the hub with the temp workspace + dataDir. The core it
	// later spawns inherits this env, so its CWM bootstraps a fresh
	// workspace under <ws>/.katyoti and resolves the same universe
	// name for its event subjects.
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	hub := exec.CommandContext(hubCtx, hubBin, "serve")
	hub.Env = append(os.Environ(), "KTW_WORKSPACE="+ws, "KTW_DATA="+dataDir)
	// The hub forwards spawned sidecars' stderr onto its own, so this
	// one log carries both hub boot and core boot diagnostics.
	hubLog, err := os.Create(filepath.Join(dataDir, "hub-combined.log"))
	if err != nil {
		t.Fatalf("create hub log: %v", err)
	}
	hub.Stdout = hubLog
	hub.Stderr = hubLog
	if err := hubharness.Tie(hub); err != nil {
		t.Fatal(err)
	}
	if err := hub.Start(); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	// Join the spawned tree to a KILL_ON_JOB_CLOSE job owned by the
	// test process: hub, core, and their own children (embedded NATS,
	// ktw-display supervisor) all die with the test — no zombies even
	// on a panic or Ctrl-C.
	if err := publish.AttachToParentJob(hub.Process); err != nil {
		t.Logf("job attach (hub): %v — orphan possible on abnormal exit", err)
	}
	t.Cleanup(func() {
		hubCancel()
		_ = hub.Wait()
		_ = hubLog.Close()
		if t.Failed() {
			if raw, err := os.ReadFile(hubLog.Name()); err == nil {
				const tail = 6000
				if len(raw) > tail {
					raw = raw[len(raw)-tail:]
				}
				t.Logf("hub+core combined log tail:\n%s", raw)
			}
		}
	})

	lockPath := hubclient.LockFilePath(dataDir)
	waitFor(t, 15*time.Second, "hub lockfile", func() bool {
		_, err := os.Stat(lockPath)
		return err == nil
	})

	hc, err := hubclient.ConnectToHub(context.Background(), hubclient.ConnectOptions{
		DataDir: dataDir,
		Mode:    hubclient.ModeReuse,
	})
	if err != nil {
		t.Fatalf("connect hub: %v", err)
	}

	// RAW event subscription from THIS process — the same NATS leg an
	// EventService subscriber in another sidecar would use. Wildcards
	// across workspace + object token; we filter for ".created" below.
	events := make(chan string, 16)
	subject := "ktw.local.universe." + smokeUniverse + ".workspace.*.object.>"
	if _, err := hc.MessageBus().Subscribe(context.Background(), subject,
		func(_ context.Context, msg mbapi.Message) {
			events <- msg.Subject()
		}); err != nil {
		t.Fatalf("subscribe %s: %v", subject, err)
	}

	// One core per workspace: CWM takes a single-writer lock, so the
	// test must NOT spawn its own ktw-agents — the hub already spawns
	// one for the universe (catalog discovery + lazy ensure-spawned),
	// and a second core would deadlock on the workspace lock. Drive the
	// hub's core through a RetrySender: the first slot call triggers
	// ensure-spawned if discovery hasn't booted it yet, then retries.
	inner := messagebus.NewSender(hc.MessageBus(), "local", smokeUniverse)
	sender := messagebus.NewRetrySender(inner, "ktw-hub")
	waitFor(t, 90*time.Second, "core slot readiness (ktsp/ping)", func() bool {
		pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := sender.Call(pingCtx, "ktw-agents:ktsp/ping", map[string]any{})
		return err == nil
	})

	// File a request in the core's inbox — the object write publishes its
	// created event on the bus; assert it reaches THIS process.
	fileCtx, cancelFile := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFile()
	if _, err := sender.Call(fileCtx, "ktw-agents:inbox/file", map[string]any{
		"title":        "cross-process event smoke",
		"description":  "filed by the cross-process smoke test",
		"requested_by": "smoke",
	}); err != nil {
		t.Fatalf("inbox/file: %v", err)
	}

	deadline := time.After(20 * time.Second)
	for {
		select {
		case subj := <-events:
			if strings.HasSuffix(subj, ".created") && strings.Contains(subj, ".object.") {
				return // cross-process delivery proven
			}
		case <-deadline:
			t.Fatal("object.created event never reached the subscriber process")
		}
	}
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// findBinary answers the test's own copy of bin/<name> (hubharness.Binary):
// bin/ stays replaceable while the smoke runs.
func findBinary(t *testing.T, name string) string {
	t.Helper()
	return hubharness.Binary(t, name)
}
