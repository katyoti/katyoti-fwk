// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// Containment is the UNION of the two sides: a parent's allowed_children and a
// child's accept_parents. Each method answers for one side, and an empty list
// on a side means "no restriction from here", never "nothing allowed".
func TestObjectTypeContainmentSides(t *testing.T) {
	for _, tc := range []struct {
		name   string
		t      *ObjectType
		parent ObjectType
		want   bool
	}{
		{"nil type", nil, ObjectType{Name: "module"}, true},
		{"no accept_parents", &ObjectType{Name: "note"}, ObjectType{Name: "module"}, true},
		{"a wildcard", &ObjectType{Name: "note", AcceptParents: []string{"*"}}, ObjectType{Name: "anything"}, true},
		{"a listed parent", &ObjectType{Name: "note", AcceptParents: []string{"goal"}}, ObjectType{Name: "goal"}, true},
		{"an unlisted parent", &ObjectType{Name: "note", AcceptParents: []string{"goal"}}, ObjectType{Name: "feature"}, false},
		// : an entry may name a capability the parent's type
		// carries - the level, whatever the type is called.
		{"a level capability", &ObjectType{Name: "stream", AcceptParents: []string{"module"}}, ObjectType{Name: "software-module", Capabilities: []string{"module"}}, true},
		{"a level the parent lacks", &ObjectType{Name: "stream", AcceptParents: []string{"module"}}, ObjectType{Name: "software-project", Capabilities: []string{"project"}}, false},
	} {
		if got := tc.t.IsParentAllowed(tc.parent); got != tc.want {
			t.Errorf("%s: IsParentAllowed(%q) = %v, want %v", tc.name, tc.parent.Name, got, tc.want)
		}
	}
	if (&ObjectType{AcceptParents: []string{"*", "module"}}).NamesParentType("system-module") {
		t.Error("NamesParentType: '*' and a capability name no parent type")
	}
	parent := &ObjectType{Name: "module", AllowedChildren: []string{"goal"}}
	if !parent.AllowsChild("goal") || parent.AllowsChild("note") {
		t.Error("AllowsChild: the parent side lists goal only")
	}
	if !(&ObjectType{Name: "group"}).AllowsChild("note") {
		t.Error("AllowsChild: no allowed_children means no restriction")
	}
}
