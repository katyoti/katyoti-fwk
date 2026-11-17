// SPDX-License-Identifier: Apache-2.0

package api

import "errors"

// Sentinel errors returned by ktw-gocode services. Callers wrap
// these via errors.Is for branching; KTSP / agent-tool surfaces
// translate them into typed error responses.
var (
	// ErrInvalidInput — request payload failed validation.
	ErrInvalidInput = errors.New("gocode: invalid input")

	// ErrNotFound — the addressed entity (module / UC / OD / …)
	// does not exist.
	ErrNotFound = errors.New("gocode: not found")

	// ErrImmutable — attempt to mutate an immutable entity
	// (audit review node, test-run item).
	ErrImmutable = errors.New("gocode: entity is immutable")

	// ErrConflict — write rejected because of a concurrent change
	// or a uniqueness violation (e.g. UC-id collision).
	ErrConflict = errors.New("gocode: conflict")

	// ErrPromoteGate — `module promote` refused because one or
	// more tier-promotion gate conditions failed. The output's
	// Blockers field lists the unmet gates.
	ErrPromoteGate = errors.New("gocode: promote gate refused")

	// ErrModuleKindMismatch — operation requires a specific
	// module.kind (e.g. sidecar-only config) but the target
	// module is a different kind.
	ErrModuleKindMismatch = errors.New("gocode: module kind mismatch")

	// ErrToolExec — underlying Go toolchain command failed.
	// Output's Stderr field carries the message.
	ErrToolExec = errors.New("gocode: tool exec failed")
)
