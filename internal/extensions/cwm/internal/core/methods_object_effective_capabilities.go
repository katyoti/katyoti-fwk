// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Effective capabilities for an Object — the capabilities in force on it.
//
// Two sources compose, in this order:
//  1. the object's effective set — its type's bundle, its aspects', its own
//     groups (effectiveCapabilities) — each tagged with who brings it;
//  2. capabilities that attach themselves to it (`attach:`, a ShapeFilter
//     matched against the first source: an attached capability never
//     attaches another).
//
// Order matters only for de-duplication: the first source wins over an
// attached capability with the same id. A policy is never a capability and is
// never listed here.

// ObjectEffectiveCapabilities resolves the capabilities in force on one object.
func (s *Service) ObjectEffectiveCapabilities(ctx stdctx.Context, in api.ObjectEffectiveCapabilitiesInput) (api.ObjectEffectiveCapabilitiesOutput, error) {
	if in.ID == "" {
		return api.ObjectEffectiveCapabilitiesOutput{}, fmt.Errorf("%w: ID required", api.ErrInvalidInput)
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectEffectiveCapabilitiesOutput{}, err
	}
	obj, ok := ix.get(in.ID)
	if !ok {
		return api.ObjectEffectiveCapabilitiesOutput{}, api.ObjectNotFound(in.ID)
	}
	return api.ObjectEffectiveCapabilitiesOutput{Capabilities: s.effectiveRefs(obj)}, nil
}

// effectiveRefs is the effective capability set of an object VALUE — it needs
// no index, so a write can ask it about the object it is about to store.
func (s *Service) effectiveRefs(obj api.Object) []api.CapabilityRef {
	out := api.ObjectEffectiveCapabilitiesOutput{Capabilities: []api.CapabilityRef{}}
	seen := map[string]bool{}

	// 1 — what the object itself carries. Each capability group present IS an
	// capability in force; the object is the most specific statement there is.
	//
	// 7b: the EFFECTIVE set — the type's bundle ∪ the aspects' ∪ the instance's
	// groups — read through the one function. The source says who brings each:
	// "type" when the type declares it (whether or not the object fills its
	// fields), "aspect:<name>" when an aspect does, "object" when only the
	// object's own group does. The ref carries what the DECLARATION says
	// (schema, label, run), so a caller can tell which capabilities are
	// runnable — the list NodeRunChecks had.
	declared := s.capabilitiesByName()
	source := map[string]string{}
	if t, ok := s.lookupObjectType(obj.Type); ok {
		for _, id := range t.CapabilitiesInForce() {
			source[id] = "type"
		}
	}
	for _, name := range obj.Aspects {
		if a, ok := s.lookupAspect(name); ok {
			for _, id := range a.Capabilities {
				if source[id] == "" {
					source[id] = "aspect:" + name
				}
			}
		}
	}
	for _, id := range s.effectiveCapabilities(obj) {
		if seen[id] {
			continue
		}
		seen[id] = true
		ref := api.CapabilityRef{ID: id}
		if c, ok := declared[id]; ok {
			ref = c.AsRef()
		}
		ref.Source = source[id]
		if ref.Source == "" {
			ref.Source = "object"
		}
		out.Capabilities = append(out.Capabilities, ref)
	}

	// 2 — capabilities that ATTACH themselves by selector (`attach:`, the
	// memory every object has). By shape only (ShapeFilter): a filter over an
	// object's own data would make its shape depend on its content. Matched
	// against the set above, so an attached capability never attaches another.
	// A rule is a Policy, never listed here.
	carried := make([]string, 0, len(out.Capabilities))
	for _, r := range out.Capabilities {
		carried = append(carried, r.ID)
	}
	for _, c := range s.decl().Capabilities {
		if seen[c.Name] {
			continue
		}
		if ok, _ := c.AttachesTo(obj.Type, carried); !ok {
			continue
		}
		seen[c.Name] = true
		a := c.AsRef()
		a.Source = "attached"
		out.Capabilities = append(out.Capabilities, a)
	}
	return out.Capabilities
}
