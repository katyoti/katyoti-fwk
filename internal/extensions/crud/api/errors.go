// SPDX-License-Identifier: Apache-2.0

package api

import "errors"

// Sentinel errors. Backends MUST wrap these (via fmt.Errorf("...: %w",
// ErrNotFound)) when reporting the corresponding failure modes so
// generic callers (record_load / record_save tools, GUI Collection
// Browser, federation overlays) can branch on errors.Is without
// depending on backend-specific error types.
//
// Anything else (a network failure, a parse error, a programmer
// mistake) flows through as a plain error and surfaces as
// CodeInternalError on the KTSP boundary — backends do not need to
// invent new sentinels for transient infrastructure faults.
var (
	// ErrNotFound — Load / Delete called with a (type, id) that
	// doesn't exist. The framework returns this for the agent /
	// GUI to render a clean "not found" rather than a generic
	// failure.
	ErrNotFound = errors.New("crud: record not found")

	// ErrUnsupported — operation not supported by this connection
	// (e.g. Watch on a backend with WatchCap.Supported = false, a
	// filter op outside FiltersCap.Ops). Callers should consult
	// Capabilities before retrying with a different shape.
	ErrUnsupported = errors.New("crud: operation not supported by this connection")

	// ErrConflict — Save violated a precondition (duplicate key,
	// optimistic-concurrency version mismatch, schema validation).
	// The error chain SHOULD carry a human-readable message
	// describing which precondition failed.
	ErrConflict = errors.New("crud: conflict / precondition failed")

	// ErrReadOnly — connection rejects writes
	// (CapabilitiesOutput.Writable = false). Save / Delete wrap
	// this so the GUI can disable mutating controls preemptively.
	ErrReadOnly = errors.New("crud: connection is read-only")

	// ErrInvalidInput — the request shape is wrong (missing
	// required field, unknown type, filter op not in
	// FiltersCap.Ops). Distinct from ErrUnsupported: this is
	// "the caller asked something nonsensical", not "the backend
	// can't honor an otherwise valid request".
	ErrInvalidInput = errors.New("crud: invalid input")
)
