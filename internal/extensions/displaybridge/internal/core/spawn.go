// SPDX-License-Identifier: Apache-2.0

// Package core holds the displaybridge implementation. Hidden
// from external consumers — the public surface is in api/.
package core

import (
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
)

// BunSpec returns the ExternalProcessSpec used to lazy-spawn the ktw-display
// bun sidecar. Auto-detects a compiled binary at app/ktw-display/dist/ktw-display{,.exe}
// and falls back to `bun run app/ktw-display/src/index.ts` for dev.
//
// Substitution variables (resolved by the supervisor at spawn time):
//
//	${ALLOC_PORT}    — kernel-allocated free port
//	${SESSION_TOKEN} — crypto-random per-spawn token
//	${KTW_WORKSPACE} — workspace folder
func BunSpec() publish.ExternalProcessSpec {
	cmd, args := resolveBunCommand()
	return publish.ExternalProcessSpec{
		Command: cmd,
		Args:    args,
		Env: map[string]string{
			"KTW_DISPLAY_PORT":  "${ALLOC_PORT}",
			"KTW_DISPLAY_TOKEN": "${SESSION_TOKEN}",
			// Future: KTW_KATYOTI_KTSP_URL for bun → agents callbacks. Not used in Phase 1.
		},
		HealthzURL: "http://127.0.0.1:${ALLOC_PORT}/healthz",
		// The compiled server is a large binary: its first start on a machine
		// waits for the antivirus to scan it, often past the default 10s.
		HealthzTimeout: time.Minute,
		Kind:           "ktw-display",
		MaxRestarts:    5,
	}
}

// resolveBunCommand picks the compiled binary if it exists, otherwise falls
// back to `bun run` against the TypeScript source. This lets dev workflows
// (where the binary may not be built yet) work transparently while production
// builds use the compiled binary for fast startup.
func resolveBunCommand() (cmd string, args []string) {
	if path, ok := findCompiledBinary(); ok {
		return path, nil
	}
	// Dev fallback: invoke bun against the TS entry. Path is relative to the
	// process's working directory (typically the repo root); resolved by the
	// supervisor at spawn time.
	return "bun", []string{"run", "app/ktw-display/src/index.ts"}
}

// findCompiledBinary returns the absolute path of the ktw-display compiled
// binary if it exists (api.LocateDisplayBinary: KTW_DISPLAY_BIN, the installed
// one, beside the running binary, app/ktw-display/dist/). Returns ok=false if
// none is found; the caller falls back to `bun run` against the TS source
// (dev mode, requires bun in PATH).
func findCompiledBinary() (string, bool) {
	found := api.LocateDisplayBinary()
	return found.Path, found.Path != ""
}
