// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"strings"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// UC-058 URL/SSRF half — these tests are the L1 closure for the fetch-tools
// allowlist + always-on private-network deny.

// TestURLAllowed_BareWildcardAllowsAnyPublic — entry `*` is the bypass.
// Public hosts pass; private-net still denied (the deny is always-on).
func TestURLAllowed_BareWildcardAllowsAnyPublic(t *testing.T) {
	ok, _ := urlAllowed("https://api.example.com/v1/foo", []string{"*"})
	if !ok {
		t.Error("bare * should allow public host")
	}
	// Private-net is denied even when `*` is in the allowlist.
	if ok, _ := urlAllowed("http://127.0.0.1/", []string{"*"}); ok {
		t.Error("bare * must NOT override private-net deny (loopback)")
	}
	if ok, _ := urlAllowed("http://169.254.169.254/latest/meta-data/", []string{"*"}); ok {
		t.Error("bare * must NOT override private-net deny (cloud metadata)")
	}
}

// TestURLAllowed_ExactHostMatch — entry `api.example.com` matches that exact
// host on any scheme; other hosts deny.
func TestURLAllowed_ExactHostMatch(t *testing.T) {
	allow := []string{"api.example.com"}
	cases := []struct {
		url     string
		wantOk  bool
		comment string
	}{
		{"https://api.example.com/v1", true, "exact host + https"},
		{"http://api.example.com/v1", true, "exact host + http (any scheme)"},
		{"https://API.Example.COM/x", true, "case-insensitive host match"},
		{"https://www.example.com/", false, "different subdomain rejected"},
		{"https://attacker.example/?u=api.example.com", false, "evil host, victim is in path/query"},
	}
	for _, c := range cases {
		ok, reason := urlAllowed(c.url, allow)
		if ok != c.wantOk {
			t.Errorf("%s: %s → ok=%v reason=%q (want ok=%v)", c.comment, c.url, ok, reason, c.wantOk)
		}
	}
}

// TestURLAllowed_WildcardSubdomain — `*.shop.example` matches subdomains but
// NOT shop.example itself (per OWASP). Foiled-attack corpus inline.
func TestURLAllowed_WildcardSubdomain(t *testing.T) {
	allow := []string{"*.shop.example"}
	cases := []struct {
		url    string
		wantOk bool
		note   string
	}{
		{"https://api.shop.example/", true, "direct subdomain"},
		{"https://a.b.c.shop.example/", true, "deep subdomain"},
		{"https://shop.example/", false, "naked apex NOT covered by *.shop.example"},
		{"https://attackershop.example/", false, "suffix lookalike rejected"},
		{"https://shop.example.attacker.example/", false, "victim-in-domain-tail rejected"},
	}
	for _, c := range cases {
		ok, reason := urlAllowed(c.url, allow)
		if ok != c.wantOk {
			t.Errorf("%s (%s): ok=%v reason=%q (want %v)", c.note, c.url, ok, reason, c.wantOk)
		}
	}
}

// TestURLAllowed_SchemeConstraint — `https://api.example.com` allows only that
// exact scheme+host pair. http to same host is denied.
func TestURLAllowed_SchemeConstraint(t *testing.T) {
	allow := []string{"https://api.example.com"}
	if ok, _ := urlAllowed("https://api.example.com/", allow); !ok {
		t.Error("scheme-qualified entry should match https")
	}
	if ok, _ := urlAllowed("http://api.example.com/", allow); ok {
		t.Error("scheme-qualified entry should NOT match plain http")
	}
}

// TestURLAllowed_EmptyAllowlistDenies — empty list = "no fetch under this
// spawn"; even a public URL fails.
func TestURLAllowed_EmptyAllowlistDenies(t *testing.T) {
	ok, reason := urlAllowed("https://api.example.com/", nil)
	if ok {
		t.Error("empty allowlist should deny")
	}
	if !strings.Contains(reason, "no urlAllowlist entries") {
		t.Errorf("reason %q should cite the gating reason", reason)
	}
}

// TestURLAllowed_PrivateNetCorpus — every entry in the OWASP cheatsheet
// deny-list rejects even with a wildcard allowlist. Critical — this is
// the SSRF protection.
func TestURLAllowed_PrivateNetCorpus(t *testing.T) {
	corpus := []string{
		"http://127.0.0.1/",
		"http://127.1.2.3/", // anywhere in 127/8
		"http://localhost/",
		"http://[::1]/",    // IPv6 loopback
		"http://10.0.0.1/", // RFC 1918
		"http://10.255.255.255/",
		"http://172.16.0.1/", // RFC 1918 (172.16/12)
		"http://172.31.255.255/",
		"http://192.168.1.1/",                      // RFC 1918
		"http://169.254.169.254/latest/meta-data/", // AWS/GCP metadata
		"http://0.0.0.0/",
		"http://[fc00::1]/", // IPv6 unique-local
		"http://[fe80::1]/", // IPv6 link-local
	}
	allow := []string{"*"} // most permissive — should still fail
	for _, u := range corpus {
		ok, reason := urlAllowed(u, allow)
		if ok {
			t.Errorf("UC-058 SSRF: %s should be denied even with allowlist=[*]", u)
		}
		if !strings.Contains(reason, "private-network") {
			t.Errorf("%s denied with reason %q — should cite private-network", u, reason)
		}
	}
}

