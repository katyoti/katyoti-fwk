// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
)

func TestBunSpec_HasRequiredFields(t *testing.T) {
	spec := BunSpec()

	if spec.Command == "" {
		t.Error("Command must not be empty")
	}
	if spec.Kind != "ktw-display" {
		t.Errorf("Kind = %q, want %q", spec.Kind, "ktw-display")
	}
	if spec.HealthzURL == "" {
		t.Error("HealthzURL must not be empty")
	}
	if !strings.Contains(spec.HealthzURL, "${ALLOC_PORT}") {
		t.Errorf("HealthzURL = %q, want substitution variable ${ALLOC_PORT}", spec.HealthzURL)
	}
	if spec.MaxRestarts <= 0 {
		t.Errorf("MaxRestarts = %d, want > 0", spec.MaxRestarts)
	}
}

func TestBunSpec_EnvVars(t *testing.T) {
	spec := BunSpec()

	if got, want := spec.Env["KTW_DISPLAY_PORT"], "${ALLOC_PORT}"; got != want {
		t.Errorf("Env[KTW_DISPLAY_PORT] = %q, want %q", got, want)
	}
	if got, want := spec.Env["KTW_DISPLAY_TOKEN"], "${SESSION_TOKEN}"; got != want {
		t.Errorf("Env[KTW_DISPLAY_TOKEN] = %q, want %q", got, want)
	}
}

func TestResolveBunCommand_DevFallback(t *testing.T) {
	// In the test environment there's typically no compiled binary, so
	// we expect the dev fallback (bun run) — assuming KTW_DISPLAY_BIN env is unset.
	t.Setenv("KTW_DISPLAY_BIN", "")

	cmd, args := resolveBunCommand()
	// Either the test env has a compiled binary (cmd points at it, args empty)
	// or it falls back to bun run (cmd="bun", args=["run", ...]).
	if cmd != "bun" && cmd == "" {
		t.Error("Command must not be empty")
	}
	if cmd == "bun" && (len(args) < 2 || args[0] != "run") {
		t.Errorf("Dev fallback: args = %v, want ['run', '<entry>']", args)
	}
}

func TestResolveBunCommand_BinOverride(t *testing.T) {
	// KTW_DISPLAY_BIN points at a non-existent path; resolver should fall back
	// to dev (since stat fails on the override).
	t.Setenv("KTW_DISPLAY_BIN", "/nonexistent/ktw-display")

	cmd, args := resolveBunCommand()
	if cmd == "/nonexistent/ktw-display" {
		t.Errorf("Expected non-existent override to be ignored; got cmd=%q args=%v", cmd, args)
	}
}
