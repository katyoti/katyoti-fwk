// SPDX-License-Identifier: Apache-2.0

package humangate

import (
	"bytes"
	"context"
	"strings"
	"testing"

	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
)

// newTestPrompter wires a CLIPrompter against an in-memory reader/writer
// pair so tests can drive the prompt and inspect the rendered output.
func newTestPrompter(input string) (*CLIPrompter, *bytes.Buffer) {
	out := &bytes.Buffer{}
	p := NewCLIPrompter(strings.NewReader(input), out)
	return p, out
}

// ── PromptPermission ────────────────────────────────────────────────────

func TestCLIPrompter_PromptPermission_DefaultIsDenyOnce(t *testing.T) {
	p, _ := newTestPrompter("\n") // empty line → enter
	resp, err := p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{
		ToolName: "file-read",
		Question: "test",
	})
	if err != nil {
		t.Fatalf("PromptPermission: %v", err)
	}
	if resp.Decision != hgapi.PermissionDecisionDenyOnce {
		t.Errorf("default decision = %v, want DenyOnce", resp.Decision)
	}
}

func TestCLIPrompter_PromptPermission_DenyOnce(t *testing.T) {
	p, _ := newTestPrompter("1\n")
	resp, err := p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{ToolName: "file-read", Question: "?"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Decision != hgapi.PermissionDecisionDenyOnce {
		t.Errorf("got %v, want DenyOnce", resp.Decision)
	}
}

func TestCLIPrompter_PromptPermission_AllowOnce(t *testing.T) {
	p, _ := newTestPrompter("2\n")
	resp, err := p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{ToolName: "file-read", Question: "?"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Decision != hgapi.PermissionDecisionAllowOnce {
		t.Errorf("got %v, want AllowOnce", resp.Decision)
	}
	if resp.SelectedRule != "" {
		t.Errorf("AllowOnce should not carry SelectedRule, got %q", resp.SelectedRule)
	}
}

func TestCLIPrompter_PromptPermission_AllowAlwaysWithSuggestion(t *testing.T) {
	p, _ := newTestPrompter("3\n")
	resp, err := p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{
		ToolName: "file-read",
		Question: "?",
		Suggestions: []hgapi.PermissionSuggestion{
			{Label: "Always allow under /home/user", Rule: "file_read.absolute_path:/home/user/**"},
			{Label: "Always allow everywhere", Rule: "file_read.absolute_path:**"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Decision != hgapi.PermissionDecisionAllowAlways {
		t.Errorf("got %v, want AllowAlways", resp.Decision)
	}
	if resp.SelectedRule != "file_read.absolute_path:/home/user/**" {
		t.Errorf("SelectedRule = %q, want first suggestion's rule", resp.SelectedRule)
	}
}

func TestCLIPrompter_PromptPermission_DenyAlways_SynthesizedRule(t *testing.T) {
	// 2 suggestions → slot 5 is "Never allow this tool".
	p, _ := newTestPrompter("5\n")
	resp, err := p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{
		ToolName: "file-read",
		Question: "?",
		Suggestions: []hgapi.PermissionSuggestion{
			{Label: "a", Rule: "file_read.absolute_path:/a"},
			{Label: "b", Rule: "file_read.absolute_path:/b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Decision != hgapi.PermissionDecisionDenyAlways {
		t.Errorf("got %v, want DenyAlways", resp.Decision)
	}
	if resp.SelectedRule != "file-read:**" {
		t.Errorf("synthesized DenyAlways rule = %q, want %q", resp.SelectedRule, "file-read:**")
	}
}

func TestCLIPrompter_PromptPermission_DenyAlways_NoSuggestions(t *testing.T) {
	// 0 suggestions → slot 3 is "Never allow this tool".
	p, _ := newTestPrompter("3\n")
	resp, err := p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{
		ToolName: "bash",
		Question: "?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Decision != hgapi.PermissionDecisionDenyAlways {
		t.Errorf("got %v, want DenyAlways", resp.Decision)
	}
	if resp.SelectedRule != "bash:**" {
		t.Errorf("got %q, want bash:**", resp.SelectedRule)
	}
}

func TestCLIPrompter_PromptPermission_GarbageInput_FailsClosed(t *testing.T) {
	p, _ := newTestPrompter("not a number\n")
	resp, err := p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{ToolName: "x", Question: "?"})
	if err == nil {
		t.Fatal("expected parse error")
	}
	// Decision should be DenyOnce on parse failure (fail-closed).
	if resp.Decision != hgapi.PermissionDecisionDenyOnce {
		t.Errorf("on parse error, got %v, want DenyOnce", resp.Decision)
	}
}

func TestCLIPrompter_PromptPermission_OutOfRange_FailsClosed(t *testing.T) {
	p, _ := newTestPrompter("99\n")
	resp, err := p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{
		ToolName: "x",
		Question: "?",
		// no suggestions → max valid is 3 (deny always)
	})
	if err == nil {
		t.Fatal("expected out-of-range error")
	}
	if resp.Decision != hgapi.PermissionDecisionDenyOnce {
		t.Errorf("on out-of-range, got %v, want DenyOnce", resp.Decision)
	}
}

func TestCLIPrompter_PromptPermission_RendersSuggestionLabels(t *testing.T) {
	// Verify the prompt output includes the suggestion labels at the
	// expected slot numbers.
	p, out := newTestPrompter("1\n")
	_, _ = p.PromptPermission(context.Background(), hgapi.PromptPermissionRequest{
		ToolName: "file-read",
		Question: "outside workspace",
		Suggestions: []hgapi.PermissionSuggestion{
			{Label: "narrow scope", Rule: "x"},
			{Label: "broad scope", Rule: "y"},
		},
	})
	rendered := out.String()
	for _, want := range []string{
		"outside workspace",
		"1. Deny once",
		"2. Allow once",
		"3. narrow scope",
		"4. broad scope",
		`5. Never allow tool "file-read"`,
		"Choose [1]:",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered output missing %q\nfull:\n%s", want, rendered)
		}
	}
}

// ── Smoke: existing PromptConfirm path still works ─────────────────────

func TestCLIPrompter_PromptConfirm_StillWorks(t *testing.T) {
	p, _ := newTestPrompter("y\n")
	resp, err := p.Prompt(context.Background(), hgapi.PromptRequest{Type: "confirm", Question: "ok?"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Confirmed {
		t.Error("expected Confirmed=true")
	}
}
