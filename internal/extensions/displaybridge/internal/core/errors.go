// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
)

// ErrEndpointUnavailable is returned when the bridge has no resolved bun
// base URL — typically because the supervisor has not spawned bun yet, or
// the EnvEndpoint provider has no KTW_DISPLAY_PORT in scope.
var ErrEndpointUnavailable = errors.New("displaybridge: bun endpoint unavailable (not spawned?)")

// BridgeError wraps a non-2xx HTTP response from bun. It preserves the wire
// shape of the bun-side error JSON (`{error, detail, diagnostics}`) so
// consumers can branch on Diagnostic.Code without re-parsing.
//
// Status carries the HTTP status code; useful for mapping 401/404/501 to
// caller-specific behavior.
//
// Per-status semantics (from app/ktw-display/src/index.ts):
//   - 401: missing/wrong Bearer token
//   - 404: unknown route (bridge bug or version skew with bun)
//   - 501: route declared but handler not yet implemented (Phase 1 normal state
//     — surfaces the slot's bun-side todo as a structured error)
//   - 5xx: transient bun errors; retried by the client for idempotent calls
type BridgeError struct {
	Status      int              `json:"-"`
	Message     string           `json:"error"`
	Detail      string           `json:"detail,omitempty"`
	Diagnostics []api.Diagnostic `json:"diagnostics,omitempty"`
}

// Error implements the error interface.
func (e *BridgeError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("displaybridge: bun returned %d: %s: %s", e.Status, e.Message, e.Detail)
	}
	return fmt.Sprintf("displaybridge: bun returned %d: %s", e.Status, e.Message)
}

// IsNotImplemented reports whether the bun side returned 501 — meaning the
// route exists but the handler isn't built yet (Phase 1 stub state).
// Consumers can use this to distinguish "not yet ready" from real failures.
func (e *BridgeError) IsNotImplemented() bool {
	return e != nil && e.Status == 501
}

// IsUnauthorized reports whether the bun side returned 401.
func (e *BridgeError) IsUnauthorized() bool {
	return e != nil && e.Status == 401
}

// IsServerError reports whether the status is in the 5xx range.
func (e *BridgeError) IsServerError() bool {
	return e != nil && e.Status >= 500 && e.Status < 600
}

// IsTransient reports whether the status is a transient 5xx — safe to
// retry. Excludes 501 (Not Implemented), which is a permanent state during
// Phase 1 of the bridge (bun stubs return 501 until their handler todo
// lands; retrying just adds latency).
func (e *BridgeError) IsTransient() bool {
	return e.IsServerError() && e.Status != 501
}

// mapErrorResponse builds a BridgeError from a non-2xx HTTP response body.
// If the body is valid JSON in bun's error shape, fields are preserved;
// otherwise the body is captured verbatim in Detail.
func mapErrorResponse(status int, body []byte) *BridgeError {
	be := &BridgeError{Status: status}
	if len(body) == 0 {
		be.Message = fmt.Sprintf("HTTP %d", status)
		return be
	}
	// Try the bun JSON shape first.
	if err := json.Unmarshal(body, be); err == nil && be.Message != "" {
		return be
	}
	// Fallback: opaque body becomes Detail; Message gets a generic label.
	be.Message = fmt.Sprintf("HTTP %d", status)
	be.Detail = string(body)
	return be
}
