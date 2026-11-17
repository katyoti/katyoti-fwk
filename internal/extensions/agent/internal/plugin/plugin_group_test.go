// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

func refs(specs ...string) []api.PluginRef {
	out := make([]api.PluginRef, 0, len(specs))
	for _, s := range specs {
		out = append(out, api.ParsePluginRef(s))
	}
	return out
}

func names(rs []api.PluginRef) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, r.Ref())
	}
	return strings.Join(parts, " ")
}

// ExpandGroups had no test of its own until 2026-09-22 — the shorthand form,
// the explicit `group:` form, nested groups, dedup, the unknown-group error
// and the cycle error were all exercised only by running a task.
func TestExpandGroups_ShorthandExplicitNestedDedup(t *testing.T) {
	groups := GroupRegistry{
		"standard":    refs("workspace", "web"),
		"web-enabled": refs("standard", "browser"),
	}
	got, err := ExpandGroups(refs("web-enabled", "group:standard", "bash", "workspace"), groups)
	if err != nil {
		t.Fatalf("ExpandGroups: %v", err)
	}
	// Nested expansion in order, every leaf once, a leaf named twice once.
	if names(got) != "workspace web browser bash" {
		t.Fatalf("expanded to %q, want %q", names(got), "workspace web browser bash")
	}
}

func TestExpandGroups_UnknownGroupIsAnError(t *testing.T) {
	_, err := ExpandGroups(refs("group:standrd"), GroupRegistry{"standard": refs("workspace")})
	if err == nil || !strings.Contains(err.Error(), `unknown group "standrd"`) {
		t.Fatalf("a misspelled explicit group must fail loudly, got %v", err)
	}
	// The shorthand form of an unknown name is a leaf type: the resolver's
	// job to refuse, not the expander's.
	got, err := ExpandGroups(refs("standrd"), GroupRegistry{"standard": refs("workspace")})
	if err != nil || names(got) != "standrd" {
		t.Fatalf("shorthand of an unknown name passes through as a leaf, got %q %v", names(got), err)
	}
}

func TestExpandGroups_CycleIsAnError(t *testing.T) {
	groups := GroupRegistry{"a": refs("b", "workspace"), "b": refs("a")}
	_, err := ExpandGroups(refs("a"), groups)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("a group cycle must be an error naming the cycle, got %v", err)
	}
}
