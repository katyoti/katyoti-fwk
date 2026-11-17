// SPDX-License-Identifier: Apache-2.0

// Package refmatch resolves wildcard / glob reference selectors against a set
// of addressable items (provider + name + advertised flag). It is the single
// shared wildcard model behind the task-config `skills:` and `tools:` lists
// (and, later, `plugins:`), so all three resolve consistently.
//
// A caller parses its domain URI into a Selector and calls Match:
//
//	skills:  "*"                         → Selector{Name: "*"}
//	         "doc-create"                → Selector{Name: "doc-create"}        (exact)
//	         "cwm-*"                     → Selector{Name: "cwm-*"}             (glob)
//	         "ktw-agents:/skill/cwm-*"  → Selector{Provider: "ktw-agents", Name: "cwm-*"}
//	tools:   "ktw-weather:weather-service/*"        → Selector{Provider:"ktw-weather", Name:"*", ...}
//	         "ktw-weather:weather-service/get-forecast" → exact
//
// THE RULE (mirrors the skill catalog's "visibility ≠ callability"):
//   - a BULK selector (name is "" / "*" / contains glob metachars) matches
//     only ADVERTISED items — so `*` and `<provider>/*` never pull in the
//     un-advertised long tail;
//   - an EXACT-name selector matches regardless of Advertised — naming a thing
//     explicitly is the opt-in to the un-advertised.
package refmatch

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Item is an addressable thing (a skill, a tool/action) a selector can match.
type Item struct {
	Provider   string
	Name       string
	Advertised bool
}

// Selector is a parsed reference: an optional provider scope + a name glob.
// An empty Name (or "*") means "all".
type Selector struct {
	Provider string // "" = any provider
	Name     string // doublestar glob; "" or "*" = all
}

// Bulk reports whether the selector is a wildcard/glob (vs an exact name).
// Bulk selectors are restricted to advertised items in Match.
func (s Selector) Bulk() bool {
	return s.Name == "" || s.Name == "*" || strings.ContainsAny(s.Name, "*?[{")
}

func (s Selector) matches(it Item) bool {
	if s.Provider != "" && s.Provider != it.Provider {
		return false
	}
	if s.Bulk() && !it.Advertised {
		return false
	}
	pat := s.Name
	if pat == "" {
		pat = "*"
	}
	ok, err := doublestar.Match(pat, it.Name)
	return err == nil && ok
}

// Match returns the items matching ANY selector — union, de-duplicated by
// (provider, name), input item order preserved. Bulk selectors see only
// advertised items; exact-name selectors see all. Empty selectors → nil.
func Match(items []Item, sels []Selector) []Item {
	if len(sels) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(items))
	var out []Item
	for _, it := range items {
		matched := false
		for _, s := range sels {
			if s.matches(it) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		key := it.Provider + "\x00" + it.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
	}
	return out
}
