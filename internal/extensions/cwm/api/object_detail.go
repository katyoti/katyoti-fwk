// SPDX-License-Identifier: Apache-2.0

package api

// THE DECLARED TYPES as the GUI's forms and pickers read them. Item types and
// node types share the ObjectType namespace, so a type lookup is ONE lookup,
// and "which types may live here" is ONE picker (ObjectAllowedChildren). What
// a pane shows of one object is the object pane's (internal/extensions/
// objectpane), composed from these reads.

// ObjectTypeGetInput requests one declared object-type by name.
type ObjectTypeGetInput struct {
	Name string `json:"name" jsonschema:"Object type name (canonical); required"`
}

// ObjectTypeGetOutput carries the whole declaration — the create form reads
// its capability list and body schema from here.
type ObjectTypeGetOutput struct {
	ObjectType ObjectType `json:"object_type"`
}

// ObjectTypeListInput takes no argument today. Filters (by capability, by
// provider) belong here when a caller needs one, not before.
type ObjectTypeListInput struct{}

// ObjectTypeListOutput returns every declared object-type, sorted by Name.
type ObjectTypeListOutput struct {
	ObjectTypes []ObjectType `json:"object_types"`
}

// ObjectAllowedChildrenInput asks what may be created under a parent.
type ObjectAllowedChildrenInput struct {
	ParentID string `json:"parent_id,omitempty" jsonschema:"Parent object id; empty = the project / workspace root"`
}

// ObjectAllowedChildrenOutput is the picker, sorted by Name.
type ObjectAllowedChildrenOutput struct {
	Choices []TypeChoice `json:"choices" jsonschema:"Creatable child types, sorted by Name"`
}

// ObjectAspectListInput takes no argument: the declared aspects are a small
// global set, and the "+ Aspect" chip row offers all of them.
type ObjectAspectListInput struct{}

// ObjectAspectListOutput returns every declared aspect, sorted by Name.
type ObjectAspectListOutput struct {
	Aspects []Aspect `json:"aspects"`
}
