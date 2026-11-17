// SPDX-License-Identifier: Apache-2.0

// Package scan reads a document tree before it leaves for its publisher
// : an external URL, hidden text or another tenant's term in
// a document is a finding, and a tree with a finding is held for a person
// instead of being written. It is pure: the rules come from the caller.
package scan

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

// Rules are what the scan allows and what it looks for.
type Rules struct {
	// AllowedHosts are the hosts a link or an image may point to (a host, or
	// a domain a host ends with).
	AllowedHosts []string
	// Tenant is the publishing tenant: its own terms are never findings.
	Tenant string
	// TenantTerms are the terms of each tenant - a client's name, a project
	// code - that must never appear in another tenant's publication.
	TenantTerms map[string][]string
}

// linkRE is a markdown link or image target, and an HTML href or src.
var linkRE = regexp.MustCompile(`(?i)\]\(\s*<?([a-z][a-z0-9+.-]*://[^)\s>]+)|(?:href|src)\s*=\s*["']([a-z][a-z0-9+.-]*://[^"']+)["']|<([a-z][a-z0-9+.-]*://[^>\s]+)>`)

// bareURLRE is a URL written in the text.
var bareURLRE = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s<>()"'\]]+`)

// hiddenREs are markup that hides text from a reader.
var hiddenREs = []struct {
	re   *regexp.Regexp
	what string
}{
	{regexp.MustCompile(`(?s)<!--.*?-->`), "an HTML comment"},
	{regexp.MustCompile(`(?i)style\s*=\s*["'][^"']*(display\s*:\s*none|visibility\s*:\s*hidden|font-size\s*:\s*0|opacity\s*:\s*0(?:\.0+)?\s*(?:;|["'])|color\s*:\s*(?:#fff(?:fff)?|white|transparent)\b)`), "text styled to be hidden"},
	{regexp.MustCompile(`(?i)<[a-z][^>]*\shidden(?:\s|=|>|/)`), "an element marked hidden"},
}

// Scan is every finding of the tree, node by node, in order: each node's
// title (a group's too: it is a page of its own), a document's markdown and
// the names of its assets.
func Scan(tree api.DocumentTree, rules Rules) []api.ScanFinding {
	var out []api.ScanFinding
	terms := otherTenantTerms(rules)
	var walk func(nodes []api.Node)
	walk = func(nodes []api.Node) {
		for _, n := range nodes {
			texts := []string{n.Title}
			if n.Document != nil {
				texts = append(texts, n.Document.Markdown)
				for _, a := range n.Document.Assets {
					texts = append(texts, a.Name)
				}
			}
			out = append(out, scanNode(n.Key, texts, rules, terms)...)
			walk(n.Children)
		}
	}
	walk(tree.Nodes)
	return out
}

// scanNode is every finding of one node's texts: its title, its markdown,
// its assets' names.
func scanNode(key string, texts []string, rules Rules, terms []tenantTerm) []api.ScanFinding {
	var out []api.ScanFinding
	add := func(kind, excerpt, why string) {
		out = append(out, api.ScanFinding{Key: key, Kind: kind, Excerpt: clip(excerpt), Why: why})
	}
	seen := map[string]bool{}
	external := func(raw string) {
		raw = strings.TrimRight(raw, ".,;:")
		if seen[raw] {
			return
		}
		seen[raw] = true
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			add(api.ScanExternalURL, raw, "an address that cannot be read")
			return
		}
		if !allowed(u.Hostname(), rules.AllowedHosts) {
			add(api.ScanExternalURL, raw, fmt.Sprintf("the host %s is not an allowed host", u.Hostname()))
		}
	}
	found := map[int]bool{}
	for _, md := range texts {
		if md == "" {
			continue
		}
		for _, m := range linkRE.FindAllStringSubmatch(md, -1) {
			for _, g := range m[1:] {
				if g != "" {
					external(g)
				}
			}
		}
		for _, m := range bareURLRE.FindAllString(md, -1) {
			external(m)
		}
		for _, h := range hiddenREs {
			for _, m := range h.re.FindAllString(md, -1) {
				add(api.ScanHiddenText, m, h.what)
			}
		}
		if r, at := invisibleRune(md); r != 0 {
			add(api.ScanHiddenText, around(md, at), fmt.Sprintf("an invisible character U+%04X", r))
		}
		for i, t := range terms {
			if found[i] {
				continue
			}
			if loc := t.re.FindStringIndex(md); loc != nil {
				found[i] = true
				add(api.ScanTenantTerm, around(md, loc[0]), fmt.Sprintf("a term of the tenant %s", t.tenant))
			}
		}
	}
	return out
}

// allowed is whether host is one of the allowed hosts or under one of them.
func allowed(host string, hosts []string) bool {
	host = strings.ToLower(host)
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(h), "."))
		if h != "" && (host == h || strings.HasSuffix(host, "."+h)) {
			return true
		}
	}
	return false
}

// invisibleRune is the first character a reader cannot see: zero-width,
// a Unicode tag, a bidirectional control - and where it is.
func invisibleRune(s string) (rune, int) {
	for i, r := range s {
		switch {
		case r >= 0x200B && r <= 0x200F, r == 0x2060, r == 0xFEFF, r == 0x00AD,
			r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069,
			r >= 0xE0000 && r <= 0xE007F:
			return r, i
		}
	}
	return 0, -1
}

type tenantTerm struct {
	tenant string
	re     *regexp.Regexp
}

// otherTenantTerms are the terms of every tenant but the publishing one, each
// matched as a whole word, case ignored.
func otherTenantTerms(rules Rules) []tenantTerm {
	var tenants []string
	for t := range rules.TenantTerms {
		if t != rules.Tenant {
			tenants = append(tenants, t)
		}
	}
	sort.Strings(tenants)
	var out []tenantTerm
	for _, t := range tenants {
		for _, term := range rules.TenantTerms[t] {
			term = strings.TrimSpace(term)
			if term == "" {
				continue
			}
			out = append(out, tenantTerm{tenant: t, re: regexp.MustCompile(`(?i)(^|[^\pL\pN_])` + regexp.QuoteMeta(term) + `($|[^\pL\pN_])`)})
		}
	}
	return out
}

// around is a short excerpt of s around at.
func around(s string, at int) string {
	from, to := at-30, at+50
	if from < 0 {
		from = 0
	}
	if to > len(s) {
		to = len(s)
	}
	for from > 0 && !utf8Start(s[from]) {
		from--
	}
	for to < len(s) && !utf8Start(s[to]) {
		to++
	}
	return s[from:to]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// clip keeps an excerpt short and on one line.
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}
