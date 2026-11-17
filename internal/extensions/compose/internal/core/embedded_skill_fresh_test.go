// SPDX-License-Identifier: Apache-2.0

package core

import "testing"

const freshSource = `---
name: trend
description: Research trends — load when asked.
---

# Trend

Resolve the inputs, then call the **web:search/query** tool for each trend.
`

func TestCompareEmbeddedFresh_Match(t *testing.T) {
	// Same body, only line-ending + trailing-whitespace noise differs → fresh.
	served := "---\nname: trend\ndescription: x\n---\r\n\r\n# Trend\r\n\r\nResolve the inputs, then call the **web:search/query** tool for each trend.   \r\n"
	r := CompareEmbeddedFresh("ktw-acme", served, freshSource)
	if r.Status != "pass" {
		t.Errorf("identical bodies (modulo CRLF/whitespace) must be fresh, got %+v", r)
	}
}

func TestCompareEmbeddedFresh_Drift(t *testing.T) {
	// The served body lost a sentence the source has → stale.
	stale := "---\nname: trend\n---\n\n# Trend\n\nResolve the inputs.\n"
	r := CompareEmbeddedFresh("ktw-acme", stale, freshSource)
	if r.Status != "fail" {
		t.Fatalf("diverged body must fail, got %+v", r)
	}
	if r.Findings[0].Rule != "stale-embedded-skill" {
		t.Errorf("rule = %q, want stale-embedded-skill", r.Findings[0].Rule)
	}
}

func TestCompareEmbeddedFresh_FrontmatterOnlyDiffIgnored(t *testing.T) {
	// Different frontmatter, identical BODY → fresh (we compare the method body).
	served := "---\nname: trend\ndescription: TOTALLY DIFFERENT\nicon: x\n---\n\n# Trend\n\nResolve the inputs, then call the **web:search/query** tool for each trend.\n"
	if r := CompareEmbeddedFresh("ktw-acme", served, freshSource); r.Status != "pass" {
		t.Errorf("frontmatter-only difference must not flag stale, got %+v", r)
	}
}

func TestProviderOf(t *testing.T) {
	if got := providerOf("ktw-acme:/skill/dossier-trend-research"); got != "ktw-acme" {
		t.Errorf("providerOf = %q", got)
	}
	if got := providerOf("noscheme"); got != "noscheme" {
		t.Errorf("providerOf(noscheme) = %q", got)
	}
}
