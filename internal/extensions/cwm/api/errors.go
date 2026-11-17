// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors returned by CWMService methods.
// Consumers can use errors.Is / errors.As for typed dispatch.

var (
	// ErrNotFound is returned when a node, item, collection, file, or
	// other entity does not exist.
	ErrNotFound = errors.New("cwm: not found")

	// ErrAlreadyExists is the symmetric counterpart to ErrNotFound: the caller asked
	// to create something that is already there. Distinct from ErrInvalidInput —
	// the input was fine, the world already contains it. Objects carry AUTHORED ids
	// (unlike uuid-keyed nodes), so collisions are a real, handleable condition.
	ErrAlreadyExists = errors.New("cwm: already exists")

	// ErrConflict is returned when a conditional write (ObjectUpdate with a
	// revision) meets a newer stored version: someone wrote the object since it
	// was read. The caller reads again and redoes its change.
	ErrConflict = errors.New("cwm: the object changed since it was read")

	// ErrRefusedByHook is returned when a `before_create` / `before_update`
	// hook refused the write (or could not be asked, which refuses too). The
	// message carries the hook's uri and its reason.
	ErrRefusedByHook = errors.New("cwm: refused by hook")

	// ErrTransitionRefused is returned when ObjectTransition cannot move an
	// object: no such transition from its state, a terminal state, or a gate
	// that does not hold (and no allowed force). The message says which.
	ErrTransitionRefused = errors.New("cwm: transition refused")

	// ErrBudgetExceeded is returned when a write would exceed the
	// kind's max_size_bytes (or the per-class default).
	ErrBudgetExceeded = errors.New("cwm: budget exceeded")

	// ErrLockHeldByOtherProcess is returned when the workspace's
	// .lock file is held by another live CWM process.
	ErrLockHeldByOtherProcess = errors.New("cwm: workspace lock held by another process")

	// ErrCircularReference is returned when a write would create a
	// cycle in parent_id, dependencies, or depends_on.
	ErrCircularReference = errors.New("cwm: circular reference")

	// ErrTypeRuleViolation is returned when a write violates a node
	// type's allowed_children or allowed_item_kinds rule.
	ErrTypeRuleViolation = errors.New("cwm: type rule violation")

	// ErrReferenced is returned when a delete would leave a link dangling:
	// the object has children, or other objects link to it.
	ErrReferenced = errors.New("cwm: still referenced")

	// ErrInvalidInput is returned for malformed input that can't be
	// expressed via JSON Schema validation alone (e.g. mutually
	// exclusive fields both set, a bare id held by several projects).
	// It refuses the caller's input (ktsp.InputRefusal): over KTSP it
	// answers invalid params, never an internal error.
	ErrInvalidInput error = inputRefusal("cwm: invalid input")

	// ErrNotImplemented is returned by published methods whose
	// backing implementation hasn't shipped yet — used during
	// incremental rollout of the CWMService surface so the KTSP
	// container can dispatch every declared method even when only
	// some are wired end-to-end.
	ErrNotImplemented = errors.New("cwm: not implemented")

	// ErrProjectRefused is what every read and write of a project the
	// universe refuses answers: its data is newer than the
	// binary knows, or its type needs a solution the universe
	// lacks. The text reads `project refused: <reason>` - the GUI
	// reads that lead (projectRefusal.ts); the service itself starts.
	ErrProjectRefused = errors.New("project refused")
)

// inputRefusal is a sentinel that refuses the caller's input: it satisfies
// ktsp.InputRefusal (structurally - api imports no protocol), so a KTSP
// handler answers it as invalid params.
type inputRefusal string

func (e inputRefusal) Error() string { return string(e) }

// InputRefused marks the error as a refusal of the caller's input.
func (inputRefusal) InputRefused() bool { return true }

// sentinels are the errors Restore recognises.
var sentinels = []error{ErrNotFound, ErrAlreadyExists, ErrConflict, ErrRefusedByHook, ErrTransitionRefused,
	ErrBudgetExceeded, ErrLockHeldByOtherProcess, ErrCircularReference, ErrTypeRuleViolation, ErrReferenced,
	ErrInvalidInput, ErrNotImplemented, ErrProjectRefused}

// Restore gives back its sentinel to an error that crossed a process
// boundary: a CWMService reached over KTSP returns the error as text, so
// errors.Is(err, ErrNotFound) no longer holds. The sentinel whose text the
// error carries is wrapped back around it; an error that already is one, or
// carries none, is returned as it is.
func Restore(err error) error {
	if err == nil {
		return nil
	}
	for _, s := range sentinels {
		if errors.Is(err, s) {
			return err
		}
	}
	msg := err.Error()
	for _, s := range sentinels {
		if strings.Contains(msg, s.Error()) {
			// Both wrapped: the sentinel for errors.Is, the wire error for
			// what it carries (its message key, usermsg.Of).
			return fmt.Errorf("%w: %w", s, err)
		}
	}
	return err
}
