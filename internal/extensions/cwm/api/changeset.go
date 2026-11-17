// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"time"
)

// A REACTION'S CHANGE SET. A subscription that declares
// `atomic: true` has its reaction's object writes STAGED instead of written:
// its runner opens a change set keyed by the reaction's id (ChangeSetBegin)
// before the action, and commits it (ChangeSetCommit) after the action and
// before it reports success. While the set is open, a write whose cause is
// that reaction — create, update (transitions, capabilities, edges included),
// delete without detach — is checked (before_* hooks included) and staged,
// keyed by the object's id; the reaction reads its own staged writes back
// (ObjectGet, the revision checks, the existence checks), nobody else sees
// them. The commit checks them again, verifies each object's revision at its
// first touch against the current file, and writes them all or none: a
// revision that moved is ErrConflict, the set is discarded and the reaction
// fails its attempt — a rerun stages afresh on the current state. A failed
// attempt discards its set (ChangeSetDiscard, or the next ChangeSetBegin).
// Moves, splits, renames, named and attached files and a delete with detach
// are refused inside a set. A write with no cause, or one of a reaction with
// no open set, is written as it always was.
//
// A CALLER'S CHANGE SET. Any in-process caller opens a set under
// an id of its choosing (Set in place of Reaction: exactly one is given) and
// marks the writes that join it with WithChangeSet(ctx, id). They stage and
// commit as a reaction's do; the commit runs under the cause of the set's
// first write, so a person's or an agent's on_* hooks and async matches go
// out once at the commit, never held in an outbox. The marker is a context
// value: it does not cross the bus.

type changeSetKey struct{}

// WithChangeSet routes the object writes made under ctx into the caller's
// change set id (opened by ChangeSetBegin{Set: id}); "" leaves any set.
func WithChangeSet(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, changeSetKey{}, id)
}

// ChangeSetFrom is the caller's change set WithChangeSet set, or "".
func ChangeSetFrom(ctx context.Context) string {
	id, _ := ctx.Value(changeSetKey{}).(string)
	return id
}

// Change-set write operations.
const (
	ChangeSetCreate = "create"
	ChangeSetUpdate = "update"
	ChangeSetDelete = "delete"
)

// ChangeSetWrite is one object a change set holds: what will be (or was)
// done to it.
type ChangeSetWrite struct {
	ID       string `json:"id"`
	Module   string `json:"module"`
	Op       string `json:"op"                 jsonschema:"create | update | delete"`
	Revision string `json:"revision,omitempty" jsonschema:"The revision of the bytes staged (or written, after a commit); empty for a delete"`
}

// ChangeSetBeginInput opens a reaction's or a caller's change set.
type ChangeSetBeginInput struct {
	Reaction string `json:"reaction,omitempty" jsonschema:"The reaction's id (<subscription>@<event-id>); this or set"`
	Set      string `json:"set,omitempty"      jsonschema:"A caller's change set id; this or reaction"`
}

// ChangeSetBeginOutput lists what a leftover set of the reaction held —
// discarded, never written.
type ChangeSetBeginOutput struct {
	Discarded []ChangeSetWrite `json:"discarded"`
}

// ChangeSetCommitInput commits a reaction's or a caller's change set.
type ChangeSetCommitInput struct {
	Reaction string `json:"reaction,omitempty" jsonschema:"The reaction's id (<subscription>@<event-id>); this or set"`
	Set      string `json:"set,omitempty"      jsonschema:"A caller's change set id; this or reaction"`
}

// ChangeSetCommitOutput lists the writes applied; none when the reaction has
// no open set.
type ChangeSetCommitOutput struct {
	Writes []ChangeSetWrite `json:"writes"`
}

// ChangeSetDiscardInput drops a reaction's or a caller's change set: nothing is written.
type ChangeSetDiscardInput struct {
	Reaction string `json:"reaction,omitempty" jsonschema:"The reaction's id (<subscription>@<event-id>); this or set"`
	Set      string `json:"set,omitempty"      jsonschema:"A caller's change set id; this or reaction"`
}

// ChangeSetDiscardOutput lists the writes dropped.
type ChangeSetDiscardOutput struct {
	Discarded []ChangeSetWrite `json:"discarded"`
}

// ChangeSetListInput lists the open change sets.
type ChangeSetListInput struct{}

// ChangeSetSummary is one open change set.
type ChangeSetSummary struct {
	Reaction string           `json:"reaction,omitempty"`
	Set      string           `json:"set,omitempty"`
	At       time.Time        `json:"at"     jsonschema:"When it was opened"`
	Writes   []ChangeSetWrite `json:"writes"`
}

// ChangeSetListOutput is every open change set, oldest first.
type ChangeSetListOutput struct {
	Sets []ChangeSetSummary `json:"sets"`
}
