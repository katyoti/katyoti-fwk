// SPDX-License-Identifier: Apache-2.0

// Package hubharness spawns a `ktw-hub serve` subprocess for tests that
// need a running hub (since the 2026-06-05 cache redesign, ktw-agents
// fetches catalog data via `ktw-hub:/CatalogService/*` over the bus —
// no fallback — so tests that exercise ktw-agents end-to-end MUST start
// a hub first).
//
// Usage:
//
//	func TestSomething(t *testing.T) {
//	    cleanup := hubharness.Start(t, dataDir)
//	    defer cleanup() // optional — `Start` also registers t.Cleanup
//	    // … tests that spawn ktw-agents …
//	}
//
// One hub per dataDir. Concurrent tests using DIFFERENT dataDirs each
// get their own hub on a separate embedded NATS port. Concurrent tests
// using the SAME dataDir would conflict — serialize them.
//
// The hub runs from the test process's own copy of bin/ktw-hub[.exe]
// (Binary), never from bin/, and ends with the test process (Tie).
//
// Skips the calling test (via t.Skip) when bin/ktw-hub[.exe] isn't
// built — keeps CI/local checkouts that haven't done `make ktw-hub`
// from failing loudly. Build the binary explicitly if your test MUST
// run.
package hubharness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
)

// startupBudget is how long Start waits for the hub to become reachable.
// See the note at the wait loop: this covers catalog hydration (one spawn per
// member sidecar), not just the embedded NATS server.
const startupBudget = 90 * time.Second

// Start spawns `ktw-hub serve` with the given `dataDir` and waits for
// the lockfile to materialize (proof the hub is reachable). Returns a
// cleanup function the caller can defer; `Start` also registers it via
// `t.Cleanup` so a forgotten defer doesn't leak the hub process.
//
// Skips the test if `bin/ktw-hub[.exe]` isn't built. Fatals if the
// process fails to start or the lockfile doesn't appear within 10 s.
func Start(t *testing.T, dataDir string) func() {
	t.Helper()

	// The test's own copy of the hub (Binary): bin/ktw-hub stays free for a
	// build to replace while the hub runs.
	binary := Binary(t, "ktw-hub")
	if binary == "" {
		MissingBinary(t, "hubharness: ktw-hub binary not found")
		return func() {} // never reached after Skip, but satisfies the return contract
	}

	// Fresh state — remove any stale lockfile so we can distinguish
	// "ours" from a previous run's leftover.
	_ = os.Remove(hubclient.LockFilePath(dataDir))

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "serve")
	cmd.Env = append(os.Environ(), "KTW_DATA="+dataDir)
	// The hub caches every member under CacheDir. A test that does not pin
	// KTW_CACHE would otherwise write into the developer's real cache and read
	// records from it (found by ms-contribution-ext e19, 2026-09-25); pin it
	// beside the data dir unless the test chose one.
	if os.Getenv("KTW_CACHE") == "" {
		cmd.Env = append(cmd.Env, "KTW_CACHE="+filepath.Join(dataDir, "cache"))
	}
	// Capture stderr/stdout for diagnosis if the hub crashes during
	// startup; piped into the test's log on cleanup.
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	// The hub and every process it starts end with the test process, even
	// when that is killed before its cleanup runs.
	if err := Tie(cmd); err != nil {
		cancel()
		t.Fatalf("hubharness: tie the hub to the test process: %v", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("hubharness: start %s serve: %v", binary, err)
	}

	// Drain pipes to prevent the subprocess from blocking when its
	// stdout/stderr buffers fill. The harness doesn't display the
	// stream — debug-on-failure surfacing is via the cmd.Wait error
	// captured in the cleanup func.
	// KEEP the output instead of discarding it. A hub that fails to start
	// used to report only "lockfile did not appear within 10s", which names
	// the symptom and nothing else — the reason was on a stream the harness
	// was throwing away. Every minute spent guessing at that was a minute the
	// hub had already explained itself and nobody was listening.
	var log tail
	go log.drain(stdout)
	go log.drain(stderr)

	// Wait for the lockfile — the hub writes it once NATS + KTSP + catalog are
	// ready to serve.
	//
	// The budget used to be 10 s, justified by "the local-NATS embedded server
	// usually publishes in < 1 s" — which measures the wrong thing. NATS is
	// ready in milliseconds; what follows is CATALOG HYDRATION, which spawns
	// every member sidecar. On a machine that scans each unsigned binary on
	// first execution, six cold spawns alone exceed the old budget, so the
	// suite failed with a timeout that said nothing about the code under test.
	//
	// This is a startup wait, not a latency assertion — nothing here is
	// measuring how fast a hub SHOULD boot. Generous, so that a real hang is
	// still caught but a slow machine is not reported as a defect.
	//
	// Ready means the lockfile is READABLE and parses, not that it exists.
	// The wait used to be an os.Stat: the file appeared, the caller went on
	// to ConnectToHub, and its read hit ERROR_SHARING_VIOLATION — on this
	// machine an on-access scanner holds a file the hub has just renamed into
	// place for a moment (once in ten runs under load, 2026-09-26, e29 H6).
	// ReadLockFile now waits out such a hold itself; here it is also the
	// readiness signal, so a caller never sees a lockfile this harness has
	// not read.
	lockPath := hubclient.LockFilePath(dataDir)
	deadline := time.Now().Add(startupBudget)
	var lf *hubclient.LockFile
	var readErr error
	for time.Now().Before(deadline) {
		if lf, readErr = hubclient.ReadLockFile(dataDir); lf != nil {
			break
		}
		// If the process already exited (crashed), bail out early
		// with the exit reason rather than spinning until timeout.
		if cmd.ProcessState != nil {
			cancel()
			t.Fatalf("hubharness: ktw-hub exited before writing lockfile: %v\n--- hub output ---\n%s",
				cmd.ProcessState, log.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lf == nil {
		cancel()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("hubharness: hub failed to write a readable lockfile %s within %s (last read: %v)\n--- hub output ---\n%s",
			lockPath, startupBudget, readErr, log.String())
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			// The members the hub spawned are listed while it still lives: a
			// killed hub's members outlive it (Windows ends no child with its
			// parent) and hold files in the test's folders — the run owner's
			// lock under <data>/runs/owners — so the folders' removal would fail.
			members := descendants(cmd.Process.Pid)
			// Cancel the ctx so CommandContext kills the hub (TerminateProcess
			// on Windows), then end its members and wait for every one to exit.
			cancel()
			_ = cmd.Wait()
			if alive := stopAll(members); len(alive) > 0 {
				t.Errorf("hubharness: processes the hub started outlive it %s after being stopped: %v", memberStopWait, alive)
			}
			// Best-effort lockfile cleanup — if the hub's PersistentPostRun
			// fired, it already removed it.
			_ = os.Remove(lockPath)
			// A failing test gets the hub's own account of the run; without it
			// a client-side error ("slot not found") is all the evidence there is.
			if t.Failed() {
				t.Logf("--- hub output (%s) ---\n%s", dataDir, log.String())
			}
		})
	}
	t.Cleanup(cleanup)
	return cleanup
}

// drain reads from r until EOF, discarding everything. Used to prevent
// the subprocess from blocking on a full pipe buffer.
// tail keeps the last few KiB a subprocess wrote, so a startup failure can
// print the reason instead of only the timeout. Bounded, because a hub that
// starts fine goes on to log for the whole test.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

const tailMax = 16 << 10

func (t *tail) drain(r interface {
	Read(p []byte) (int, error)
}) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.buf = append(t.buf, buf[:n]...)
			if len(t.buf) > tailMax {
				t.buf = t.buf[len(t.buf)-tailMax:]
			}
			t.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.buf) == 0 {
		return "(the hub wrote nothing)"
	}
	return strings.TrimSpace(string(t.buf))
}

