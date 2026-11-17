// SPDX-License-Identifier: Apache-2.0

package core

import "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// policiesFor is every policy that applies to obj, in catalog order — rules
// put on objects by shape (type, effective capabilities), never in the
// object's capability set.
func (s *Service) policiesFor(obj api.Object) []api.Policy {
	var out []api.Policy
	seen := map[string]bool{}
	var caps []string
	for _, r := range s.effectiveRefs(obj) {
		caps = append(caps, r.ID)
	}
	for _, p := range s.decl().Policies {
		if p.Name == "" || seen[p.Name] {
			continue
		}
		if ok, _ := p.AppliesTo(obj.Type, caps); !ok {
			continue
		}
		seen[p.Name] = true
		out = append(out, p)
	}
	return out
}
