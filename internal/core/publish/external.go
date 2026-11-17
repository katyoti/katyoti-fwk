// SPDX-License-Identifier: Apache-2.0

package publish

import "time"

// ExternalProcessSpec describes a non-Go sidecar that runs as an external
// process (e.g., a bun TypeScript server, a Python subprocess) supervised by
// the ktw-agents core.
//
// Used with WithExternalProcess as a PublishOption. The core's external-process
// supervisor (internal/core/publish/supervisor.go) lazy-spawns the process on
// first call, healthz-probes for readiness, and restarts on crash with
// exponential backoff.
//
// Substitution variables available in Args and Env values:
//
//	${KTW_WORKSPACE} — workspace folder, set by core at spawn
//	${ALLOC_PORT}    — free TCP port allocated by core just before spawn
//	${SESSION_TOKEN} — crypto/rand 32-byte hex generated per spawn
//	${VAR}           — any process env var (process env already has the .env chain applied)
//
// See:
//   - (modules/sidecars/decisions/OD-002-how-does-a-go-binary-run-a-sidecar-that-is-not-a.cwm.md)
type ExternalProcessSpec struct {
	// Command is the executable to run (e.g., "bun", "python3"). Resolved via
	// PATH; absolute paths also accepted.
	Command string

	// Args are the CLI args passed to Command. Each value supports ${VAR}
	// substitution.
	Args []string

	// Env is the env-var map injected at spawn (in addition to the process
	// env). Each value supports ${VAR} substitution. Always set:
	//   KTW_WORKSPACE  — workspace folder
	//   ALLOC_PORT     — allocated port (string form)
	//   SESSION_TOKEN  — generated token
	Env map[string]string

	// HealthzURL is the URL polled to confirm readiness after spawn. Supports
	// ${ALLOC_PORT} substitution. Polled every 100ms; succeeds on first 2xx
	// response. Empty means no healthz probe (caller assumes the process is
	// ready immediately after Start).
	HealthzURL string

	// HealthzTimeout bounds the wait for the first 2xx. 0 = the default (10s).
	// A large binary started for the first time on a machine can take much
	// longer: the antivirus scans it before it runs.
	HealthzTimeout time.Duration

	// Kind is a short identifier used as the stderr-log prefix
	// (e.g., "ktw-display"). Empty falls back to Command.
	Kind string

	// MaxRestarts caps the number of unexpected-exit restarts within the
	// restart-window (5 minutes). After exhaustion, the supervisor marks the
	// process permanently failed and stops trying. 0 = use default (5).
	MaxRestarts int
}

// ExternalProcessCap is the Capability emitted by WithExternalProcess.
//
// AutoKTSPContainer / AutoHTTPServer / similar protocol adapters may inspect
// this capability to decide how to wire the sidecar into the runtime.
type ExternalProcessCap struct {
	Spec ExternalProcessSpec
}

// CapabilityName implements Capability.
func (c ExternalProcessCap) CapabilityName() string { return "external-process" }

// WithExternalProcess declares that this PublishedService is backed by an
// external process. The core's external-process supervisor lazy-spawns the
// process on first call to any of the service's slots (or on explicit
// activation, depending on the routing layer wired in by the binary).
//
// Mutually exclusive with WithKTSPContainer for stdio-KTSP (Go sidecars):
// stdio sidecars use the existing pattern; external-process sidecars use this.
//
// Pairs naturally with WithKTSPSlotTags / WithServiceDef so the service still
// participates in catalog discovery.
func WithExternalProcess(spec ExternalProcessSpec) PublishOption {
	return func(ps *PublishedService) {
		ps.Capabilities = append(ps.Capabilities, ExternalProcessCap{Spec: spec})
	}
}
