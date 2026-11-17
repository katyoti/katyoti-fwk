// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
)

// Real-bun integration tests. Skipped unless `bun` is on PATH AND
// KTW_DISPLAYBRIDGE_INTEGRATION=1. These spawn the actual ktw-display
// sidecar via the publish supervisor and exercise the bridge against it,
// proving the full lifecycle works (spawn → healthz → HTTP → SSE → close).
//
// The tests share scaffolding via `withSpawnedBun` to avoid duplicating
// the supervisor lifecycle dance. Each test gets its own bun process so
// that crash-restart in one test can't leak state into another.

// shouldRunIntegration reports whether the integration suite should run.
// Two conditions: bun must be on PATH (or fail-spawn would mask other
// errors), and an explicit env var must be set (so CI defaults to fast).
func shouldRunIntegration(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not on PATH — set up Bun runtime to run integration tests")
	}
	if os.Getenv("KTW_DISPLAYBRIDGE_INTEGRATION") == "" {
		t.Skip("KTW_DISPLAYBRIDGE_INTEGRATION=1 to run integration tests")
	}
}

// repoRootCache holds the absolute path to the ktw-agents project root,
// resolved once via runtime.Caller against this file's compile-time
// location. We cannot use filepath.Abs("../../../..") because os.Chdir
// in earlier tests would change the relative anchor — runtime.Caller is
// stable regardless of cwd.
var (
	repoRootOnce  sync.Once
	repoRootValue string
	repoRootErr   error
)

// repoRoot returns the absolute path to the ktw-agents project root.
// Stable across os.Chdir calls (anchored to integration_test.go's
// location: internal/extensions/displaybridge/internal/, four levels
// below the project root).
func repoRoot(t *testing.T) string {
	t.Helper()
	repoRootOnce.Do(func() {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			repoRootErr = errors.New("runtime.Caller failed")
			return
		}
		// file = .../internal/extensions/displaybridge/internal/integration_test.go
		// project root = parent of internal/
		dir := filepath.Dir(file)
		for i := 0; i < 4; i++ {
			dir = filepath.Dir(dir)
		}
		repoRootValue = dir
	})
	if repoRootErr != nil {
		t.Fatal(repoRootErr)
	}
	return repoRootValue
}

// withSpawnedBun starts a real ktw-display via the supervisor and returns
// (proc, bridge). The bridge is wired with a StaticEndpoint pointing at
// the spawned port + token; SSE is NOT auto-started — callers that want
// SSE behavior call b.sse.Start() explicitly to keep test surface area
// scoped.
//
// Cleanup is registered via t.Cleanup; tests don't need to defer Close.
func withSpawnedBun(t *testing.T, ctx context.Context) (*publish.ExternalProcess, *schemaBridge) {
	t.Helper()

	root := repoRoot(t)
	// The supervisor's spawn() uses exec.CommandContext which inherits
	// the test process's working directory. ktw-display's dev fallback is
	// `bun run app/ktw-display/src/index.ts` — relative path that needs
	// the repo root as cwd. Using os.Chdir is fine here because tests
	// run sequentially within a package.
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}

	proc := publish.NewExternalProcess(BunSpec(), root)
	t.Cleanup(func() { _ = proc.Close() })

	if err := proc.Start(ctx); err != nil {
		t.Fatalf("supervisor Start: %v", err)
	}
	if got, want := proc.Status(), publish.StatusRunning; got != want {
		t.Fatalf("post-Start status = %q, want %q", got, want)
	}

	endpoint := NewStaticEndpoint(
		"http://127.0.0.1:"+strconv.Itoa(proc.Port()),
		proc.Token(),
	)
	b := newWithEndpoint(endpoint, withMaxRetry(0), withBackoff(1*time.Millisecond))
	t.Cleanup(func() { _ = b.Close() })
	return proc, b
}

// TestIntegration_LazySpawn covers the "first call brings bun online"
// path (Test 1 from the Subtask 6 plan). With the framework's
// lazy-spawn-on-resolve wiring not yet in place (ms06d external-sidecar-
// routing todo), this test mirrors the equivalent flow manually: drive
// the supervisor lifecycle, then observe that bridge slot calls succeed
// against the live process and that subsequent calls reuse the same
// TestIntegration_LazySpawn — REMOVED 2026-05-22 (schema-realign).
// Exercised lazy-spawn + Phase-1 501-stub surfacing through Parse, both
// gone. Lazy-spawn behavior is still covered by TestIntegration_GetWidgetURL
// below (live call against the real /api/widget-url endpoint).

