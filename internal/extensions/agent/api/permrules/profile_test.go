// SPDX-License-Identifier: Apache-2.0

package permrules

import (
	"errors"
	"strings"
	"testing"
)

// ── Profile.AllowMatches / DenyMatches ──────────────────────────────────

func TestProfile_NilProfile_NoMatch(t *testing.T) {
	var p *Profile
	if ok, _ := p.AllowMatches("file-read", nil); ok {
		t.Error("nil profile should never allow")
	}
	if ok, _ := p.DenyMatches("file-read", nil); ok {
		t.Error("nil profile should never deny")
	}
}

func TestProfile_EmptyProfile_NoMatch(t *testing.T) {
	p := &Profile{Name: "empty"}
	if ok, _ := p.AllowMatches("file-read", nil); ok {
		t.Error("empty profile should never allow")
	}
	if ok, _ := p.DenyMatches("file-read", nil); ok {
		t.Error("empty profile should never deny")
	}
}

func TestProfile_AllowMatches_FirstMatchWins(t *testing.T) {
	p := &Profile{
		Name: "test",
		AllowRules: []Rule{
			MustParseRule("file-read.workspace_path:**"),
			MustParseRule("file-read.absolute_path:**"),
			MustParseRule("file-write.workspace_path:**"),
		},
	}
	ok, rule := p.AllowMatches("file-read", map[string]string{"workspace_path": "src/main.go"})
	if !ok {
		t.Fatal("expected allow")
	}
	if rule.Field != "workspace_path" {
		t.Errorf("first match should have been workspace_path, got %q", rule.Field)
	}
}

func TestProfile_DenyMatches(t *testing.T) {
	p := &Profile{
		Name: "plan-mode",
		DenyRules: []Rule{
			MustParseRule("file-write:**"),
			MustParseRule("file-edit:**"),
			MustParseRule("bash:**"),
		},
	}
	if ok, _ := p.DenyMatches("file-write", nil); !ok {
		t.Error("file-write should be denied")
	}
	if ok, _ := p.DenyMatches("file-read", nil); ok {
		t.Error("file-read should NOT be denied")
	}
	if ok, _ := p.DenyMatches("bash", map[string]string{"command": "ls"}); !ok {
		t.Error("bash should be denied")
	}
}

func TestProfile_AllowAndDeny_Independent(t *testing.T) {
	// A profile can have both lists. Allow doesn't override Deny —
	// they're checked at different points in the gate (deny first,
	// allow only on Ask). Independent here means the two lists
	// don't interact within the profile struct itself.
	p := &Profile{
		Name: "plan-mode",
		AllowRules: []Rule{
			MustParseRule("file-read:**"),
		},
		DenyRules: []Rule{
			MustParseRule("file-write:**"),
		},
	}
	if ok, _ := p.AllowMatches("file-read", nil); !ok {
		t.Error("file-read should match allow")
	}
	if ok, _ := p.DenyMatches("file-write", nil); !ok {
		t.Error("file-write should match deny")
	}
	// And no cross-contamination:
	if ok, _ := p.AllowMatches("file-write", nil); ok {
		t.Error("file-write should NOT match allow")
	}
	if ok, _ := p.DenyMatches("file-read", nil); ok {
		t.Error("file-read should NOT match deny")
	}
}

// ── AddSessionAllow / AddSessionDeny shadow base rules ─────────────────

func TestProfile_AddSessionAllow_InsertsAtFront(t *testing.T) {
	p := &Profile{
		Name: "base",
		AllowRules: []Rule{
			MustParseRule("file-read.workspace_path:**"), // base rule
		},
	}
	p.AddSessionAllow(MustParseRule("file-read.absolute_path:/home/user/**"))

	if len(p.AllowRules) != 2 {
		t.Fatalf("AllowRules len = %d, want 2", len(p.AllowRules))
	}
	if p.AllowRules[0].Field != "absolute_path" {
		t.Errorf("session rule should be at index 0, got %v", p.AllowRules[0])
	}
	if p.AllowRules[1].Field != "workspace_path" {
		t.Errorf("base rule should be at index 1, got %v", p.AllowRules[1])
	}
}

