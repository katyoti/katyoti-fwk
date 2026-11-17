// SPDX-License-Identifier: Apache-2.0

// Package variables provides generic ${{domain:key}} variable expansion
// with a pluggable resolver registry. Any package can register a domain
// resolver (e.g., "env", "secret", "vault") and all config/template
// processing can use ExpandVars to resolve placeholders uniformly.
package variables

import (
	"regexp"
	"strings"
	"sync"
)

// pattern matches ${{domain:key}} placeholders.
// Double braces distinguish config-level variables from shell-style ${key}.
var pattern = regexp.MustCompile(`\$\{\{([^}]+)\}\}`)

// resolvers holds registered domain resolvers.
var (
	mu        sync.RWMutex
	resolvers = map[string]Resolver{}
)

// Resolver resolves a key within a domain.
// Returns ("", false) if the key is not found; the placeholder is kept as-is.
type Resolver func(key string) (string, bool)

// RegisterResolver registers a resolver for a ${{domain:key}} domain.
// It is safe to call concurrently. Panics if the domain is already registered.
func RegisterResolver(domain string, resolver Resolver) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := resolvers[domain]; dup {
		panic("variables: resolver already registered for domain " + domain)
	}
	resolvers[domain] = resolver
}

// HasResolver returns true if a resolver is registered for the given domain.
func HasResolver(domain string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := resolvers[domain]
	return ok
}

// ExpandVars replaces all ${{domain:key}} placeholders in s using registered resolvers.
// Unresolved variables (unknown domain or key not found) are left as-is.
func ExpandVars(s string) string {
	return pattern.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[3 : len(match)-2] // strip ${{ and }}
		return expand(inner, match)
	})
}

// ExpandVarsBytes is a convenience wrapper that operates on []byte.
func ExpandVarsBytes(data []byte) []byte {
	return pattern.ReplaceAllFunc(data, func(match []byte) []byte {
		inner := string(match[3 : len(match)-2])
		result := expand(inner, string(match))
		return []byte(result)
	})
}

func expand(expr, original string) string {
	domain, key, found := strings.Cut(expr, ":")
	if !found {
		return original // no domain prefix — leave as-is
	}

	mu.RLock()
	resolver, ok := resolvers[domain]
	mu.RUnlock()

	if ok {
		if val, resolved := resolver(key); resolved {
			return val
		}
	}

	return original // unresolved
}

// Keys is the keys of domain that the ${{domain:key}} placeholders of data
// name, each once, in the order they first appear: what a text reads through
// that domain (the keys a config file reads with ${{env:KEY}}), resolved or not.
func Keys(data []byte, domain string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range pattern.FindAllSubmatch(data, -1) {
		d, key, found := strings.Cut(string(m[1]), ":")
		if !found || d != domain || key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
