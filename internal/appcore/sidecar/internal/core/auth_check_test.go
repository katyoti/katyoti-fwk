// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

func TestAuthCheckItem(t *testing.T) {
	a := publish.AuthDef{Name: "example", Label: "Example login", LoginHint: "ktw-example auth login"}

	// Unknown (no Check) — reported as a warning that points at the login hint.
	it := authCheckItem(a, false, false)
	if it.Category != "auth" || it.Name != "example" {
		t.Fatalf("identity: %+v", it)
	}
	if it.Status != "warning" || it.Severity != "warn" || it.Hint == "" {
		t.Fatalf("unknown: %+v", it)
	}

	// Known + not authenticated — missing, with the login hint.
	it = authCheckItem(a, false, true)
	if it.Status != "missing" || it.Severity != "warn" || it.Hint == "" {
		t.Fatalf("missing: %+v", it)
	}

	// Known + authenticated — ok, no hint needed.
	it = authCheckItem(a, true, true)
	if it.Status != "ok" || it.Severity != "info" {
		t.Fatalf("ok: %+v", it)
	}
}
