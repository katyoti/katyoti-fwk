// SPDX-License-Identifier: Apache-2.0

package api

// THE MULTI-CODE MATCH: an application source follows an
// application by its code, and a catalog's search finds a code but ranks it
// among the names that hold it, so an application typed by its code could
// fall past the limit. Every application connector puts the code matches
// first, before it cuts the list, through these helpers - once, here.

import "strings"

// HasCode tells whether one of codes - codes separated by « ; » - is q, a code
// matched whole, case and surrounding spaces ignored. An empty q matches none.
func HasCode(codes, q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	for _, c := range strings.Split(codes, ";") {
		if c = strings.TrimSpace(c); c != "" && strings.EqualFold(c, q) {
			return true
		}
	}
	return false
}

// CodeMatchesFirst puts first the items one of whose codes (code reads them,
// as HasCode does) is query, keeping the order within the matches and within
// the rest; an empty query leaves items as they are. Call it before the list
// is cut to its limit.
func CodeMatchesFirst[T any](items []T, code func(T) string, query string) []T {
	q := strings.TrimSpace(query)
	if q == "" {
		return items
	}
	first := make([]T, 0, len(items))
	var rest []T
	for _, it := range items {
		if HasCode(code(it), q) {
			first = append(first, it)
		} else {
			rest = append(rest, it)
		}
	}
	return append(first, rest...)
}
