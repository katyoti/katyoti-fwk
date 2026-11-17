// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// An error that crossed a process boundary as text gets its sentinel back;
// one that already is a sentinel, or carries none, is left as it is.
func TestRestoreGivesBackTheSentinel(t *testing.T) {
	wire := errors.New(`Internal error: cwm: not found: object "todo.verdict-AC-001"`)
	if errors.Is(wire, ErrNotFound) {
		t.Fatal("the wire error already is ErrNotFound: the test proves nothing")
	}
	if !errors.Is(Restore(wire), ErrNotFound) {
		t.Errorf("Restore(%q) is not ErrNotFound", wire)
	}
	if !errors.Is(Restore(errors.New("Internal error: cwm: already exists: x")), ErrAlreadyExists) {
		t.Error("already exists not restored")
	}
	plain := errors.New("connection refused")
	if Restore(plain) != plain || Restore(nil) != nil {
		t.Error("an error carrying no sentinel changed")
	}
	if Restore(ErrConflict) != ErrConflict {
		t.Error("a sentinel was rewrapped")
	}
}

// An invalid input refuses the caller's input: over KTSP it answers invalid
// params, not an internal error, and the caller's Restore still
// finds ErrInvalidInput in the wire error's text.
func TestAnInvalidInputCrossesKTSPAsInvalidParams(t *testing.T) {
	refusal := fmt.Errorf("ObjectGet: %w: object %q is in several projects (walk-a, walk-b)", ErrInvalidInput, "sales.N-1")
	if !ktsp.IsInputRefusal(refusal) {
		t.Fatal("ErrInvalidInput is not a ktsp.InputRefusal")
	}
	wire := ktsp.FromError(refusal)
	if wire.Code != ktsp.CodeInvalidParams {
		t.Errorf("Code = %d (%q), want invalid params", wire.Code, wire.Message)
	}
	// Across the wire only the code and the text remain.
	crossed := &ktsp.RPCError{Code: wire.Code, Message: wire.Message}
	if !errors.Is(Restore(crossed), ErrInvalidInput) {
		t.Errorf("Restore(%q) is not ErrInvalidInput", crossed.Message)
	}
	if ktsp.IsInputRefusal(fmt.Errorf("x: %w", ErrNotFound)) {
		t.Error("a not-found reads as an input refusal")
	}
}
