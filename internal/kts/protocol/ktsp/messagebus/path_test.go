// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// TestParsePath_HappyPath covers every shortcut form documented in the
// grammar. ctx carries a self-provider via WithSelfProvider so the
// empty-provider shortcut resolves correctly without booting an Application.
func TestParsePath_HappyPath(t *testing.T) {
	ctx := application.WithSelfProvider(context.Background(), "ktw-agents")

	cases := []struct {
		in   string
		want Path
	}{
		// Fully qualified
		{
			"ktw-agents:Terminal(default)/StartTask",
			Path{Provider: "ktw-agents", Slottype: "terminal", Instance: "default", Method: "start-task"},
		},
		// Default instance via shortcut
		{
			"ktw-agents:Terminal/StartTask",
			Path{Provider: "ktw-agents", Slottype: "terminal", Instance: "default", Method: "start-task"},
		},
		// Self-provider via empty token before ':'
		{
			":Terminal/StartTask",
			Path{Provider: "ktw-agents", Slottype: "terminal", Instance: "default", Method: "start-task"},
		},
		// Cross-binary call
		{
			"ktw-hub:Catalog/ListResources",
			Path{Provider: "ktw-hub", Slottype: "catalog", Instance: "default", Method: "list-resources"},
		},
		// Named instance
		{
			":LLMProfiles(openai-prod)/Complete",
			Path{Provider: "ktw-agents", Slottype: "llm-profiles", Instance: "openai-prod", Method: "complete"},
		},
		// Already-kebab inputs
		{
			":resource-service/list",
			Path{Provider: "ktw-agents", Slottype: "resource-service", Instance: "default", Method: "list"},
		},
		// Provider with hyphens (already kebab)
		{
			"ktw-example:Discovery/ListAPIs",
			Path{Provider: "ktw-example", Slottype: "discovery", Instance: "default", Method: "list-apis"},
		},
		// A method of several segments keeps them, each kebabed
		{
			"ktw-methods:Tasks(default)/Dossier/RefineScope",
			Path{Provider: "ktw-methods", Slottype: "tasks", Instance: "default", Method: "dossier/refine-scope"},
		},
	}
	for _, c := range cases {
		got, err := ParsePath(ctx, c.in)
		if err != nil {
			t.Errorf("ParsePath(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParsePath(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

// TestParsePath_ErrorCases covers the documented error grammar.
func TestParsePath_ErrorCases(t *testing.T) {
	ctx := application.WithSelfProvider(context.Background(), "ktw-agents")

	cases := []struct {
		in      string
		wantSub string // substring of the expected error message
	}{
		{"Terminal/StartTask", "missing ':'"},                // no separator
		{"ktw-agents:Terminal", "missing '/Method' suffix"},  // no slash separating type from method
		{"ktw-agents:Terminal/", "empty method"},             // empty method
		{"ktw-agents:(default)/Start", "empty Type"},         // empty type before parens
		{"ktw-agents:Terminal()/Start", "empty instance"},    // empty parens
		{"ktw-agents:Terminal(foo/Start", "no matching ')'"}, // unbalanced parens
		{":Terminal_Bad/StartTask", "invalid char"},          // underscore not allowed in kebab
		{":Terminal/a//b", "is empty"},                       // an empty method segment
		// Legacy `:/` prefix is tolerated for the migration window — see
		// TestParsePath_LegacySlashPrefix below for the explicit coverage.
	}
	for _, c := range cases {
		_, err := ParsePath(ctx, c.in)
		if err == nil {
			t.Errorf("ParsePath(%q): expected error containing %q, got nil", c.in, c.wantSub)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("ParsePath(%q): error = %q, want substring %q", c.in, err.Error(), c.wantSub)
		}
	}
}

// TestParsePath_EmptyProviderWithoutCtx — when authoring path elides the
// provider AND no self-provider is bound to ctx, parse fails with a
// specific actionable message (rather than producing an empty token
// that would later panic in SlotSubject's validation).
func TestParsePath_EmptyProviderWithoutCtx(t *testing.T) {
	_, err := ParsePath(context.Background(), ":Terminal/StartTask")
	if err == nil {
		t.Fatalf("expected error for empty provider + no ctx, got nil")
	}
	if !strings.Contains(err.Error(), "self-provider") {
		t.Errorf("error = %q, want substring 'self-provider'", err.Error())
	}
}

// TestPath_Subject round-trips the parsed Path back into the canonical
// authoring form `<provider>:<slot>[(<instance>)]/<method>` (per
// the URI grammar). The `(default)` instance is omitted
// because it's the implicit value.
func TestPath_Subject(t *testing.T) {
	cases := []struct {
		path Path
		want string
	}{
		{
			Path{Provider: "ktw-agents", Slottype: "terminal", Instance: "default", Method: "start-task"},
			"ktw-agents:terminal/start-task",
		},
		{
			Path{Provider: "ktw-example", Slottype: "example-service", Instance: "prod", Method: "search"},
			"ktw-example:example-service(prod)/search",
		},
	}
	for _, c := range cases {
		if got := c.path.Subject(); got != c.want {
			t.Errorf("Path.Subject() for %+v = %q, want %q", c.path, got, c.want)
		}
	}
}

// TestParsePath_LegacySlashPrefix covers the migration-window backward
// compat: the pre-2026-05-23 `:/` prefix MUST still parse to the same
// Path as the canonical no-slash form, so existing binaries don't break
// mid-cutover.
func TestParsePath_LegacySlashPrefix(t *testing.T) {
	ctx := application.WithSelfProvider(context.Background(), "ktw-agents")
	cases := []struct {
		legacy    string
		canonical string
	}{
		{"ktw-agents:/Terminal/StartTask", "ktw-agents:Terminal/StartTask"},
		{":/Terminal/StartTask", ":Terminal/StartTask"},
		{"ktw-hub:/Catalog/ListResources", "ktw-hub:Catalog/ListResources"},
	}
	for _, c := range cases {
		legacyP, lerr := ParsePath(ctx, c.legacy)
		canonP, cerr := ParsePath(ctx, c.canonical)
		if lerr != nil || cerr != nil {
			t.Errorf("ParsePath errors: legacy(%q)=%v, canonical(%q)=%v", c.legacy, lerr, c.canonical, cerr)
			continue
		}
		if legacyP != canonP {
			t.Errorf("legacy %q parsed to %+v; canonical %q parsed to %+v (should match)", c.legacy, legacyP, c.canonical, canonP)
		}
	}
}
