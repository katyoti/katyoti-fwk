// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"os"
	"sync"
)

// EndpointProvider supplies the bun sidecar's base URL and per-spawn auth
// token. It is consulted on every HTTP call so token rotation across bun
// restarts is picked up without the client holding stale state.
//
// Production wiring (Subtask 3 of go-bridge-for-display-server): a
// supervisor-backed provider reads Port() / Token() from the live
// *publish.ExternalProcess, lazy-spawning bun on first call.
//
// Test/manual wiring: StaticEndpoint pins fixed values; EnvEndpoint reads
// from KTW_DISPLAY_PORT / KTW_DISPLAY_TOKEN environment variables.
type EndpointProvider interface {
	// BaseURL returns the bun base URL without trailing slash, e.g.
	// "http://127.0.0.1:51817". Empty string signals bun is not reachable
	// — the client surfaces this as ErrEndpointUnavailable.
	BaseURL() string

	// Token returns the per-spawn session token used for the bun
	// `Authorization: Bearer <token>` header on /api/* routes. Empty
	// means dev/loopback mode (bun accepts unauth-ed requests).
	Token() string
}

// StaticEndpoint is an EndpointProvider with fixed values. Used in tests
// (point at a httptest.Server) and in manual setups where bun is started
// out-of-band.
type StaticEndpoint struct {
	URL string
	Tok string
	mu  sync.RWMutex
}

// NewStaticEndpoint constructs a StaticEndpoint.
func NewStaticEndpoint(baseURL, token string) *StaticEndpoint {
	return &StaticEndpoint{URL: baseURL, Tok: token}
}

// BaseURL returns the configured URL.
func (s *StaticEndpoint) BaseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.URL
}

// Token returns the configured token.
func (s *StaticEndpoint) Token() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Tok
}

// Set updates URL and token atomically. Useful when the consumer learns the
// bun port after spawn (e.g. supervisor-backed wiring in Subtask 3).
func (s *StaticEndpoint) Set(url, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.URL = url
	s.Tok = token
}

// EnvEndpoint reads KTW_DISPLAY_PORT and KTW_DISPLAY_TOKEN from the process
// environment on every call.
//
// Note: the supervisor (publish.ExternalProcess) sets these vars on the
// CHILD process (bun), not the parent. EnvEndpoint is therefore primarily
// useful for tests (which use t.Setenv) and for manual setups where the
// operator runs `bun run ...` separately and exports the vars in the
// ktw-agents shell.
//
// For production lazy-spawn behavior, Subtask 3 introduces a
// supervisor-backed provider that reads from the live *ExternalProcess.
type EnvEndpoint struct{}

// BaseURL builds "http://127.0.0.1:<port>" from KTW_DISPLAY_PORT.
// Returns "" if the env var is unset (callers map this to
// ErrEndpointUnavailable).
func (EnvEndpoint) BaseURL() string {
	port := os.Getenv("KTW_DISPLAY_PORT")
	if port == "" {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%s", port)
}

// Token returns KTW_DISPLAY_TOKEN.
func (EnvEndpoint) Token() string {
	return os.Getenv("KTW_DISPLAY_TOKEN")
}
