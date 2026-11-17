// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A control plane that matches the caller reports no skew; one that does not
// reports it by name, with the remedy.
//
// The failure this pins: an installed set from 2026-09-14 against a launcher
// built 2026-09-19. resolveHubBinary prefers the installed copy, so the old hub
// ran — and a hub older than the contribution model it loads reports an EMPTY
// CATALOG rather than an error. Nothing said the versions differed.
func TestSkewWarning_NamesTheBinaryAndTheFix(t *testing.T) {
	skews := []Skew{{Name: "ktw-hub", Path: `C:\cp\ktw-hub.exe`, Installed: "abc1234"}}
	w := SkewWarning("def5678", skews)
	for _, want := range []string{"ktw-hub", "abc1234", "def5678", "make install"} {
		if !strings.Contains(w, want) {
			t.Errorf("warning must name %q; got:\n%s", want, w)
		}
	}
}

func TestSkewWarning_SilentWhenAligned(t *testing.T) {
	if w := SkewWarning("v1", nil); w != "" {
		t.Errorf("no skew must print nothing, got %q", w)
	}
}

// A binary with no version of its own cannot accuse anything of disagreeing.
func TestVersionSkew_UnversionedCallerAccusesNothing(t *testing.T) {
	if got := VersionSkew("dev"); got != nil {
		t.Errorf(`want nil for an unversioned caller, got %v`, got)
	}
	if got := VersionSkew(""); got != nil {
		t.Errorf(`want nil for an empty version, got %v`, got)
	}
}

// probeVersion reads the LAST field, so the prefix may gain words.
func TestProbeVersion_ReadsTheLastField(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake.bat")
	if err := os.WriteFile(script, []byte("@echo ktw-hub version 6aefbed8\r\n"), 0o755); err != nil {
		t.Skipf("cannot stage a probe target here: %v", err)
	}
	got, err := probeVersion(script)
	if err != nil {
		t.Skipf("probe not runnable in this environment: %v", err)
	}
	if got != "6aefbed8" {
		t.Errorf("probeVersion = %q, want 6aefbed8", got)
	}
}

// ktw-gui must never be probed: `--version` opens a WINDOW and waits, so a
// probe hangs the launch this check protects. Found the hard way — it held a
// terminal for 600 s.
func TestVersionSkew_NeverProbesTheDesktopBinary(t *testing.T) {
	if answersVersionOnStdout("ktw-gui") {
		t.Error("ktw-gui answers --version with a window, not a line; it must not be probed")
	}
	probed := 0
	for _, name := range InfraBinaries {
		if answersVersionOnStdout(name) {
			probed++
		}
	}
	if probed == 0 {
		t.Error("every infra binary was excluded — the check would never detect skew")
	}
}
