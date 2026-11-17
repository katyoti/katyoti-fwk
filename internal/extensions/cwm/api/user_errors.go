// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
)

// Person-facing refusals. Each is the error CWM always returned — same
// sentinel (errors.Is), same English text for the CLI, the logs and the
// agents — given a message key and its params (usermsg), so the GUI says it in
// the person's language: `errors:<key>` in its locales. A key is
// `cwm.<name>`; its params are the values the sentence names.

// ObjectNotFound is the refusal of a verb on an object the project lacks.
func ObjectNotFound(id string) error {
	return usermsg.Wrap(fmt.Errorf("%w: object %q", ErrNotFound, id),
		"cwm.objectNotFound", map[string]any{"id": id})
}

// ObjectArchived answers a verb on an object that has ended: its file was
// archived (ObjectArchive) at path. It is a not-found — the object left the
// project — that says where it went.
func ObjectArchived(id, path string) error {
	return usermsg.Wrap(fmt.Errorf("%w: object %q has ended: archived at %s", ErrNotFound, id, path),
		"cwm.objectArchived", map[string]any{"id": id, "path": path})
}

// ObjectAlreadyExists refuses a create whose id is taken; where says where
// the existing one lives ("" when it is not known, or "being created").
func ObjectAlreadyExists(id, where string) error {
	err := fmt.Errorf("%w: object %q already exists", ErrAlreadyExists, id)
	if where == "being created" {
		err = fmt.Errorf("%w: object %q is being created", ErrAlreadyExists, id)
	} else if where != "" {
		err = fmt.Errorf("%w: object %q already exists (%s)", ErrAlreadyExists, id, where)
	}
	return usermsg.Wrap(err, "cwm.objectAlreadyExists", map[string]any{"id": id})
}

// ObjectConflict refuses a conditional write the object outran: someone wrote
// it since revision was read.
func ObjectConflict(id, revision string) error {
	return usermsg.Wrap(fmt.Errorf("%w: %q was written since revision %.12s was read", ErrConflict, id, revision),
		"cwm.objectConflict", map[string]any{"id": id})
}

// ObjectReferenced refuses a delete that would leave links dangling: what
// names the children or the objects that link to it.
func ObjectReferenced(err error, id, what string) error {
	return usermsg.Wrap(err, "cwm.objectReferenced", map[string]any{"id": id, "by": what})
}

// RefusedByHook is a write a hook refused, naming its refuser — the binary
// that contributed the entry, else where it was declared (h.Source) — and the
// hook's reason.
func RefusedByHook(point string, h ResolvedHook, reason string) error {
	by := h.Provider
	if by == "" {
		by = h.Source
	}
	return usermsg.Wrap(fmt.Errorf("%w: %s %s (by %s, %s): %s", ErrRefusedByHook, point, h.URI, by, h.Source, reason),
		"cwm.refusedByHook", map[string]any{"point": point, "hook": h.URI, "by": by, "source": h.Source, "reason": reason})
}

// Invalid is a validation refusal given its key: err keeps its text,
// key is `cwm.<name>`.
func Invalid(err error, key string, params map[string]any) error {
	return usermsg.Wrap(err, key, params)
}

// UserMsg makes a size refusal say itself in the person's language.
func (e *SizeBudgetError) UserMsg() (usermsg.Msg, bool) {
	return usermsg.Msg{Key: "cwm.sizeBudget", Params: map[string]any{
		"id": e.ID, "type": e.Type, "bytes": e.Bytes, "max": e.Max,
	}}, true
}

// UserMsg makes a guard's refusal say itself in the person's language. The
// reason is the guard's own (an expression and what its functions answered).
func (e *TransitionRefusedError) UserMsg() (usermsg.Msg, bool) {
	return usermsg.Msg{Key: "cwm.transitionRefused", Params: map[string]any{
		"id": e.Object, "transition": e.Transition, "from": e.From, "to": e.To, "reason": e.Reason,
	}}, true
}
