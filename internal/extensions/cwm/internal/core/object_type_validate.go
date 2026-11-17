// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Type validation for objects (D1/D5).
//
// Two properties are deliberate and worth stating, because both are the
// difference between a safety feature that survives and one that gets switched
// off:
//
//  1. NO DECLARATIONS ⇒ NO VALIDATION. A workspace that has not migrated its 41
//     type declarations behaves exactly as it did. Refusing every write the
//     moment the feature exists is how a check gets permanently disabled.
//  2. WARN IS NOT SILENCE. An undeclared type is logged with the object id, so
//     the gap is findable before the flip to error. Warn that nobody can see is
//     the same as off.

// lookupObjectType resolves a declared object-type by name.
func (s *Service) lookupObjectType(name string) (api.ObjectType, bool) {
	s.objTypesMu.RLock()
	defer s.objTypesMu.RUnlock()
	t, ok := s.objTypes[name]
	if !ok {
		// An unconverted repository's retired types read in their legacy
		// shape (migrate_legacy.go).
		return s.legacyObjectType(name)
	}
	return t, ok
}

// isModuleLevel reports whether an object sits at the module level: its type
// carries the module capability, or - its type not declared here - the
// object carries it itself. Never its type's name.
func (s *Service) isModuleLevel(o api.Object) bool {
	if t, ok := s.lookupObjectType(o.Type); ok {
		return t.Level() == api.CapModule
	}
	_, has := o.Capabilities[api.CapModule]
	return has
}

// declaredTypeCount reports how many object-types are declared. Zero is the
// switch that keeps validation off for an unmigrated workspace.
func (s *Service) declaredTypeCount() int {
	s.objTypesMu.RLock()
	defer s.objTypesMu.RUnlock()
	return len(s.objTypes)
}

// typeValidationMode returns the effective mode, honouring the no-declarations
// rule above.
func (s *Service) typeValidationMode() string {
	if s.declaredTypeCount() == 0 {
		return api.TypeValidationOff
	}
	switch s.Config.ObjectTypeValidation {
	case api.TypeValidationOff, api.TypeValidationWarn:
		return s.Config.ObjectTypeValidation
	default:
		// Empty or unrecognised ⇒ error: CWM guarantees every stored object
		// is valid, so a write that would store an invalid one is refused.
		// An unrecognised mode must not relax that — a typo is not a setting.
		return api.TypeValidationError
	}
}

// validateObjectType checks an object before it is written: its type, its
// capability names and groups (against their schemas), its edges, its aspects,
// its placement. Returns an error only in "error" mode; "warn" logs and accepts.
func (s *Service) validateObjectType(obj api.Object, verb string) error {
	return s.validateObjectIn(obj, verb, s.typeValidationMode())
}