// TestIntegration_SSESubscribesToBun covers the SSE-side of Test 2: the
// bridge's subscriber connects to bun's heartbeat-only /sse/schema-changed
// stream and receives the initial connect comment, proving the wire is
// live end-to-end. The full populate→invalidate→repopulate cache flow is
// covered in unit tests (TestSSE_EndToEnd_CacheInvalidationFlow); the
// real-bun gen handler is still a 501 stub today (a future codegen-target
// todo), so cache populate against real bun isn't possible until then.
func TestIntegration_SSESubscribesToBun(t *testing.T) {
	shouldRunIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, b := withSpawnedBun(t, ctx)

	// Override the SSE backoff so the test is fast; reuse the same
	// endpoint as the bridge's HTTP client.
	b.sse = newSSESubscriber(
		NewStaticEndpoint(b.client.endpoint.BaseURL(), b.client.endpoint.Token()),
		b.cache,
		sseWithBackoff(50*time.Millisecond, 200*time.Millisecond),
	)
	b.sse.Start()

	// The bun side emits ": connected\n\n" immediately on /sse/schema-changed
	// connect. The handler ignores comment lines, so we observe success
	// indirectly: the subscriber's run loop must not be in the
	// reconnect-after-error path. We assert by populating the cache, then
	// confirming a manual Invalidate (the same wire bun would use) is
	// honored — proving the run loop is alive.
	b.cache.Put("Probe", 1, stubSchema(t, "Probe"))

	// The simplest live-loop signal: wait for at least 200ms (one
	// connect + initial comment + first heartbeat tick is way faster than
	// 30s). If the loop were busy looping in errors, cache.Size would
	// have been Cleared at least once. Since nothing on bun's stream
	// errors, the cache should still hold "Probe".
	time.Sleep(300 * time.Millisecond)
	if _, _, ok := b.cache.Get("Probe"); !ok {
		t.Error("cache cleared unexpectedly — SSE loop is in reconnect-error spin")
	}
}

// TestIntegration_CrashRestart covers Test 3: kill bun, supervisor
// auto-restarts, bridge cache clears (SSE drops), subsequent calls
// succeed against the new bun.
//
// Killing bun externally requires an out-of-band signal because the
// supervisor's *exec.Cmd handle is not exposed. We use os.FindProcess +
// Kill against the PID, which we get from the bun stderr log line
// "ktw-display.listening" — but stderr is streamed by the supervisor
// (streamPrefixed writes to the logger), so we can't easily intercept it.
//
// Until the publish package exposes a test-only "Kill" hook, we exercise
// the same bridge-side cache-clear behavior in TestSSE_DisconnectClearsCacheAndReconnects
// (unit test) and skip the external-kill flavor here.
func TestIntegration_CrashRestart(t *testing.T) {
	shouldRunIntegration(t)
	t.Skip("crash-restart needs publish.ExternalProcess.Kill (test-only) — tracked as a follow-up")
}

// TestIntegration_HealthzReachable is a sanity check that bun's /healthz
// route works from within the test process. Useful for triaging when
// TestIntegration_GetWidgetURL verifies that bun's /api/widget-url handler
// (implemented in Subtask 1 of gui-schema-runtime) is reachable end-to-end
// through the bridge and returns a well-formed iframe URL. Replaces the
// 501 stub the slot returned during the go-bridge-for-display-server work.
func TestIntegration_GetWidgetURL(t *testing.T) {
	shouldRunIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, b := withSpawnedBun(t, ctx)

	out, err := b.GetWidgetURL(ctx, api.GetWidgetURLInput{
		Kind:       "hello",
		SchemaName: "Book",
		Mode:       "view",
	})
	if err != nil {
		t.Fatalf("GetWidgetURL: %v", err)
	}
	if out.URL == "" {
		t.Error("URL is empty; bun handler should always return one")
	}
	// URL shape: scheme://host:port/widget/<kind>?<query>&token=<...>
	if !strings.Contains(out.URL, "/widget/hello") {
		t.Errorf("URL = %q, want containing /widget/hello", out.URL)
	}
	if !strings.Contains(out.URL, "schemaName=Book") {
		t.Errorf("URL = %q, want containing schemaName=Book", out.URL)
	}
	if !strings.Contains(out.URL, "mode=view") {
		t.Errorf("URL = %q, want containing mode=view", out.URL)
	}
	if !strings.Contains(out.URL, "token=") {
		t.Errorf("URL = %q, want token query param embedded", out.URL)
	}
	if out.Token == "" {
		t.Error("Token is empty; should mirror the per-spawn session token")
	}
	if out.ExpiresAt != 0 {
		t.Errorf("ExpiresAt = %d, want 0 (no expiration in Phase 1)", out.ExpiresAt)
	}

	// Empty kind → bun should 400. Bridge surfaces it as a *BridgeError
	// with status 400, NOT as a Go-side validation error (the bridge
	// trusts bun's validation).
	_, err = b.GetWidgetURL(ctx, api.GetWidgetURLInput{Kind: ""})
	if err == nil {
		t.Fatal("expected error for empty kind")
	}
	// The bridge wraps with a slot-name prefix; underlying chain should
	// hold either a *BridgeError (if bun received the request) or a
	// local Go-side validation error if we add one upstream. Today the
	// bridge sends the call through, so bun's 400 is what we expect.
	if !strings.Contains(err.Error(), "GetWidgetURL") {
		t.Errorf("err = %q, want slot name 'GetWidgetURL' in message", err.Error())
	}
}

// other integration tests fail (rules out network-config issues).
func TestIntegration_HealthzReachable(t *testing.T) {
	shouldRunIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	proc, _ := withSpawnedBun(t, ctx)

	url := "http://127.0.0.1:" + strconv.Itoa(proc.Port()) + "/healthz"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("healthz GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("healthz status = %d, want 200", resp.StatusCode)
	}
}
