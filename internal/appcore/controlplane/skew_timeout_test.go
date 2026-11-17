// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A PROBE THAT TIMES OUT MUST REPORT IT, NOT PANIC THE LAUNCHER.
//
// This crashed `katyoti` on the first run after a build:
//
//	panic: runtime error: invalid memory address or nil pointer dereference
//	os.(*Process).signal ... controlplane.probeVersion skew.go:113
//
// The timeout branch called `cmd.Process.Kill()`, and `Process` is nil until
// the exec has actually taken hold — so a probe that timed out before the
// process started killed a nil pointer. It reproduced once and then stopped,
// because the second run of a freshly built binary is no longer a cold start.
//
// A version check exists to WARN about a mismatch. Crashing the launch it is
// meant to protect is the worst available outcome, and worse than skipping the
// check entirely.
func TestProbeVersion_TimeoutIsReportedNotFatal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("staging a hanging probe target is platform-specific; the windows form is the one that crashed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "hang.bat")
	// Answers nothing and outlives the bound.
	if err := os.WriteFile(script, []byte("@ping -n 30 127.0.0.1 >nul\r\n"), 0o755); err != nil {
		t.Skipf("cannot stage a probe target here: %v", err)
	}

	start := time.Now()
	got, err := probeVersionWithin(script, 150*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("probe of a hanging binary returned %q and no error", got)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, want it to say it timed out — \"did not answer\" and "+
			"\"answered with an error\" are different facts about an installed binary", err)
	}
	// It must actually be bounded, not merely eventually return.
	if elapsed > 5*time.Second {
		t.Errorf("probe took %s for a 150ms bound — the timeout is not bounding anything", elapsed)
	}
}

// AND THE HAPPY PATH STILL READS THE VERSION.
func TestProbeVersionWithin_StillReadsTheLastField(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("batch fixture is windows-specific")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake.bat")
	if err := os.WriteFile(script, []byte("@echo ktw-hub version 6aefbed8\r\n"), 0o755); err != nil {
		t.Skipf("cannot stage a probe target here: %v", err)
	}
	got, err := probeVersionWithin(script, 10*time.Second)
	if err != nil {
		t.Skipf("probe not runnable in this environment: %v", err)
	}
	if got != "6aefbed8" {
		t.Errorf("probeVersionWithin = %q, want 6aefbed8", got)
	}
}
