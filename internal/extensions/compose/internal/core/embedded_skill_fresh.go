// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

// embedded-skill-fresh is compose's latest-sync check: a sidecar embeds its
// skills at BUILD time, so a RUNNING sidecar can serve an OLD body versus an
// edited source file. Testing a stale embedded skill scores the wrong version,
// so this linter diffs the body the catalog actually SERVES against the source
// file and fails (error-severity) when they diverge — the deterministic form of
// "rebuild the sidecar before testing". Compose owns it because "skill" is a
// compose concept; the comparison itself is pure + unit-testable.

// CompareEmbeddedFresh returns pass when the served (embedded) skill body matches
// the source file body, else a stale-embedded-skill error naming the provider to
// rebuild. Only the body (post-frontmatter) is compared — that is the method the
// test exercises; line endings + trailing whitespace are normalized so a pure
// CRLF/whitespace difference is never a false stale.
func CompareEmbeddedFresh(provider, served, file string) verify.LintResult {
	sb, fb := normalizeBody(bodyOf(served)), normalizeBody(bodyOf(file))
	if sb == fb {
		return verify.Result()
	}
	msg := fmt.Sprintf(
		"the running %q serves a skill body that differs from the source (served %d vs file %d chars, normalized) — rebuild %s so the embedded skill matches the latest source before testing it",
		provider, len(sb), len(fb), provider)
	return verify.Result(verify.LintFinding{Rule: "stale-embedded-skill", Message: msg, Severity: verify.SeverityError})
}

// bodyOf returns the content after the YAML frontmatter, or the whole content
// when there is no frontmatter (so a body-only served form still compares).
func bodyOf(content string) string {
	if _, body, ok := verify.SplitFrontmatter(content); ok {
		return body
	}
	return content
}

// normalizeBody makes the comparison robust to line-ending + trailing-whitespace
// noise: CRLF→LF, strip per-line trailing whitespace, trim leading/trailing blank lines.
func normalizeBody(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// providerOf extracts the provider segment of a resource URI
// (`ktw-example:/skill/foo` → `ktw-example`).
func providerOf(uri string) string {
	if i := strings.IndexByte(uri, ':'); i > 0 {
		return uri[:i]
	}
	return uri
}
