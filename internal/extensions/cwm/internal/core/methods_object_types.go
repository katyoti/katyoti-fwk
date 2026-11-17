// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"slices"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The declared types as the GUI's forms and pickers read them: one type
// lookup, one type listing, the aspect picker and the child picker.

// ObjectTypeGet returns one declared object-type.
func (s *Service) ObjectTypeGet(_ stdctx.Context, in api.ObjectTypeGetInput) (api.ObjectTypeGetOutput, error) {
	if in.Name == "" {
		return api.ObjectTypeGetOutput{}, fmt.Errorf("%w: Name required", api.ErrInvalidInput)
	}
	t, ok := s.lookupObjectType(in.Name)
	if !ok {
		return api.ObjectTypeGetOutput{}, fmt.Errorf("%w: object type %q", api.ErrNotFound, in.Name)
	}
	return api.ObjectTypeGetOutput{ObjectType: t}, nil
}

// ObjectTypeList returns every declared object-type, sorted by Name.
func (s *Service) ObjectTypeList(_ stdctx.Context, _ api.ObjectTypeListInput) (api.ObjectTypeListOutput, error) {
	out := api.ObjectTypeListOutput{ObjectTypes: []api.ObjectType{}}
	for _, t := range s.objectTypes() {
		out.ObjectTypes = append(out.ObjectTypes, t)
	}
	sort.Slice(out.ObjectTypes, func(i, j int) bool { return out.ObjectTypes[i].Name < out.ObjectTypes[j].Name })
	return out, nil
}

// ObjectAspectList returns every declared aspect, sorted by Name — the "+ Aspect"
// picker (objects-what-the-user-sees.md section 5).
func (s *Service) ObjectAspectList(_ stdctx.Context, _ api.ObjectAspectListInput) (api.ObjectAspectListOutput, error) {
	out := api.ObjectAspectListOutput{Aspects: []api.Aspect{}}
	out.Aspects = append(out.Aspects, s.decl().Aspects...)
	sort.Slice(out.Aspects, func(i, j int) bool { return out.Aspects[i].Name < out.Aspects[j].Name })
	return out, nil
}

