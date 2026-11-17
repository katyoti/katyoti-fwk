// SPDX-License-Identifier: Apache-2.0

package api

// STRUCTURE EDITS THAT CHANGE IDENTITY, with a trail (2026-09-25).
//
// A move WITHIN a module is `ObjectUpdate` rewriting `parent`, and
// that is all it is — phase 7 settled it and no verb is needed. What needs a
// verb is the case that is NOT a move: an object id is module-qualified, so
// refiling an object into another module CHANGES ITS IDENTITY. The old id
// stops existing and a new one starts, which is a rename plus a relocation
// plus every reference to the old one going stale.
//
// THE TRAIL IS THE POINT, and the journal keeps it. Each verb publishes ONE
// event saying what happened and why — `object.<token>.moved` (from id, from
// module, reason), `object.<token>.split` (source, successors, reason) — and
// the journal records it, because the one thing a reader cannot recover
// afterwards is that these ids were the same thing. The writes underneath that
// the event already reports are marked with its cause. No `decision` object is
// written: a decision is recorded when someone decided something.

// ObjectMoveInput refiles an object, possibly into another module.
type ObjectMoveInput struct {
	ID string `json:"id" jsonschema:"The object to move; required"`
	// NewParent is the object it will hang under. Empty makes it a root of
	// its module, which is legitimate — a module object has no parent.
	NewParent string `json:"new_parent,omitempty" jsonschema:"The new parent object id; empty = a root"`
	// NewModule refiles into another module, which MINTS A NEW ID. Empty
	// keeps the object where it is and only the parent changes.
	NewModule string `json:"new_module,omitempty" jsonschema:"Target module; empty = stay in the current one. Setting it changes the object's id"`
	// NewLocalID RENAMES: the local part of the id becomes this one, in the
	// module the object ends in. A rename alone (no new module, no new parent)
	// keeps the parent; every reference to the object follows it.
	NewLocalID string `json:"new_local_id,omitempty" jsonschema:"Rename: the new local part of the id (UC-074, g-memory); a rename alone keeps the parent. Every parent link and edge naming the object is rewritten"`
	// Reason is REQUIRED. The trail exists to answer "why", and a trail
	// carrying no reason records only that someone did something.
	Reason string `json:"reason" jsonschema:"Why the move happened; required — the journal records it with the move"`
}

// ObjectMoveOutput reports the move and everything it had to rewrite.
type ObjectMoveOutput struct {
	// ID is what the object is called NOW. It differs from the input id
	// whenever the module changed.
	ID string `json:"id"`
	// PreviousID is what it was called before, empty when only the parent
	// changed. A caller holding the old id needs to be told, not to infer it.
	PreviousID string `json:"previous_id,omitempty"`
	// Rewritten lists every object whose file changed BECAUSE of the move —
	// children whose parent was renamed, and the sources of edges that pointed
	// at the old id. Reported rather than silent: a structure edit that
	// touches nine other files should say so.
	Rewritten []string `json:"rewritten,omitempty"`
	// Dangling lists references that could NOT be rewritten, if any. Empty is
	// the normal case; a non-empty list is the honest form of a partial move.
	Dangling []string `json:"dangling,omitempty"`
}

// ObjectSplitInput replaces one object with several.
type ObjectSplitInput struct {
	ID string `json:"id" jsonschema:"The object being split; required"`
	// Successors are the objects that replace it — at least two, or it is a
	// rename rather than a split.
	Successors []ObjectSplitTarget `json:"successors" jsonschema:"At least two; fewer is a rename, not a split"`
	// Reason is REQUIRED, as for a move.
	Reason string `json:"reason" jsonschema:"Why the split happened; required"`
	// DeleteSource removes the original once its children have been dealt
	// with. Default false: keeping it is the reversible choice, and a split
	// that silently deleted its source would lose whatever the source's body
	// held that no successor inherited.
	DeleteSource bool `json:"delete_source,omitempty" jsonschema:"Remove the original after the split; default false — keeping it is reversible"`
}

// ObjectSplitTarget is one successor.
type ObjectSplitTarget struct {
	// ID is optional; empty mints one from the title the way any create does.
	ID          string `json:"id,omitempty"`
	Type        string `json:"type"  jsonschema:"The successor's object type; required"`
	Title       string `json:"title" jsonschema:"Its name; required"`
	Description string `json:"description,omitempty"`
	// Children names which of the SOURCE's children this successor adopts.
	// EXPLICIT on purpose: splitting is exactly the moment someone decides
	// which part goes where, and a rule that sent them all to the first
	// successor would be a guess dressed as a default.
	Children []string `json:"children,omitempty" jsonschema:"Ids of the source's children this successor adopts"`
}

// ObjectSplitOutput reports the successors and the trail.
type ObjectSplitOutput struct {
	Successors []string `json:"successors"`
	// Rewritten lists the children that were re-parented.
	Rewritten []string `json:"rewritten,omitempty"`
	// SourceKept says whether the original still exists, so a caller does not
	// have to guess from the absence of an error.
	SourceKept bool `json:"source_kept"`
	// Orphans are the source's children that NO successor adopted. They stay
	// where they are, and are named because a split that quietly stranded
	// three children would look successful.
	Orphans []string `json:"orphans,omitempty"`
}
