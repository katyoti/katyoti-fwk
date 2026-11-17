// SPDX-License-Identifier: Apache-2.0

package worktree

import (
	"regexp"
	"strings"
	"testing"
)

// TestDeriveSlug_FormatMatchesValidator — the slug shape produced by
// deriveSlug must satisfy slugPattern. Regression guard so a future tweak to
// the format string and the regex don't drift apart.
func TestDeriveSlug_FormatMatchesValidator(t *testing.T) {
	for _, kind := range []string{KindIsolated, KindFork, "iso", "fork"} {
		slug := deriveSlug(kind, "any-agent-id")
		if !slugPattern.MatchString(slug) {
			t.Errorf("derived slug %q (kind=%q) does not match validator regex", slug, kind)
		}
	}
}

// TestDeriveSlug_NeverContainsCallerInput — no LLM-supplied text reaches the
// slug. We pass a hostile-looking displayName/agentID to deriveSlug and
// assert none of those substrings appears in the result. This is the R8
// defence-in-depth guarantee.
func TestDeriveSlug_NeverContainsCallerInput(t *testing.T) {
	hostile := []string{
		";rm-rf-slash",
		"$(whoami)",
		"--upload-pack=evil",
		"../../etc",
		"`backtick`",
		"name with spaces and \x00 nul",
	}
	for _, agent := range hostile {
		slug := deriveSlug(KindIsolated, agent)
		for _, frag := range []string{";", "$", "(", "`", "..", " ", "\x00", "--"} {
			if strings.Contains(slug, frag) {
				t.Errorf("slug %q derived from agentID %q contains forbidden fragment %q",
					slug, agent, frag)
			}
		}
	}
}

// TestValidateSlug_RejectsLLMShapedInput — the validator refuses anything
// that isn't the hash-derived format. If a future caller bypasses
// CreateWorktree and constructs a slug manually with LLM input, the
// validator catches it.
func TestValidateSlug_RejectsLLMShapedInput(t *testing.T) {
	m := NewManager(t.TempDir(), Config{BaseDir: t.TempDir()})

	rejected := []string{
		"some-task-name",              // pre-R8 free-form name
		"wt-iso-WRONGCASE-1",          // hash must be lowercase hex
		"wt-iso-1234567g-1",           // hash has non-hex char
		"wt-iso-12345678",             // missing seq
		"wt-iso-12345678-",            // empty seq
		"wt-iso-12345678-1; rm -rf /", // injection attempt
		"wt-iso-12345678-1/../escape", // traversal attempt
		"wt-toolongkind-12345678-1",   // kind > 8 chars
		"wt-x-12345678-1",             // kind too short (< 2 chars)
		"",                            // empty
	}
	for _, slug := range rejected {
		if err := m.ValidateSlug(slug); err == nil {
			t.Errorf("ValidateSlug(%q): expected error, got nil", slug)
		}
	}

	// Confirm a real derived slug passes.
	good := deriveSlug(KindIsolated, "agent")
	if err := m.ValidateSlug(good); err != nil {
		t.Errorf("ValidateSlug(%q): valid derived slug rejected: %v", good, err)
	}
}

// TestDeriveSlug_Uniqueness — back-to-back calls in the same nanosecond yield
// different slugs (the monotonic counter disambiguates).
func TestDeriveSlug_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 200)
	for i := 0; i < 200; i++ {
		s := deriveSlug(KindIsolated, "agent")
		if seen[s] {
			t.Fatalf("duplicate slug after %d iterations: %q", i, s)
		}
		seen[s] = true
	}
}

// TestCreateWorktree_RequiresKind — empty kind is rejected at the API.
func TestCreateWorktree_RequiresKind(t *testing.T) {
	m := NewManager(t.TempDir(), Config{BaseDir: t.TempDir()})
	_, err := m.CreateWorktree("", "displayName", "agent")
	if err == nil {
		t.Fatal("CreateWorktree with empty kind should be rejected")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("error %q should mention 'kind'", err.Error())
	}
}

// Sanity: confirm slugPattern compiles to what the package-level var expects.
// Catches a future copy-paste-then-tweak regression.
func TestSlugPattern_Wellformed(t *testing.T) {
	got := slugPattern.String()
	want := `^wt-[a-z]{2,8}-[a-f0-9]{8}-\d+$`
	if got != want {
		t.Errorf("slugPattern = %q, want %q", got, want)
	}
	// Also exercise via a known good slug.
	if !regexp.MustCompile(want).MatchString("wt-iso-abcdef01-42") {
		t.Error("hand-spun slug should match the canonical regex")
	}
}