func TestProfile_AddSessionAllow_ShadowsBaseOnFirstMatch(t *testing.T) {
	// A session add for `*` should shadow ALL base rules — first
	// match wins.
	p := &Profile{
		Name: "base",
		AllowRules: []Rule{
			MustParseRule("file-read.workspace_path:**"),
		},
	}
	p.AddSessionAllow(MustParseRule("*"))

	// The wildcard should be the first match for any tool/dimensions.
	ok, rule := p.AllowMatches("bash", map[string]string{"command": "anything"})
	if !ok {
		t.Fatal("expected allow")
	}
	if rule.ToolName != "*" {
		t.Errorf("first matching rule should be *, got %+v", rule)
	}
}

func TestProfile_AddSessionDeny_InsertsAtFront(t *testing.T) {
	p := &Profile{
		Name: "base",
		DenyRules: []Rule{
			MustParseRule("file-write:**"),
		},
	}
	p.AddSessionDeny(MustParseRule("bash:**"))

	if len(p.DenyRules) != 2 {
		t.Fatalf("DenyRules len = %d, want 2", len(p.DenyRules))
	}
	if p.DenyRules[0].ToolName != "bash" {
		t.Errorf("session rule should be at index 0, got %+v", p.DenyRules[0])
	}
}

// ── Clone ───────────────────────────────────────────────────────────────

func TestProfile_Clone_DeepCopiesRules(t *testing.T) {
	p := &Profile{
		Name:        "base",
		Description: "test",
		AllowRules:  []Rule{MustParseRule("file-read:**")},
		DenyRules:   []Rule{MustParseRule("file-write:**")},
	}
	clone := p.Clone()

	// Mutating the clone must not affect the original.
	clone.AddSessionAllow(MustParseRule("bash:**"))
	if len(p.AllowRules) != 1 {
		t.Errorf("clone mutation leaked: original AllowRules len = %d", len(p.AllowRules))
	}
	if len(clone.AllowRules) != 2 {
		t.Errorf("clone AllowRules len = %d, want 2", len(clone.AllowRules))
	}
}

func TestProfile_Clone_NilSafe(t *testing.T) {
	var p *Profile
	if got := p.Clone(); got != nil {
		t.Errorf("Clone of nil = %v, want nil", got)
	}
}

// ── LoadProfile (YAML decoding) ─────────────────────────────────────────

func TestLoadProfile_HappyPath(t *testing.T) {
	yaml := []byte(`
description: "Test profile"
allow_rules:
  - "file-read.workspace_path:**"
  - "file-read.absolute_path:/home/user/**"
deny_rules:
  - "file-write:**"
`)
	p, err := LoadProfile("test", yaml)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if p.Name != "test" {
		t.Errorf("Name = %q, want %q", p.Name, "test")
	}
	if p.Description != "Test profile" {
		t.Errorf("Description = %q", p.Description)
	}
	if len(p.AllowRules) != 2 {
		t.Errorf("AllowRules len = %d, want 2", len(p.AllowRules))
	}
	if len(p.DenyRules) != 1 {
		t.Errorf("DenyRules len = %d, want 1", len(p.DenyRules))
	}
	if p.AllowRules[1].ValueGlob != "/home/user/**" {
		t.Errorf("AllowRules[1] glob = %q", p.AllowRules[1].ValueGlob)
	}
}

func TestLoadProfile_EmptyLists(t *testing.T) {
	yaml := []byte(`
description: "no rules"
allow_rules: []
deny_rules: []
`)
	p, err := LoadProfile("empty", yaml)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if len(p.AllowRules) != 0 || len(p.DenyRules) != 0 {
		t.Errorf("expected empty lists, got %d allow / %d deny", len(p.AllowRules), len(p.DenyRules))
	}
}

func TestLoadProfile_InvalidRuleErrors(t *testing.T) {
	yaml := []byte(`
description: "broken"
allow_rules:
  - "file-read.workspace_path:**"
  - "this is not a valid rule"
`)
	_, err := LoadProfile("broken", yaml)
	if err == nil {
		t.Fatal("expected error from malformed rule")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should mention profile name 'broken', got %q", err.Error())
	}
	if !errors.Is(err, errInvalidRule) {
		t.Errorf("error should wrap errInvalidRule, got %v", err)
	}
}

func TestLoadProfile_FullAccessProfile(t *testing.T) {
	yaml := []byte(`
description: "Full access — use with care"
allow_rules:
  - "*"
deny_rules: []
`)
	p, err := LoadProfile("full-access", yaml)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	// `*` should match anything.
	ok, _ := p.AllowMatches("any_tool", map[string]string{"any": "value"})
	if !ok {
		t.Error("full-access profile should allow anything")
	}
}
