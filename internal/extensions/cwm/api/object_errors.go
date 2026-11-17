// SPDX-License-Identifier: Apache-2.0

package api

// ERRORS ARE OBJECT DATA. CWM refuses a write that would ADD a problem, and
// REPORTS what it reads: an object stored with a problem — an undeclared type,
// a field group against its schema, a state outside its workflow, a link to an
// object that does not exist — is loaded anyway and carries its errors, so a
// person sees it (red in the tree, a panel in Information, the global Errors
// list) and can fix it through the API. A file that cannot be read at all is
// reported by path.

// Diagnostic codes an object's errors carry.
const (
	DiagnosticInvalid        = "invalid"         // fails validation as a write would (type, capabilities, schemas, edges, containment)
	DiagnosticLifecycle      = "lifecycle"       // a state outside its workflow
	DiagnosticDanglingParent = "dangling-parent" // the parent is not an object of the project
	DiagnosticDanglingLink   = "dangling-link"   // an edge targets an object that does not exist
	DiagnosticUnreadable     = "unreadable"      // the file does not parse
	DiagnosticDuplicateID    = "duplicate-id"    // two files carry one id
	DiagnosticHookFailed     = "hook-failed"     // an on_* hook's action failed after its retries (the call stays in the hook journal)
)

// ObjectErrorListInput selects where to list the objects in error. Both empty
// = the whole project.
type ObjectErrorListInput struct {
	Module string `json:"module,omitempty" jsonschema:"Only the objects of this module"`
	Within string `json:"within,omitempty" jsonschema:"Only this object and everything under it"`
}

// ObjectErrors is one object in error.
type ObjectErrors struct {
	ID     string             `json:"id"`
	Module string             `json:"module,omitempty"`
	Type   string             `json:"type,omitempty"`
	Name   string             `json:"name,omitempty"`
	Path   string             `json:"path"`
	Errors []ObjectDiagnostic `json:"errors"`
}

// ObjectErrorListOutput lists every object in error, by id, and the files that
// could not be read at all (no id to file them under).
type ObjectErrorListOutput struct {
	Objects    []ObjectErrors  `json:"objects"`
	Unreadable []InvalidObject `json:"unreadable,omitempty" jsonschema:"Files that are not objects CWM can read: unparseable, or a second file for one id"`
	Total      int             `json:"total" jsonschema:"Objects in error plus unreadable files"`
}
