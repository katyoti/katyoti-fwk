// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// skew.go — does the installed control plane match the binary that is about to
// use it?
//
// # The failure this exists for
//
// `katyoti` launches the control-plane set (InfraBinaries), and resolveHubBinary
// deliberately prefers the INSTALLED copy over a freshly built one. That is
// right for a shipped install and wrong to do silently: a developer running
// ./bin/katyoti from a current checkout gets whatever hub was installed days
// ago, and the two need not agree about anything.
//
// Observed 2026-09-19: an installed set from 2026-09-14 against sidecars built
// that morning. The contribution envelope had stopped carrying its own `Schema`
// three days earlier (a kind declares it now), so the old hub read a field that
// no longer travels, registered ZERO data-types, and marked every contribution
// invalid. What the user saw was "Catalog unavailable" and "no tasks
// registered" — a working system reporting nothing, with the actual cause five
// days and one process away.
//
// InfraBinaries' own doc already says the set "moves together … shipping them
// independently is a skew bug you build by hand". This checks the other axis:
// that the set matches whoever is launching it.

// Skew is one installed control-plane binary whose version differs from the
// caller's.
type Skew struct {
	Name      string // binary name, e.g. "ktw-hub"
	Path      string // where the installed copy lives
	Installed string // the version it reports
}

// versionProbeTimeout bounds one `<binary> --version`. It is a local exec that
// prints one line; a binary that cannot answer that fast is reported as
// unknown rather than allowed to delay a launch.
const versionProbeTimeout = 5 * time.Second

// waitDelayAfterTimeout bounds how long a killed probe may still hold its
// output pipe before the read is abandoned. It is short on purpose: by the
// time it applies, the probe has already failed and the only question left is
// how fast the launcher learns.
const waitDelayAfterTimeout = 500 * time.Millisecond

// VersionSkew reports which installed control-plane binaries disagree with
// `want`, the version of the binary asking.
//
// Only binaries that are actually installed are probed — a dev checkout with no
// control plane has nothing to disagree with, and reports none. An unreadable
// version is reported as skew with Installed="unknown": "I could not tell" and
// "they match" are different answers, and only one of them is safe to ignore.
//
// Never fatal by itself. The caller decides what to do, because the right
// response differs: a launcher warns, a test may refuse.
func VersionSkew(want string) []Skew {
	want = strings.TrimSpace(want)
	if want == "" || want == "dev" {
		// Nothing to compare against. A binary with no version of its own
		// cannot accuse anything else of disagreeing with it.
		return nil
	}
	var out []Skew
	for _, name := range InfraBinaries {
		if !answersVersionOnStdout(name) {
			continue
		}
		path, err := TargetPath(name)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue // not installed — nothing to skew
		}
		got, err := probeVersion(path)
		switch {
		case err != nil:
			out = append(out, Skew{Name: name, Path: path, Installed: "unknown"})
		case got != want:
			out = append(out, Skew{Name: name, Path: path, Installed: got})
		}
	}
	return out
}

// answersVersionOnStdout reports whether `<binary> --version` prints a line and
// exits, rather than doing something else.
//
// ktw-gui is a DESKTOP application: `--version` opens a window and waits, so
// probing it would hang the very launch this check is meant to protect (caught
// by it hanging a terminal for 600 s). The set is installed atomically and its
// own doc says it "moves together", so the members that do answer represent it.
func answersVersionOnStdout(name string) bool { return name != "ktw-gui" }

// probeVersion runs `<binary> --version` and returns the version token.
//
// The output is `<name> version <v>`, so the last field is the version. Parsing
// the last field rather than a fixed column keeps this working if the prefix
// ever gains a word.
func probeVersion(path string) (string, error) {
	return probeVersionWithin(path, versionProbeTimeout)
}

// probeVersionWithin is probeVersion with an explicit bound, so the timeout
// path is testable without waiting five seconds for it.
//
// IT USES CommandContext RATHER THAN A GOROUTINE AND A SELECT, and that is the
// whole point. The previous shape ran `cmd.Output()` in a goroutine and, on
// timeout, called `cmd.Process.Kill()` — but `Process` is nil until Output has
// actually started the process, so a probe that timed out BEFORE the exec took
// hold dereferenced nil and panicked the launcher:
//
//	panic: runtime error: invalid memory address or nil pointer dereference
//	os.(*Process).signal ... controlplane.probeVersion skew.go:113
//
// It reproduced on the first run after a build and not the second, which is
// the signature of a cold start: a freshly written, unsigned binary is scanned
// on first execution, and the scan can outrun this bound.
//
// The same shape also raced: after the timeout branch returned, the goroutine
// was still writing `out` and `err` that the caller had moved past.
//
// CommandContext kills the child when the context expires, so there is no
// nil to dereference and nothing still writing behind the return.
func probeVersionWithin(path string, within time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	publish.NoConsoleWindow(cmd)
	// WaitDelay BOUNDS THE RETURN, which cancelling the context alone does not.
	// Killing the child does not kill ITS children, and `Output` waits for the
	// stdout pipe to close — so a probe target that spawns anything leaves the
	// read blocked long after the deadline. Measured: a 150ms bound returned
	// after 29s without this. A launcher that hangs is a launcher that is
	// broken, just more quietly than one that panics.
	cmd.WaitDelay = waitDelayAfterTimeout
	out, err := cmd.Output()
	if ctx.Err() != nil {
		// Reported as its own failure, never folded into err: "did not answer
		// in time" and "answered with an error" are different facts about the
		// installed binary.
		return "", fmt.Errorf("controlplane: %s --version timed out after %s", path, within)
	}
	if err != nil {
		return "", err
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return "", fmt.Errorf("controlplane: %s --version printed nothing", path)
	}
	return fields[len(fields)-1], nil
}

// SkewWarning renders a human-facing warning for a non-empty skew list, or ""
// when there is none.
//
// One place, so the launcher and anything else that checks say the same thing —
// including the fix, because a warning that does not name the remedy is one
// people learn to scroll past.
func SkewWarning(want string, skews []Skew) string {
	if len(skews) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "warning: the installed control plane does not match this binary (%s).\n", want)
	for _, s := range skews {
		fmt.Fprintf(&b, "  %-12s installed=%s  (%s)\n", s.Name, s.Installed, s.Path)
	}
	b.WriteString("  The installed copy WINS over ./bin, so this is what will run.\n")
	b.WriteString("  A hub older than the contribution model it loads reports an empty\n")
	b.WriteString("  catalog rather than an error. Refresh it with:  make install\n")
	return b.String()
}