// ObjectAllowedChildren is the create picker under one parent.
//
// ONE PICKER, and the merge had to settle a contradiction. The node side had
// two: NodeListAllowedChildTypes offered a type only when it EXPLICITLY named
// the parent in accept_parents, while NodeListAllowedItemTypes offered EVERY
// registered kind when the parent restricted nothing. So an empty
// accept_parents meant "never offered" on one side and "offered everywhere" on
// the other — the same empty list, two opposite readings.
//
// The reading kept is the one the WRITE PATH already uses: empty means no
// restriction (ObjectType.IsParentAllowed, checkObjectParentChild). A
// picker that reads a declaration differently from the verb behind it offers
// options the create refuses, or hides ones it would accept — and the node
// child picker did the second: NodeCreate accepted a type with an empty
// accept_parents under any parent the picker never offered it for.
//
// So, in two cases:
//
//   - The parent CURATED its children (allowed_children non-empty): its list,
//     plus every type that explicitly names this parent type - not '*', which
//     names none. A curated parent offers what it can hold.
//   - The parent curated NOTHING: under an object below the levels, the types
//     naming it - its type or a capability its type carries; one accepting
//     any parent ('*') or saying nothing is not offered there
//     its write staying unrestricted. Under a module or a project, and at the
//     universe's root, every type the child side admits.
//
// Both cases stay a SUBSET of what ObjectCreate accepts, so no offered type is
// refused on create. Abstract types are never offered: they cannot be
// instantiated.
func (s *Service) ObjectAllowedChildren(ctx stdctx.Context, in api.ObjectAllowedChildrenInput) (api.ObjectAllowedChildrenOutput, error) {
	out := api.ObjectAllowedChildrenOutput{Choices: []api.TypeChoice{}}
	types := s.objectTypes()
	if len(types) == 0 {
		return out, nil
	}
	var parentType string
	var parentObj *api.Object
	if in.ParentID != "" {
		if err := s.index().ensureLoaded(); err != nil {
			return out, err
		}
		parent, ok := s.index().get(in.ParentID)
		if !ok {
			return out, api.ObjectNotFound(in.ParentID)
		}
		parentType = parent.Type
		parentObj = &parent
	}
	// A type's accept_when is a condition over the parent
	// evaluated in the object vocabulary as a panel's when is - the
	// capabilities' functions callable, a costly one not run (a display
	// switch) - the parent's environment made
	// once. Fail closed: a condition that does not hold, or cannot be
	// evaluated, leaves the type out. No parent: not evaluated.
	var parentEnv map[string]any
	accepts := func(t api.ObjectType) bool {
		if t.AcceptWhen == "" || parentObj == nil {
			return true
		}
		if parentEnv == nil {
			parentEnv = s.objectEnv(ctx, *parentObj, true)
		}
		disabled, _ := evalPredicate(s.objectCache(), t.AcceptWhen, parentEnv)
		return !disabled
	}
	pt, parentDeclared := s.lookupObjectType(parentType)
	if !parentDeclared {
		pt = api.ObjectType{Name: parentType}
	}

	seen := map[string]bool{}
	add := func(t api.ObjectType, named bool) {
		if t.Abstract || seen[t.Name] || !accepts(t) {
			return
		}
		seen[t.Name] = true
		c := choiceFromObjectType(t)
		c.Named = named
		out.Choices = append(out.Choices, c)
	}
	parentCurated := parentDeclared && len(pt.AllowedChildren) > 0
	// Parent side: what it names, when the type exists.
	if parentCurated {
		for _, name := range pt.AllowedChildren {
			if ct, ok := types[name]; ok {
				add(ct, true)
			}
		}
	}
	// Child side.
	for _, ct := range types {
		if seen[ct.Name] {
			continue
		}
		if !ct.IsParentAllowed(pt) {
			continue
		}
		// Under a curated parent, only a type naming ITS TYPE adds to its list -
		// one accepting any parent ("*") or a class of parents (a capability,
		// ) does not: « + New » on an object offers what that object
		// can hold.
		if parentCurated && !ct.NamesParentType(parentType) {
			continue
		}
		// Under an object below the levels (a goal, a feature, a verdict...),
		// the types naming it - its type or a capability it carries; one
		// accepting any parent, or saying nothing, keeps its write unrestricted
		// but is not offered there. A module or a project curating
		// nothing, and the universe's root, offer what the child side admits.
		if !parentCurated && in.ParentID != "" && pt.Level() == "" && !ct.NamesParentType(parentType) && !namesALevel(ct, pt) {
			continue
		}
		// Named: it names this parent's type - "*" accepts any parent and
		// names none.
		add(ct, ct.NamesParentType(parentType))
	}
	sortChoices(out.Choices)
	return out, nil
}

// namesALevel: one of the type's accept_parents entries is a capability the
// parent's type carries.
func namesALevel(ct, parent api.ObjectType) bool {
	for _, p := range ct.AcceptParents {
		if slices.Contains(parent.Capabilities, p) {
			return true
		}
	}
	return false
}

// objectTypes returns a copy of the declared type map, taken under the lock.
func (s *Service) objectTypes() map[string]api.ObjectType {
	s.objTypesMu.RLock()
	defer s.objTypesMu.RUnlock()
	out := make(map[string]api.ObjectType, len(s.objTypes))
	for k, v := range s.objTypes {
		out[k] = v
	}
	return out
}

// choiceFromObjectType projects a type onto a picker row.
func choiceFromObjectType(t api.ObjectType) api.TypeChoice {
	c := api.TypeChoice{Name: t.Name, Label: t.Label, Icon: t.Icon, Description: t.Description, CreateTask: t.CreateTask}
	if c.Label == "" {
		c.Label = t.Name
	}
	if c.Icon == "" {
		c.Icon = t.IconName
	}
	return c
}
