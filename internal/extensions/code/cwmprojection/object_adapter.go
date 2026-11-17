// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	stdctx "context"
	"encoding/json"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Object → projection adapter (ms-cwm-v4 P1 / p1d).
//
// The decoders below all read the same handful of fields off what used to be an
// Item or a Node. Reading the Object shape in ONE place keeps a capability
// rename to a single edit, and keeps every decoder honest about where a field
// actually lives now: the payload is the type's `fields` and the prose `body`,
// and the tree link is `parent`. An object stores no times: when one was made
// or changed is the journal's (JournalService.Times).

// objParts is the flattened view every decoder starts from.
type objParts struct {
	ID       string
	ParentID string
	Title    string
	Fields   json.RawMessage // the type's own fields
	Prose    string          // the body
}

func partsOf(o cwmapi.Object) objParts {
	return objParts{
		ID:       o.ID,
		ParentID: o.ParentID(),
		Title:    o.Name,
		Fields:   o.FieldsJSON(),
		Prose:    o.Body,
	}
}

// capGroup returns one capability's field-group — the replacement for what used
// to be node.TypeSpecific. A missing group reads as empty rather than nil-panic,
// because a module that has not been stamped yet is normal, not an error.
func capGroup(o cwmapi.Object, name string) map[string]any {
	if g, ok := o.Capability(name); ok {
		return g
	}
	return map[string]any{}
}

// moduleCapability is where a module's scalar attributes live: what used to be
// node.TypeSpecific["tier_current"] is now object.Capabilities["code"]
// ["tier_current"]: the code capability's.
const moduleCapability = "module"

// codeCapability is where a software module's code lives: its kind, its
// packages, its tiers and its facets.
const codeCapability = "code"

// IsModule reports whether the object id is a module: whether the `module`
// capability is in force on it. Every "is this a module?" in the code and
// gocode extensions asks here — a code-module carries `module` and IS one; a
// test on the type's name refused it.
func IsModule(ctx stdctx.Context, cwm cwmapi.CWMService, id string) (bool, error) {
	return cwmapi.HasCapability(ctx, cwm, id, moduleCapability)
}

// authoredLocalID recovers the human id from a module-qualified object id —
// “ -> `UC-010`.
//
// It returns EMPTY for an unqualified id rather than the id itself. That is the
// load-bearing choice: an unqualified id means the object has not been through
// the identity collapse, so the caller must fall back to the body's own id
// field. Returning the raw id here would silently make "item-uuid" the UC's
// human identifier and skip that fallback.
func authoredLocalID(id string) string {
	if _, local, err := cwmapi.SplitObjectID(id); err == nil {
		return local
	}
	return ""
}

// DecodeUseCaseObject exposes the loader's own use-case decoder.
//
// Exported so a converter can prove its objects are readable HERE rather than
// only by itself. A second decoder written for the test would have tested the
// test: the point is that THIS code path, the one the projection actually
// runs, accepts what the converter produces.
func DecodeUseCaseObject(o cwmapi.Object) (gcapi.UseCase, error) { return decodeUseCase(o) }
