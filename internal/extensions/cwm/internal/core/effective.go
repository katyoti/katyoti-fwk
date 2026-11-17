// SPDX-License-Identifier: Apache-2.0

package core

import (
	"log/slog"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE ONE FUNCTION (phase 7b, concepts.md §4 "Capabilities on the instance"):
// an object's effective capabilities are its type's default bundle ∪ the
// groups the instance itself carries. Drawing, loading, the panels and the
// effective aspects all read this and nothing else, so an instance-added
// `composition` on a module draws, loads and panels exactly as a type-declared
// one would. Aspects (7g) expand here too, between the type and the instance.

// Load policies (api.ObjectType.Load).
const (
	LoadWithTree   = "with-tree"
	LoadOnDemand   = "on-demand"
	LoadNeverEager = "never-eager"
)

// effectiveCapabilities returns the type's declared list in its declaration
// order, then every instance group the type did not declare, sorted —
// deterministic, duplicates collapsed. An undeclared type contributes nothing
// from the type side; the instance still counts.
func (s *Service) effectiveCapabilities(obj api.Object) []string {
	seen := map[string]bool{}
	var out []string
	if t, ok := s.lookupObjectType(obj.Type); ok {
		// The type's list and the capability of every section it writes.
		for _, id := range t.CapabilitiesInForce() {
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	// ASPECTS (7g): each named aspect expands to its declared list, in the
	// object's order then the aspect's. An undeclared aspect contributes
	// nothing here — validation reports it; expansion never guesses.
	for _, tn := range obj.Aspects {
		if tr, ok := s.lookupAspect(tn); ok {
			for _, id := range tr.Capabilities {
				if id != "" && !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
		}
	}
	var extra []string
	for id := range obj.Capabilities {
		if id != "" && !seen[id] {
			seen[id] = true
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

// hasEffective reports whether the object's effective set includes cap.
func (s *Service) hasEffective(obj api.Object, cap string) bool {
	for _, c := range s.effectiveCapabilities(obj) {
		if c == cap {
			return true
		}
	}
	return false
}

// loadPolicy is how an object is loaded: the type's `load:` override when set,
// else derived from the type's `tree:` — a drawn type comes with the tree,
// anything else is a list read on demand under its parent.
func (s *Service) loadPolicy(obj api.Object) string {
	t, ok := s.lookupObjectType(obj.Type)
	if ok && t.Load != "" {
		return t.Load
	}
	if ok && t.Drawn() {
		return LoadWithTree
	}
	return LoadOnDemand
}

// isDrawn reports whether the object appears as a node of a drawn tree: an
// object loaded with the tree is drawn; an UNDECLARED type is drawn too, so it
// can be noticed rather than vanish (the rule pruneToDrawn had; unchanged).
func (s *Service) isDrawn(obj api.Object) bool {
	if _, ok := s.lookupObjectType(obj.Type); !ok {
		return true
	}
	return s.loadPolicy(obj) == LoadWithTree
}

// lookupAspect resolves a declared aspect by name.
func (s *Service) lookupAspect(name string) (api.Aspect, bool) {
	for _, t := range s.decl().Aspects {
		if t.Name == name {
			return t, true
		}
	}
	return api.Aspect{}, false
}

// dropAspectsNamingUndeclaredCapabilities enforces the declared-once rule at
// LOAD: an aspect that names a capability no pack declares is dropped, with a
// warning naming both — never expanded to a group nobody can read. Only when
// capabilities are declared at all (an unmigrated workspace declares none).
func dropAspectsNamingUndeclaredCapabilities(aspects []api.Aspect, caps []api.Capability) []api.Aspect {
	if len(caps) == 0 {
		return aspects
	}
	declared := make(map[string]bool, len(caps))
	for _, c := range caps {
		declared[c.Name] = true
	}
	kept := aspects[:0:0]
	for _, t := range aspects {
		bad := ""
		for _, id := range t.Capabilities {
			if !declared[id] {
				bad = id
				break
			}
		}
		if bad != "" {
			slog.Warn("cwm: aspect dropped at load — it names an undeclared capability", "aspect", t.Name, "capability", bad)
			continue
		}
		kept = append(kept, t)
	}
	return kept
}
