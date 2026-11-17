// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectEffectiveSurface resolves everything an object OFFERS — actions and
// badges — from its type and from every capability
// that type composes, plus the policies that attach themselves to it.
//
// WHY THIS EXISTS. Phase 1 gave capabilities somewhere to declare contributions;
// nothing read them. A slot nobody resolves is the "declared and read by nobody"
// shape this whole refactor exists to close, so the capability side is only real
// once something composes it.
//
// PRECEDENCE: LISTS accumulate — type first, then capabilities in declaration
// order, then policies. The type's own binding is the CURATED one (its label,
// confirm and when were chosen for that type), so it is seen first and wins
// the dedupe. A type's own badges are its `pane` section's, read by the object
// pane; the badges here are the capabilities' — each over its own group.
func (s *Service) ObjectEffectiveSurface(ctx stdctx.Context, in api.ObjectEffectiveSurfaceInput) (api.ObjectEffectiveSurfaceOutput, error) {
	if in.ID == "" {
		return api.ObjectEffectiveSurfaceOutput{}, fmt.Errorf("%w: ID required", api.ErrInvalidInput)
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectEffectiveSurfaceOutput{}, err
	}
	obj, ok := ix.get(in.ID)
	if !ok {
		return api.ObjectEffectiveSurfaceOutput{}, api.ObjectNotFound(in.ID)
	}

	out := api.ObjectEffectiveSurfaceOutput{ObjectID: obj.ID, Type: obj.Type}
	seenAction := map[string]bool{}
	seenBadge := map[string]bool{}

	addActions := func(refs []api.ActionRef, source string) {
		for _, r := range refs {
			if r.URI == "" || seenAction[r.URI] {
				continue
			}
			seenAction[r.URI] = true
			out.Actions = append(out.Actions, api.SurfaceAction{Ref: r, Source: source})
		}
	}
	addBadges := func(defs []api.BadgeDef, capability string) {
		for _, d := range defs {
			key := capability + "|" + d.Field
			if d.Field == "" || seenBadge[key] {
				continue
			}
			seenBadge[key] = true
			d.Capability = capability
			out.Badges = append(out.Badges, d)
		}
	}

	// 1 — the TYPE's own bindings. First, so a curated binding wins the dedupe.
	ot, typeDeclared := s.lookupObjectType(obj.Type)
	if typeDeclared {
		addActions(ot.Actions, "type")
	}

	// 2 — every capability the type composes, in declaration order.
	declared := s.capabilitiesByName()
	for _, id := range s.effectiveCapabilities(obj) { // type ∪ instance (7b)
		c, ok := declared[id]
		if !ok {
			// An undeclared capability is NOT invented here. It is reported, so
			// "this type names a capability nobody declares" is visible rather
			// than silently contributing nothing.
			out.UndeclaredCapabilities = append(out.UndeclaredCapabilities, id)
			continue
		}
		out.Capabilities = append(out.Capabilities, id)
		s.foldCapability(&out, c, addActions, addBadges)
	}

	// 3 — capabilities that attach themselves to it (the memory), matched
	// against what it carries before attachment.
	carried := s.effectiveCapabilities(obj)
	for _, c := range s.decl().Capabilities {
		if ok, _ := c.AttachesTo(obj.Type, carried); !ok {
			continue
		}
		if containsString(out.Capabilities, c.Name) {
			continue // already composed by the type; not counted twice
		}
		out.Capabilities = append(out.Capabilities, c.Name)
		s.foldCapability(&out, c, addActions, addBadges)
	}

	// 4 — the policies that apply to it.
	for _, p := range s.policiesFor(obj) {
		out.Policies = append(out.Policies, p.Name)
	}
	return out, nil
}

// foldCapability merges one capability's contributions into the surface.
func (s *Service) foldCapability(out *api.ObjectEffectiveSurfaceOutput, c api.Capability,
	addActions func([]api.ActionRef, string), addBadges func([]api.BadgeDef, string)) {

	addActions(c.Actions, "capability:"+c.Name)
	addBadges(c.Badges, c.Name)
}

// capabilitiesByName indexes the declared capabilities for lookup.
func (s *Service) capabilitiesByName() map[string]api.Capability {
	out := make(map[string]api.Capability, len(s.decl().Capabilities))
	for _, c := range s.decl().Capabilities {
		out[c.Name] = c
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
