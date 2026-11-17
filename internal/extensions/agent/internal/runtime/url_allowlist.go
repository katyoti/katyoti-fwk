// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Fetch tools are detected via toolcap.HasCapability(.., CapIssuesNetworkRequests).
// Registry in toolcap.go is the single source of truth (R3). Adding a new
// network-issuing tool: register its CapIssuesNetworkRequests in toolcap.go
// init() and it flows automatically into this hook + every other gate.

// urlInputKeys are the input-map keys we probe for a URL argument. Order
// matters: most tools use `url`; some use `endpoint` or `uri`.
var urlInputKeys = []string{"url", "endpoint", "uri"}

// privateCIDRs is the static deny-list per the OWASP SSRF cheatsheet. The
// always-on check rejects any URL whose resolved host falls in one of these
// ranges, regardless of the per-spawn allowlist.
//
// Intentionally hard-coded: this list is a security floor, not a tunable.
// Sites that legitimately need to reach localhost (e.g. dev tooling) widen
// `WorkspacePaths` at spawn time AND audit the override.
var privateCIDRs = []string{
	"127.0.0.0/8",    // loopback
	"::1/128",        // IPv6 loopback
	"10.0.0.0/8",     // RFC 1918
	"172.16.0.0/12",  // RFC 1918
	"192.168.0.0/16", // RFC 1918
	"169.254.0.0/16", // link-local (AWS/GCP/Azure metadata endpoint)
	"fc00::/7",       // IPv6 unique local
	"fe80::/10",      // IPv6 link-local
	"0.0.0.0/8",      // "this" network
}

// urlAllowed returns (true, "") when the URL is permitted by the spawn's
// allowlist AND not in a private-net range. Otherwise returns (false, reason)
// with a human-readable explanation for the deny.
//
// `allowlist == nil` (or empty) means "no fetch allowed under this spawn".
// `["*"]` means "any public host".
func urlAllowed(rawURL string, allowlist []string) (bool, string) {
	if rawURL == "" {
		// No URL parsed — let the tool surface its own missing-input error.
		return true, ""
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Sprintf("URL %q failed to parse: %v", rawURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return false, fmt.Sprintf("URL %q is missing scheme or host", rawURL)
	}

	host := u.Hostname()

	// Private-net deny — ALWAYS on. This rule fires before allowlist eval
	// so an allowlist entry of `*` can't accidentally widen access to
	// loopback / metadata endpoints.
	if isPrivateNetHost(host) {
		return false, fmt.Sprintf(
			"URL %q targets a private-network host (%s) — denied by always-on "+
				"SSRF protection. Public hosts only.",
			rawURL, host,
		)
	}

	// Empty allowlist means "no fetch under this spawn".
	if len(allowlist) == 0 {
		return false, fmt.Sprintf(
			"URL %q is denied: this spawn has restrictive WorkspacePaths and "+
				"no urlAllowlist entries — fetch tools are gated. Widen by "+
				"adding the host (or `*`) to urlAllowlist.",
			rawURL,
		)
	}

	// Allowlist match — bare `*` is the bypass; otherwise compare entries
	// against host (and optionally scheme).
	for _, entry := range allowlist {
		if entry == "*" {
			return true, ""
		}
		if entryMatches(entry, u.Scheme, host) {
			return true, ""
		}
	}

	return false, fmt.Sprintf(
		"URL %q (host %s) is not in this spawn's urlAllowlist. Allowed: %v",
		rawURL, host, allowlist,
	)
}

// entryMatches returns true when an allowlist entry permits the given
// scheme+host pair. Entry forms:
//   - `api.example.com` — exact host match, any scheme
//   - `*.example.com`   — wildcard subdomain (NOT example.com itself)
//   - `https://api.example.com` — scheme + host both must match
func entryMatches(entry, scheme, host string) bool {
	// Scheme-qualified form.
	if strings.Contains(entry, "://") {
		eu, err := url.Parse(entry)
		if err != nil || eu.Scheme == "" || eu.Host == "" {
			return false
		}
		return strings.EqualFold(eu.Scheme, scheme) && strings.EqualFold(eu.Host, host)
	}

	// Wildcard subdomain form.
	if strings.HasPrefix(entry, "*.") {
		suffix := entry[1:] // ".example.com"
		// Match `foo.example.com` and `a.b.example.com` but not `example.com`
		// itself. Use lower-case compare for case-insensitive host equality.
		hl := strings.ToLower(host)
		sl := strings.ToLower(suffix)
		return strings.HasSuffix(hl, sl) && hl != sl[1:]
	}

	// Exact host form (any scheme).
	return strings.EqualFold(entry, host)
}

// isPrivateNetHost returns true when host (a hostname or IP literal) resolves
// to or is itself a private/loopback/link-local address. Hostnames that
// don't resolve to an IP literal at parse-time return false here; the runtime
// HTTP client may still be tricked by DNS rebinding — defenders rely on
// the allowlist as the primary control, with this check catching naive
// `http://127.0.0.1/` attempts.
func isPrivateNetHost(host string) bool {
	if host == "" {
		return false
	}
	// Common loopback aliases that don't always resolve as 127.0.0.1.
	switch strings.ToLower(host) {
	case "localhost", "ip6-localhost", "ip6-loopback":
		return true
	}
	// IPv6 addresses contain `:` separators and would be misread as
	// "host:port"; try parsing as-is first to catch them, then fall back to
	// IPv4 host:port stripping.
	ip := net.ParseIP(host)
	if ip == nil {
		// Try stripping an IPv4 port (single trailing `:port`).
		if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[:i], ":") {
			ip = net.ParseIP(host[:i])
		}
	}
	if ip == nil {
		// Hostname — defer to allowlist control (no DNS lookup in the hook
		// path to avoid blocking on network).
		return false
	}
	for _, cidr := range privateCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// FetchToolDecision applies the URL allowlist + private-net deny to a fetch
// tool input. Returns nil decision when the tool is not a fetch tool or the
// spawn doesn't restrict workspaces (caller-side check already filtered).
func fetchToolDecision(toolName string, input map[string]any, paths *agentapi.WorkspacePaths) *agentapi.ToolDecision {
	if !HasCapability(toolName, CapIssuesNetworkRequests) {
		return nil
	}
	if paths == nil {
		return nil
	}
	// Fetch gate only fires when the spawn is restrictive (path lists set).
	// An unrestricted spawn (`ReadPaths: []` and `WritePaths: []`) doesn't
	// gate fetch either — matches the bash deny-by-default semantics where
	// fetch is locked-down ONLY when the spawn is locked-down.
	if isUnrestrictedPaths(paths.ReadPaths) && isUnrestrictedPaths(paths.WritePaths) {
		return nil
	}

	rawURL := extractURL(input)
	ok, reason := urlAllowed(rawURL, paths.URLAllowlist)
	if ok {
		return nil
	}
	emitAuditDeny(AuditEvent{Hook: "url_allowlist", Tool: toolName, Reason: reason, DeniedValue: rawURL})
	return &agentapi.ToolDecision{
		Decision: agentapi.Deny,
		Reason:   reason,
	}
}

// extractURL probes the input map for a URL argument. Honors a small,
// stable set of conventional keys (url, endpoint, uri) — fetch tools that
// use other keys can be added here.
func extractURL(input map[string]any) string {
	for _, k := range urlInputKeys {
		if v, ok := input[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
