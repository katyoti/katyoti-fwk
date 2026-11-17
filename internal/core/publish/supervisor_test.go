// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// substitutor tests — pure-function, no side effects.

func TestSubstitutor_BuiltInVars(t *testing.T) {
	s := substitutor{
		homePath:     "/path/to/home",
		port:         "54321",
		sessionToken: "deadbeef",
	}
	cases := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"${KTW_WORKSPACE}", "/path/to/home"},
		{"${ALLOC_PORT}", "54321"},
		{"${SESSION_TOKEN}", "deadbeef"},
		{"http://127.0.0.1:${ALLOC_PORT}/healthz", "http://127.0.0.1:54321/healthz"},
		{"prefix-${KTW_WORKSPACE}-suffix", "prefix-/path/to/home-suffix"},
		{"${KTW_WORKSPACE}/${ALLOC_PORT}", "/path/to/home/54321"},
		{"unterminated ${VAR", "unterminated ${VAR"},
	}
	for _, c := range cases {
		got := s.expand(c.input)
		if got != c.want {
			t.Errorf("expand(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestSubstitutor_FallsThroughToOsEnv(t *testing.T) {
	t.Setenv("MY_VAR_TEST_ONLY", "my-value")
	s := substitutor{}
	if got := s.expand("${MY_VAR_TEST_ONLY}"); got != "my-value" {
		t.Errorf("expand fallthrough = %q, want my-value", got)
	}
}

// Token generation — uniqueness + format.

func TestGenerateSessionToken_Format(t *testing.T) {
	tok, err := generateSessionToken()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(tok) != 64 {
		t.Errorf("token len = %d, want 64 (32-byte hex)", len(tok))
	}
	for _, c := range tok {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("token contains non-hex char %q", string(c))
			break
		}
	}
}

func TestGenerateSessionToken_Unique(t *testing.T) {
	const n = 100
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		tok, err := generateSessionToken()
		if err != nil {
			t.Fatalf("[%d] err: %v", i, err)
		}
		if seen[tok] {
			t.Fatalf("[%d] duplicate token %s", i, tok)
		}
		seen[tok] = true
	}
}

// Restart backoff — pure timing math.

func TestComputeBackoff(t *testing.T) {
	p := &ExternalProcess{}
	cases := []struct {
		n           int
		wantSeconds int
	}{
		{0, 1}, {1, 2}, {2, 4}, {3, 8}, {4, 16}, {5, 32}, {6, 60}, {10, 60},
	}
	now := time.Now()
	for _, c := range cases {
		p.restartTimes = make([]time.Time, c.n)
		for i := range p.restartTimes {
			p.restartTimes[i] = now
		}
		got := p.computeBackoffLocked()
		if got != time.Duration(c.wantSeconds)*time.Second {
			t.Errorf("computeBackoff(n=%d): got %v, want %ds", c.n, got, c.wantSeconds)
		}
	}
}

func TestShouldRestart_HonorsCap(t *testing.T) {
	p := &ExternalProcess{Spec: ExternalProcessSpec{MaxRestarts: 5}}
	now := time.Now()
	for i := 0; i < 5; i++ {
		p.restartTimes = append(p.restartTimes, now)
	}
	if p.shouldRestartLocked() {
		t.Errorf("with 5 recent restarts and cap 5, shouldRestart = true; want false")
	}
	// One older entry shouldn't count.
	p.restartTimes[0] = now.Add(-10 * time.Minute)
	if !p.shouldRestartLocked() {
		t.Errorf("with 4 recent + 1 old, shouldRestart = false; want true")
	}
}

func TestShouldRestart_DefaultCap(t *testing.T) {
	p := &ExternalProcess{} // MaxRestarts=0 → default 5
	now := time.Now()
	for i := 0; i < 5; i++ {
		p.restartTimes = append(p.restartTimes, now)
	}
	if p.shouldRestartLocked() {
		t.Errorf("default cap=5 with 5 recents → shouldRestart = true; want false")
	}
}

// Integration — spawn a tiny fake process that exposes /healthz, verify
// supervisor brings it online and tears it down.