// TestURLAllowed_MalformedURL — non-URL strings are denied with a clear
// parse-failure reason.
func TestURLAllowed_MalformedURL(t *testing.T) {
	cases := []string{
		"not-a-url",
		"://no-scheme",
		"http:",
	}
	for _, u := range cases {
		ok, _ := urlAllowed(u, []string{"*"})
		if ok {
			t.Errorf("malformed %q should be denied", u)
		}
	}

	// Empty URL is treated as "let the tool handle missing input" — passes.
	if ok, _ := urlAllowed("", []string{"*"}); !ok {
		t.Error("empty URL should NOT be denied at the hook (tool handles missing input)")
	}
}

// TestFetchToolDecision_NotAFetchTool — non-fetch tools are not gated.
func TestFetchToolDecision_NotAFetchTool(t *testing.T) {
	paths := &agentapi.WorkspacePaths{
		ReadPaths: []string{"src/"}, // restrictive (so hook is active)
	}
	if d := fetchToolDecision("file-read", map[string]any{"url": "http://x.test/"}, paths); d != nil {
		t.Errorf("file_read should pass through; got decision %+v", d)
	}
}

// TestFetchToolDecision_RestApi_PrivateNet — rest_api with a loopback URL is
// denied even when the allowlist is `*`. Verifies the wire-up at the hook
// entry point.
func TestFetchToolDecision_RestApi_PrivateNet(t *testing.T) {
	paths := &agentapi.WorkspacePaths{
		ReadPaths:    []string{"src/"},
		URLAllowlist: []string{"*"},
	}
	d := fetchToolDecision("rest-api-fetch", map[string]any{"url": "http://127.0.0.1/admin"}, paths)
	if d == nil {
		t.Fatal("expected deny for rest_api → loopback")
	}
	if d.Decision != agentapi.Deny {
		t.Errorf("decision = %v, want Deny", d.Decision)
	}
	if !strings.Contains(d.Reason, "private-network") {
		t.Errorf("reason %q should cite private-network", d.Reason)
	}
}

// TestFetchToolDecision_WebFetch_Allowlisted — web_fetch with an allowlisted
// public host passes.
func TestFetchToolDecision_WebFetch_Allowlisted(t *testing.T) {
	paths := &agentapi.WorkspacePaths{
		ReadPaths:    []string{"src/"},
		URLAllowlist: []string{"api.example.com"},
	}
	if d := fetchToolDecision("web-fetch", map[string]any{"url": "https://api.example.com/v1"}, paths); d != nil {
		t.Errorf("allowlisted URL should pass; got %+v", d)
	}
}

// TestFetchToolDecision_UnrestrictedSpawn — when the spawn doesn't set
// ReadPaths/WritePaths/URLAllowlist at all, the hook is a no-op and fetch
// tools pass through (matches the bash semantics).
func TestFetchToolDecision_UnrestrictedSpawn(t *testing.T) {
	paths := &agentapi.WorkspacePaths{} // all empty
	if d := fetchToolDecision("rest-api-fetch", map[string]any{"url": "http://127.0.0.1/"}, paths); d != nil {
		t.Errorf("unrestricted spawn shouldn't gate fetch; got %+v", d)
	}
}

// TestHook_FetchToolGated_EndToEnd — full path through WorkspacePathHook:
// build the hook, call OnPreToolExec with a fetch tool + private-net URL,
// expect a deny.
func TestHook_FetchToolGated_EndToEnd(t *testing.T) {
	hook := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		ReadPaths:    []string{"src/"},
		URLAllowlist: []string{"api.example.com"},
	})
	if hook == nil {
		t.Fatal("hook should not be nil for restrictive spawn with URL allowlist")
	}

	cases := []struct {
		name   string
		tool   string
		input  map[string]any
		denied bool
	}{
		{"rest_api allowlisted", "rest-api-fetch", map[string]any{"url": "https://api.example.com/v1"}, false},
		{"rest_api off-list", "rest-api-fetch", map[string]any{"url": "https://evil.invalid/"}, true},
		{"web_fetch loopback", "web-fetch", map[string]any{"url": "http://127.0.0.1/admin"}, true},
		{"web_fetch metadata endpoint", "web-fetch", map[string]any{"url": "http://169.254.169.254/"}, true},
		{"rest_api uses endpoint key", "rest-api-fetch", map[string]any{"endpoint": "https://api.example.com/"}, false},
	}
	for _, c := range cases {
		decision, err := hook.OnPreToolExec(nil, c.tool, c.input)
		if err != nil {
			t.Errorf("%s: unexpected err %v", c.name, err)
			continue
		}
		gotDenied := decision != nil && decision.Decision == agentapi.Deny
		if gotDenied != c.denied {
			t.Errorf("%s: denied=%v want %v (decision=%+v)", c.name, gotDenied, c.denied, decision)
		}
	}
}

// TestHook_URLAllowlistOnlyMakesHookActive — when ONLY URLAllowlist is set
// (paths empty), the hook must still allocate to gate fetch tools.
// Regression guard for the NewWorkspacePathHook's no-op shortcut.
func TestHook_URLAllowlistOnlyMakesHookActive(t *testing.T) {
	hook := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		URLAllowlist: []string{"api.example.com"},
	})
	if hook == nil {
		t.Fatal("URLAllowlist alone should activate the hook (gates fetch tools)")
	}
}