// MakeTempUniverse creates a temp dataDir with the minimum config a hub
// needs to start: a Univers.yaml with `name:` and (optionally) the
// `sidecars:` block. For tests that don't need real sidecars (catalog
// stays empty), the default empty list is fine.
//
// Returns the dataDir path; the dir is removed when the test ends (TempDir).
func MakeTempUniverse(t *testing.T, sidecarsYAML string) string {
	t.Helper()
	dataDir := TempDir(t)
	cfgDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("hubharness: mkdir config: %v", err)
	}
	manifest := fmt.Sprintf(`version: 1
name: %s
display_name: %s
icon: T
color: gray
sidecars:%s
`, t.Name(), t.Name(), sidecarsYAML)
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("hubharness: write Univers.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("# hubharness test universe\n"), 0o644); err != nil {
		t.Fatalf("hubharness: write Settings.yaml: %v", err)
	}
	return dataDir
}

// SidecarBin is one (name, binaryPath) tuple for
// `MakeTempUniverseWithSidecars`. Args are stdio-mode defaults; tests
// that need custom args build the Univers.yaml string manually via
// `MakeTempUniverse`.
type SidecarBin struct {
	Name   string
	Binary string // absolute path to the sidecar's exe
}

// MakeTempUniverseWithSidecars creates a temp dataDir whose Univers.yaml
// includes the given sidecars as stdio-mode entries. Use when the test
// needs cross-binary catalog content (e.g. ktw-agents looking up tasks
// shipped by ktw-demo) — the hub's auto-hydration will spawn each
// listed sidecar at boot, populate its cache entry under
// `<CacheDir>/sidecars/<name>/`, and serve the manifest via
// `ktw-hub:/CatalogService/GetManifest`.
//
// Each sidecar's `binary:` must point at a real exe; absent or stale
// binaries cause the auto-hydration to log warnings but never fail
// boot. To force a strict version match, build everything fresh
// BEFORE running the test (e.g. via `make ktw-agents ktw-demo`).
//
// Skips the test if any listed binary is missing — keeps a fresh
// clone (without `make`) from failing loudly.
func MakeTempUniverseWithSidecars(t *testing.T, sidecars []SidecarBin) string {
	t.Helper()
	for _, sc := range sidecars {
		if _, err := os.Stat(sc.Binary); err != nil {
			MissingBinary(t, "hubharness: sidecar binary %s missing (%v)", sc.Name, err)
			return ""
		}
	}
	var block string
	for _, sc := range sidecars {
		// transport: stdio so the connmgr spawns each on demand via
		// `<binary> sc ktsp stdio`. Args omitted — sidecarkit binaries
		// add `sc ktsp stdio` themselves when run via stdio mode.
		block += fmt.Sprintf("\n  - name: %s\n    transport: stdio\n    binary: %s\n",
			sc.Name, sc.Binary)
	}
	return MakeTempUniverse(t, block)
}

// TempDir is a test's folder, removed when the test ends. Its removal retries
// for a while, where t.TempDir's gives up at once: on Windows a file a
// stopping process (a sidecar the hub spawned, a closing application, an
// on-access scanner) still holds leaves the folder "in use" for a moment.
// One still there after the wait fails the test — a process that outlives its
// test is a leak.
func TempDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ktw-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(10 * time.Second)
		for {
			err := os.RemoveAll(dir)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("the folder %s outlives the test: %v", dir, err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	return dir
}
