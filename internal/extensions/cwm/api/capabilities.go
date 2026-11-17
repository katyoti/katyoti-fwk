// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
)

// Typed views over the capability field-groups.
//
// A capability is stored as map[string]any — opaque to CWM by design, because
// meaning belongs to the pack that contributes the type (§4). But a CONSUMER
// that knows the capability should not hand-cast its way through it: the 18
// files migrating off Node*/Item* read these fields 152 times, and 152 untyped
// casts is 152 chances to get a key or a type wrong silently.
//
// So the map stays the storage shape, and these are the typed lens over it.

// No object stores when it was made or changed: those times are derived by
// the journal (JournalService.Times), from its records of the object, else
// git's log of its file. What an object was generated from is its
// derived-from edge (EdgeTypeDerivedFrom).

// Capability ids. Consumers should use these rather than spelling the string,
// so a rename is one edit and a typo is a compile error.
const (
	CapWant      = "want"
	CapFeature   = "feature"
	CapCheckable = "checkable"
)

// DecodeCapability is the honest primitive: it reports WHY a capability could
// not be read, distinguishing "absent" from "present but malformed".
//
// The convenience accessors below collapse that to a bool because callers
// overwhelmingly want "is it usable". Anything that must not silently skip a
// malformed capability uses this.
func DecodeCapability[T any](o Object, id string) (T, bool, error) {
	var out T
	group, ok := o.Capability(id)
	if !ok {
		return out, false, nil
	}
	raw, err := json.Marshal(group)
	if err != nil {
		return out, true, fmt.Errorf("%w: capability %q of object %q: %v", ErrInvalidInput, id, o.ID, err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, true, fmt.Errorf("%w: capability %q of object %q: %v", ErrInvalidInput, id, o.ID, err)
	}
	return out, true, nil
}

// encodeCapability writes a typed value back as a field-group, dropping empty
// fields so a round trip does not fatten the file with zero values.
func encodeCapability(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var group map[string]any
	if err := json.Unmarshal(raw, &group); err != nil {
		return nil, err
	}
	if group == nil {
		group = map[string]any{}
	}
	return group, nil
}

// DisplayTitle is the ONE answer to "what is this object called": its name,
// else its id.
func (o Object) DisplayTitle() string {
	if o.Name != "" {
		return o.Name
	}
	return o.ID
}

// ParentID is the tree backbone read directly — the single most common lookup,
// and the one the index walks. Empty means a root.
func (o Object) ParentID() string {
	return o.Parent
}

// SetCapability stores a typed value as a capability field-group.
func (o *Object) SetCapability(id string, v any) error {
	group, err := encodeCapability(v)
	if err != nil {
		return fmt.Errorf("%w: encode capability %q: %v", ErrInvalidInput, id, err)
	}
	if o.Capabilities == nil {
		o.Capabilities = map[string]map[string]any{}
	}
	o.Capabilities[id] = group
	return nil
}

// ObjectAddCapabilityInput gives an object a capability: its fields, filled
// with the capability's defaults where Fields does not set them. When the
// capability declares a workflow, its state starts at the workflow's initial.
type ObjectAddCapabilityInput struct {
	ID         string         `json:"id"               jsonschema:"The object; required"`
	Capability string         `json:"capability"       jsonschema:"The capability to give; declared by a pack; required"`
	Fields     map[string]any `json:"fields,omitempty" jsonschema:"Its fields; the schema's defaults fill the rest, and a missing required field is refused"`
}

// ObjectAddCapabilityOutput is the object as written, and the fields the
// defaults filled in.
type ObjectAddCapabilityOutput struct {
	Object    Object   `json:"object"`
	Defaulted []string `json:"defaulted,omitempty" jsonschema:"Fields the capability's defaults filled in"`
}

// ObjectRemoveCapabilityInput takes a capability off an object: its field-group
// is removed. Only a capability the object carries itself — not one its type,
// an aspect or a policy brings — and none another carried capability uses.
type ObjectRemoveCapabilityInput struct {
	ID         string `json:"id"         jsonschema:"The object; required"`
	Capability string `json:"capability" jsonschema:"The capability to remove; required"`
}

// ObjectRemoveCapabilityOutput is the object as written.
type ObjectRemoveCapabilityOutput struct {
	Object Object `json:"object"`
}