// validateObjectIn is validateObjectType in a given mode — the scan runs it in
// error mode over what is stored, whatever the write mode is. It answers the
// first problem; validationErrors answers them all.
func (s *Service) validateObjectIn(obj api.Object, verb, mode string) error {
	if errs := s.validationErrors(obj, verb, mode); len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// validationErrors runs every check and keeps one error per check, so an
// object's errors can be listed and a write refused only for what it adds.
// Without a type nothing further can be checked: that one error is all.
func (s *Service) validationErrors(obj api.Object, verb, mode string) []error {
	if mode == api.TypeValidationOff {
		return nil
	}
	if obj.Type == "" {
		// An empty type is wrong in every mode — there is nothing to look up and
		// nothing downstream can resolve a bundle from it.
		if mode == api.TypeValidationError {
			return []error{api.Invalid(fmt.Errorf("%w: %s: object %q declares no type", api.ErrInvalidInput, verb, obj.ID),
				"cwm.noType", map[string]any{"id": obj.ID})}
		}
		slog.Warn("cwm: object declares no type", "verb", verb, "id", obj.ID)
		return nil
	}
	t, ok := s.lookupObjectType(obj.Type)
	if ok && verb == "ObjectCreate" && s.isLegacyType(obj.Type) {
		return []error{legacyCreate(obj, verb)}
	}
	if !ok {
		if mode == api.TypeValidationError {
			return []error{api.Invalid(fmt.Errorf("%w: %s: object %q has undeclared type %q — declare an object-type contribution for it",
				api.ErrInvalidInput, verb, obj.ID, obj.Type), "cwm.unknownType", map[string]any{"id": obj.ID, "type": obj.Type})}
		}
		slog.Warn("cwm: undeclared object type",
			"verb", verb, "id", obj.ID, "type", obj.Type,
			"hint", "declare an object-type contribution; this becomes an error after the migration (D5)")
		return nil
	}
	var errs []error
	for _, err := range []error{
		s.checkDescriptionBudget(obj, t, verb),
		s.checkInstanceCapabilities(obj, mode, verb),
		s.checkObjectEdges(obj, mode, verb),
		s.checkCapabilityGroups(obj, mode, verb),
		s.checkAspects(obj, mode, verb),
		checkBodyEcho(obj, verb),
		s.checkObjectParentChild(obj, verb),
	} {
		if err != nil {
			errs = append(errs, err)
		}
	}
	if t.Abstract {
		// Abstract is refused in BOTH modes: it is not a migration gap, it is an
		// explicit statement that this type may not be instantiated.
		errs = append(errs, api.Invalid(fmt.Errorf("%w: %s: type %q is abstract and may not be instantiated", api.ErrInvalidInput, verb, obj.Type),
			"cwm.abstractType", map[string]any{"type": obj.Type}))
	}
	return errs
}

// checkWrite refuses a write for the problems it ADDS: every validation error
// and, in error mode, every link to an object that does not exist — compared
// with the stored object (prior; nil for a create). An object read with errors
// can be written as long as the write adds none, so it is repaired one step at
// a time; the errors it still carries stay reported on read.
func (s *Service) checkWrite(obj api.Object, prior *api.Object, verb string) error {
	return s.checkWriteWith(obj, prior, verb, nil)
}

// checkWriteWith is checkWrite with the existence of a link's target answered
// by known (nil: the index) — a change set's staged creates exist for its own
// writes.
func (s *Service) checkWriteWith(obj api.Object, prior *api.Object, verb string, known func(string) bool) error {
	mode := s.typeValidationMode()
	if err := s.checkStatesInWorkflow(obj, prior, verb, mode); err != nil {
		return err
	}
	errs := s.validationErrors(obj, verb, mode)
	links := s.writeLinkProblems(obj, mode, known)
	if len(errs) == 0 && len(links) == 0 {
		return nil
	}
	had := map[string]bool{}
	if prior != nil {
		for _, e := range s.validationErrors(*prior, verb, mode) {
			had[e.Error()] = true
		}
		for _, d := range s.writeLinkProblems(*prior, mode, known) {
			had[d.Label] = true
		}
	}
	for _, e := range errs {
		if !had[e.Error()] {
			return e
		}
	}
	for _, d := range links {
		if !had[d.Label] {
			return api.Invalid(fmt.Errorf("%w: %s: object %q: %s — a link names an object of the project", api.ErrInvalidInput, verb, obj.ID, d.Label),
				"cwm.brokenLink", map[string]any{"id": obj.ID, "link": d.Label})
		}
	}
	return nil
}

// checkStatesInWorkflow refuses a write that would store a state its workflow
// does not have, unless the object already had that very state (an object
// stored invalid is repaired one write at a time). Logged in warn mode, nothing
// when validation is off.
func (s *Service) checkStatesInWorkflow(obj api.Object, prior *api.Object, verb, mode string) error {
	if mode == api.TypeValidationOff {
		return nil
	}
	had := map[string]string{}
	if prior != nil {
		had = s.statesOutsideWorkflow(*prior)
	}
	for slot, problem := range s.statesOutsideWorkflow(obj) {
		if had[slot] == problem {
			continue
		}
		what := "lifecycle"
		if slot != "" {
			what = "capability " + slot
		}
		if mode == api.TypeValidationWarn {
			slog.Warn("cwm: a state outside its workflow", "id", obj.ID, "slot", what, "problem", problem)
			continue
		}
		return fmt.Errorf("%w: %s: object %q: %s: %s", api.ErrInvalidInput, verb, obj.ID, what, problem)
	}
	return nil
}

// writeLinkProblems is linkProblems as a write sees it: refused in error mode,
// logged in warn mode, nothing when validation is off.
func (s *Service) writeLinkProblems(obj api.Object, mode string, known func(string) bool) []api.ObjectDiagnostic {
	if mode == api.TypeValidationOff {
		return nil
	}
	links := s.linkProblemsWith(obj, known)
	if mode == api.TypeValidationWarn {
		for _, d := range links {
			slog.Warn("cwm: link to an object that does not exist", "id", obj.ID, "problem", d.Label)
		}
		return nil
	}
	return links
}

// checkDescriptionBudget enforces the type's `description_max_chars` cap.
//
// NodeCreate enforced this at the write path; ObjectCreate did not, so for the
// 15 shipped types that declare a cap it had silently become unlimited. A
// missing gate is invisible in exactly the way a missing feature is not: it
// shows up as something being ALLOWED that should not have been, and no test
// notices unless it tries the forbidden thing.
//
// It is refused in BOTH warn and error mode, like Abstract and unlike an
// undeclared type. Warn exists for the MIGRATION gap — a type nobody has
// declared yet — and a budget overrun is not that: the type is declared, the
// cap is declared, and the value breaks it.
func (s *Service) checkDescriptionBudget(obj api.Object, t api.ObjectType, verb string) error {
	if t.DescriptionMaxChars <= 0 {
		return nil
	}
	if n := len([]rune(obj.Description)); n > t.DescriptionMaxChars {
		// Counted in RUNES, not bytes. The node path measured bytes, which cut
		// an accented description short of its stated cap — the budget is about
		// how much a reader has to read, and "é" is one character to them.
		return api.Invalid(fmt.Errorf("%w: %s: description %d chars exceeds type %q cap of %d",
			api.ErrBudgetExceeded, verb, n, obj.Type, t.DescriptionMaxChars),
			"cwm.descriptionTooLong", map[string]any{"type": obj.Type, "chars": n, "max": t.DescriptionMaxChars})
	}
	return nil
}

// checkObjectParentChild enforces containment on the object path — the port of
// NodeCreate's checkParentChildCompatibility (gate 2).
//
// THE RULE IS A UNION, AND THE DIRECTION MATTERS. The parent side allows when it
// declares no AllowedChildren or lists this type; the child side allows when it
// declares no AcceptParents or lists the parent type (or "*"). A write is
// refused only when BOTH sides reject it. That is deliberately permissive:
// either side alone saying nothing means "no restriction from here", so a
// workspace that has not declared containment is unconstrained rather than
// frozen.
//
// UNDECLARED TYPES IMPOSE NOTHING. If either type has no declaration, that side
// cannot reject — matching the node path, and matching the rule that a missing
// declaration is a migration gap, never a denial.
//
// This gate could only be ported AFTER the 41 legacy declarations became object
// types, because AllowedChildren is the UNION of the old
// allowed_children and allowed_item_types. Enforced before that union existed,
// it would have refused every node-child of the 10 types declaring both — a
// gate that is wrong is worse than a gate that is missing, because the first
// breaks working flows while the second only fails to catch new mistakes.
func (s *Service) checkObjectParentChild(obj api.Object, verb string) error {
	parentID := obj.ParentID()
	if parentID == "" {
		return nil // a root has no parent to be compatible with
	}
	parent, ok := s.index().get(parentID)
	if !ok {
		// An unresolvable parent is NOT this check's business. It would be a
		// dangling reference, which is a different defect with a different
		// report; refusing it here would attribute it to containment.
		return nil
	}
	childType, childDeclared := s.lookupObjectType(obj.Type)
	parentType, parentDeclared := s.lookupObjectType(parent.Type)
	if !parentDeclared {
		parentType = api.ObjectType{Name: parent.Type}
	}

	parentAllows := !parentDeclared || parentType.AllowsChild(obj.Type)
	childAllows := !childDeclared || childType.IsParentAllowed(parentType)
	if parentAllows || childAllows {
		return nil
	}
	// Both sides rejected. Name BOTH lists: knowing only one of them turns
	// fixing the declaration into guesswork.
	return api.Invalid(fmt.Errorf("%w: %s: type %q does not allow child type %q "+
		"(parent.allowed_children: %v; child.accept_parents: %v)",
		api.ErrInvalidInput, verb, parent.Type, obj.Type,
		parentType.AllowedChildren, childType.AcceptParents),
		"cwm.childTypeNotAllowed", map[string]any{"parentType": parent.Type, "type": obj.Type})
}

// checkInstanceCapabilities refuses a capability group that names no DECLARED
// capability (concepts.md §4, "Capabilities on the instance").
//
// An object may carry a capability its type does not list — that is the
// point of instance capabilities, and it is accepted here as long as the
// capability is declared SOMEWHERE. What is refused is a group nobody
// declared: it would be stored, indexed and read by nobody, which is the
// silent shape this model exists to close. The same two rules as the type
// check: no declared capabilities ⇒ no check (an unmigrated workspace behaves
// as before); warn logs, error refuses.
func (s *Service) checkInstanceCapabilities(obj api.Object, mode, verb string) error {
	caps := s.decl().Capabilities
	if len(caps) == 0 {
		return nil
	}
	declared := make(map[string]bool, len(caps))
	typeLevel := map[string]bool{}
	for _, c := range caps {
		declared[c.Name] = true
		if c.IsTypeLevel() {
			typeLevel[c.Name] = true
		}
	}
	for group := range obj.Capabilities {
		if typeLevel[group] {
			// A type-level capability is set by a type's section and carries
			// no group on objects: a group under its name is read by nobody.
			if mode == api.TypeValidationError {
				return fmt.Errorf("%w: %s: object %q carries a group %q, but that capability is set on a type (its section) and carries no group on objects",
					api.ErrInvalidInput, verb, obj.ID, group)
			}
			slog.Warn("cwm: a group of a type-level capability on object", "verb", verb, "id", obj.ID, "group", group)
			continue
		}
		if group == "" || declared[group] || s.legacyCapability(group) {
			continue
		}
		if mode == api.TypeValidationError {
			return api.Invalid(fmt.Errorf("%w: %s: object %q carries capability group %q, which no pack declares — declare the capability or drop the group",
				api.ErrInvalidInput, verb, obj.ID, group), "cwm.unknownCapability", map[string]any{"id": obj.ID, "capability": group})
		}
		slog.Warn("cwm: undeclared capability group on object",
			"verb", verb, "id", obj.ID, "group", group,
			"hint", "declare a capability contribution for it; this becomes an error after the migration (D5)")
	}
	return nil
}

// checkAspects refuses an undeclared aspect on an object (7g): error refuses,
// warn logs, and with no aspects declared at all the check is off.
func (s *Service) checkAspects(obj api.Object, mode, verb string) error {
	if len(obj.Aspects) == 0 || len(s.decl().Aspects) == 0 {
		return nil
	}
	for _, name := range obj.Aspects {
		if _, ok := s.lookupAspect(name); ok {
			continue
		}
		if mode == api.TypeValidationError {
			return api.Invalid(fmt.Errorf("%w: %s: object %q names aspect %q, which no pack declares — declare the aspect or drop it",
				api.ErrInvalidInput, verb, obj.ID, name), "cwm.unknownAspect", map[string]any{"id": obj.ID, "aspect": name})
		}
		slog.Warn("cwm: undeclared aspect on object", "verb", verb, "id", obj.ID, "aspect", name,
			"hint", "declare an aspect contribution; this becomes an error after the migration (D5)")
	}
	return nil
}
