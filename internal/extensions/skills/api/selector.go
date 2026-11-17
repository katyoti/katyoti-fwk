// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/refmatch"
)

// ParseSkillSelector turns one `skills:` entry (or a `skill:` plugin-ref Name)
// into a refmatch.Selector. Forms:
//
//	""  / "*"                          → all advertised skills
//	"doc-create"                       → exact name (reaches unadvertised)
//	"cwm-*"                            → glob over advertised names (any provider)
//	"ktw-methods:/skill/author-schema" → one named skill, provider-scoped (exact)
//	"ktw-methods:/skill/author-*"      → provider-scoped glob
//	"ktw-weather:/skill/*"             → all advertised skills of one provider
//
// The ONLY provider-scoped form is the SINGULAR resource-URI
// `<provider>:/skill/<name-or-glob>` — the same `/skill/` grammar used to LOAD a
// skill. There is no plural `/skills/` form. Anything without `:/skill/` is a
// bare name/glob with no provider scope.
func ParseSkillSelector(entry string) refmatch.Selector {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return refmatch.Selector{Name: "*"}
	}
	// Provider-scoped form: <provider>:/skill/<name-or-glob>
	if i := strings.Index(entry, ":/skill/"); i > 0 {
		return refmatch.Selector{
			Provider: entry[:i],
			Name:     entry[i+len(":/skill/"):],
		}
	}
	// Bare name or bare glob (no provider scope).
	return refmatch.Selector{Name: entry}
}

// SkillItems projects []SkillInfo into refmatch.Items, carrying each skill's
// real Advertised flag. Feed it the FULL catalog (SkillsFromCatalogAll) so the
// resolver's advertised gate is live: a bulk selector (`*`, glob) matches only
// advertised items, while an exact-name selector reaches an unadvertised one.
func SkillItems(skills []SkillInfo) []refmatch.Item {
	out := make([]refmatch.Item, len(skills))
	for i, s := range skills {
		out[i] = refmatch.Item{Provider: s.Provider, Name: s.Name, Advertised: s.Advertised}
	}
	return out
}