func TestExternalProcess_SpawnAndHealthz(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows uses cmd.exe / powershell for shell-script equivalents.
		// The test below relies on a small Go-built helper which works cross-platform.
		// We use the Go test binary itself in subprocess mode (TestMain trick).
	}

	// We use the test binary as the "external process" via env-flag trick.
	// When TEST_FAKE_HEALTHZ is set, the binary serves /healthz on $ALLOC_PORT.
	if os.Getenv("TEST_FAKE_HEALTHZ") == "1" {
		runFakeHealthzServer()
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}

	spec := ExternalProcessSpec{
		Command: exe,
		Args:    []string{"-test.run", "TestExternalProcess_SpawnAndHealthz"},
		Env: map[string]string{
			"TEST_FAKE_HEALTHZ": "1",
		},
		HealthzURL:  "http://127.0.0.1:${ALLOC_PORT}/healthz",
		Kind:        "fake-healthz",
		MaxRestarts: 1,
	}

	proc := NewExternalProcess(spec, t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if proc.Status() != StatusRunning {
		t.Errorf("status after Start = %q, want running", proc.Status())
	}
	if proc.Port() == 0 {
		t.Errorf("port not set after Start")
	}
	if len(proc.Token()) != 64 {
		t.Errorf("token len = %d, want 64", len(proc.Token()))
	}

	// Confirm the server is actually up.
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", proc.Port())
	resp, err := http.Get(url)
	if err != nil {
		t.Errorf("GET %s: %v", url, err)
	} else {
		_ = resp.Body.Close()
	}

	if err := proc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if proc.Status() != StatusStopped {
		t.Errorf("status after Close = %q, want stopped", proc.Status())
	}
}

// runFakeHealthzServer is invoked when the test binary is re-exec'd with
// TEST_FAKE_HEALTHZ=1. Reads ALLOC_PORT from env, serves /healthz on it,
// runs until SIGINT (delivered by Close()).
func runFakeHealthzServer() {
	port := os.Getenv("ALLOC_PORT")
	if port == "" {
		fmt.Fprintln(os.Stderr, "ALLOC_PORT unset")
		os.Exit(2)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:    "127.0.0.1:" + port,
		Handler: mux,
	}
	if err := srv.ListenAndServe(); err != nil && !strings.Contains(err.Error(), "Server closed") {
		fmt.Fprintf(os.Stderr, "ListenAndServe: %v\n", err)
		os.Exit(2)
	}
}

// closeWithin runs proc.Close and fails the test if it does not return in d.
func closeWithin(t *testing.T, proc *ExternalProcess, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		_ = proc.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("Close did not return within %s", d)
	}
}

// A process closed while Start is still probing healthz has no watchdog
// yet: Close must not wait for one, and Start must not launch one after.
// (A one-shot CLI command shuts down before the display bridge's child is
// healthy; Close then waited forever and the command never exited.)
func TestExternalProcess_CloseWhileStartingReturns(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}
	spec := ExternalProcessSpec{
		Command:    exe,
		Args:       []string{"-test.run", "^$"}, // runs no test, exits at once
		HealthzURL: "http://127.0.0.1:${ALLOC_PORT}/healthz",
		Kind:       "never-healthy",
	}
	proc := NewExternalProcess(spec, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan error, 1)
	go func() { started <- proc.Start(ctx) }()
	for proc.Status() != StatusStarting {
		time.Sleep(10 * time.Millisecond)
	}

	closeWithin(t, proc, 7*time.Second)

	cancel() // end the healthz probe now rather than at its 10 s timeout
	select {
	case err := <-started:
		if err == nil {
			t.Fatalf("Start succeeded although the process never became healthy")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Start did not return after its context was cancelled")
	}
}

// A process that could not even be spawned has no Wait goroutine and no
// watchdog: Close must still return.
func TestExternalProcess_CloseAfterFailedSpawnReturns(t *testing.T) {
	spec := ExternalProcessSpec{
		Command: "ktw-no-such-command-for-this-test",
		Kind:    "missing",
	}
	proc := NewExternalProcess(spec, t.TempDir())
	if err := proc.Start(context.Background()); err == nil {
		t.Fatalf("Start succeeded for a command that does not exist")
	}
	closeWithin(t, proc, 7*time.Second)
}
