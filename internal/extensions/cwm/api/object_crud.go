// SPDX-License-Identifier: Apache-2.0

package api

// Object CRUD — the authored side of the substrate.
//
// These verbs read and write ONE object file each, under the object's module
// folder in the Project (modules/<module>/objects/), committed with the code.
// They do not touch the CWM store: that is a rebuildable index, and building it
// is P2's job. Query verbs (List / Tree / Search) are deliberately absent here —
// over raw files they would rescan per call, so they answer from the index.
//
// The module is passed EXPLICITLY rather than derived from the object id: a
// dotted-id convention would silently break the moment an id did not follow it.

// ObjectCreateInput writes a new object. It fails if the object already exists —
// overwriting is Update's job, not Create's.
type ObjectCreateInput struct {
	Module string `json:"module" jsonschema:"Module that owns the object; required"`
	Object Object `json:"object" jsonschema:"The object to write"`
}

// ObjectCreateOutput returns the stored object and the file it was written to.
type ObjectCreateOutput struct {
	Object   Object `json:"object"`
	Path     string `json:"path" jsonschema:"Repo-relative path of the object file"`
	Revision string `json:"revision,omitempty" jsonschema:"The revision written: the hash of the object file"`
}

// ObjectGetInput loads one object by module + id.
type ObjectGetInput struct {
	Module string `json:"module" jsonschema:"Module that owns the object; required"`
	ID     string `json:"id"     jsonschema:"Object id; required"`
}

// ObjectGetOutput returns the loaded object.
type ObjectGetOutput struct {
	Object Object `json:"object"`
	// Errors are the problems CWM found reading the object (object_errors.go);
	// empty when it has none.
	Errors []ObjectDiagnostic `json:"errors,omitempty" jsonschema:"The object's problems as read: invalid, a state outside its workflow, a dangling link"`
	// Revision names the stored version that was read. Handed back to
	// ObjectUpdate, it makes the write conditional: refused with ErrConflict
	// when someone wrote the object in between.
	Revision string `json:"revision,omitempty" jsonschema:"The version read; pass it to ObjectUpdate to refuse a write over a newer one"`
	// Project names, when the universe serves several projects, the project
	// holding the object - as Projects of ObjectList and
	// ObjectSearch does for each object. The store set's front names it on
	// every read, whatever store answered; a universe serving one project is
	// answered by its store alone, which leaves it empty.
	Project string `json:"project,omitempty" jsonschema:"When several projects are served: the project key of the project holding the object"`
}

// ObjectUpdateInput replaces an existing object wholesale. It fails if the object
// does not exist — creating is Create's job.
type ObjectUpdateInput struct {
	Module string `json:"module" jsonschema:"Module that owns the object; empty = the object's own"`
	Object Object `json:"object" jsonschema:"The replacement object"`
	// Revision, when set, is the version the caller read (ObjectGet): the
	// write is refused with ErrConflict if the stored object is no longer that
	// version, so a read-modify-write never erases a concurrent write. An
	// update arriving from outside the process (CLI, agent tool, KTSP, an
	// action) without one is refused unless Force (object_update_guard.go).
	Revision string `json:"revision,omitempty" jsonschema:"The version read (ObjectGet.revision); a newer stored version refuses the write with a conflict; required unless force"`
	// Force writes without a revision, over whatever is stored — an edit made
	// outside CWM since the caller read the object included.
	Force bool `json:"force,omitempty" jsonschema:"Write without a revision and overwrite whatever is stored"`

	// outside is set when the input was decoded from JSON: it reached CWM
	// through a surface (object_update_guard.go), not a Go call in process.
	outside bool
}

// ObjectUpdateOutput returns the stored object and its new version.
type ObjectUpdateOutput struct {
	Object   Object `json:"object"`
	Revision string `json:"revision,omitempty" jsonschema:"The version just written"`
}

// ObjectDeleteInput removes one object file.
type ObjectDeleteInput struct {
	Module string `json:"module" jsonschema:"Module that owns the object; required"`
	ID     string `json:"id"     jsonschema:"Object id; required"`
	Detach bool   `json:"detach,omitempty" jsonschema:"Remove the links other objects have to this one; without it a linked object is refused"`
}

// ObjectDeleteOutput reports the removed object's id and, with Detach, the
// objects whose links to it were removed.
type ObjectDeleteOutput struct {
	ID       string   `json:"id"`
	Detached []string `json:"detached,omitempty"`
}

// HistoryFolder is where an archived object's file goes, under its module's
// planning space: `<module>/.ktw/_history/<yyyy-mm>/`. The `_` keeps it out
// of every scan; the ended-id set reads it.
const HistoryFolder = PlanningFolder + "/_history"

// ObjectArchiveInput ends an object: its file leaves the index for its
// module's history folder, bytes unchanged, and every link to it stays valid
// as an ended link.
type ObjectArchiveInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
	// Into names the history folder: `.ktw/_history/<segment>`; empty = the
	// month of the archive (UTC, yyyy-mm). Any other folder is refused.
	Into string `json:"into,omitempty" jsonschema:"The history folder: .ktw/_history/<segment>; empty = .ktw/_history/<yyyy-mm> of now (UTC)"`
	// Revision, when given, refuses a newer stored object (conflict).
	Revision string `json:"revision,omitempty" jsonschema:"The version read; a newer stored one is refused (conflict). Empty = archive whatever is stored"`
}

// ObjectArchiveOutput reports where the file went. AlreadyArchived: the id
// had ended already; nothing moved and no event went out.
type ObjectArchiveOutput struct {
	ID              string `json:"id"`
	Path            string `json:"path"             jsonschema:"The archived file, project-relative"`
	AlreadyArchived bool   `json:"already_archived,omitempty" jsonschema:"The id had ended already: nothing moved"`
}

// ObjectRetypeInput changes an object's type.
type ObjectRetypeInput struct {
	ID   string `json:"id"   jsonschema:"The object; required"`
	Type string `json:"type" jsonschema:"Its new type: a declared type of the same level; required"`
}

// ObjectRetypeOutput is the object under its new type.
type ObjectRetypeOutput struct {
	Object   Object `json:"object"`
	Previous string `json:"previous" jsonschema:"The type it had"`
}
