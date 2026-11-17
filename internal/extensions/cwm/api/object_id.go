// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Object identity — one authored id, module-qualified.
//
// Decided 2026-09-14 (user): the human id wins. `compose.CMP-010`, not a uuid
// plus a separate uc_id kept in sync by hand. Identity comes from the file
// content, filenames stay readable, and diffs stay reviewable.
//
// The qualifier is not decoration: it says which module folder owns the file,
// so an id is self-locating. object_crud.go warns against DERIVING the module
// silently — hence SplitObjectID, which fails loudly on an unqualified id
// instead of guessing.

// QualifyObjectID builds a module-qualified object id. If local is already
// qualified with this module, it is returned unchanged, so the call is safe to
// apply twice.
func QualifyObjectID(module, local string) string {
	if module == "" {
		return local
	}
	// A MODULE object's id is the bare module name — modules/cwm/objects/cwm.yaml
	// — so it is never self-qualified into "cwm.cwm".
	if local == module {
		return local
	}
	if strings.HasPrefix(local, module+".") {
		return local
	}
	return module + "." + local
}

// SplitObjectID recovers the owning module and the local id.
//
// It returns an error rather than a best guess: a consumer holding an
// unqualified id does not know where the object lives, and inventing a module
// would write the file into the wrong folder — a silent, hard-to-trace fault.
func SplitObjectID(id string) (module, local string, err error) {
	i := strings.Index(id, ".")
	if id == "" || i <= 0 || i == len(id)-1 {
		return "", "", fmt.Errorf("%w: object id %q is not module-qualified (want <module>.<id>)", ErrInvalidInput, id)
	}
	return id[:i], id[i+1:], nil
}

// ModuleOfObjectID names the module folder that owns an object.
//
// A MODULE object's id is the bare module name — `cwm` lives in
// modules/cwm/objects/cwm.yaml — so an unqualified id is not an error here the
// way it is for SplitObjectID: it means "this IS a module". Everything else
// must be qualified.
func ModuleOfObjectID(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("%w: empty object id", ErrInvalidInput)
	}
	if i := strings.Index(id, "."); i > 0 {
		return id[:i], nil
	}
	if strings.ContainsAny(id, `./\`) {
		return "", fmt.Errorf("%w: malformed object id %q", ErrInvalidInput, id)
	}
	return id, nil
}

// NewContainerObject builds a tree-placed object — what NodeCreate used to
// make. parent is another object's id, or empty for a root.
func NewContainerObject(module, local, typ, parent, name, description string) Object {
	return Object{ID: QualifyObjectID(module, local), Type: typ, Parent: parent, Name: name, Description: description,
		Capabilities: map[string]map[string]any{}}
}

// NewRecordObject builds a record belonging to another object — what
// ItemCreate used to make: its name, its parent and its content (JSON: a string
// is the prose body, an object is the type's fields).
func NewRecordObject(module, local, typ, parent, title string, content json.RawMessage) Object {
	o := Object{ID: QualifyObjectID(module, local), Type: typ, Parent: parent, Name: title, Capabilities: map[string]map[string]any{}}
	var prose string
	if json.Unmarshal(content, &prose) == nil {
		o.Body = prose
	} else {
		o.SetFieldsJSON(content)
	}
	return o
}
