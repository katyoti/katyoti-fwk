// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"strings"
	"testing"
)

var fixtureSkills = []SkillInfo{
	{
		Name:        "doc-create",
		Provider:    "ktw-docs",
		Description: "Creates professional documents from markdown.",
	},
	{
		Name:        "example-report",
		Provider:    "ktw-example",
		Description: "Reports on example items.",
		Requires:    []string{"workspace", "skill:example-guide"},
	},
}

func TestBuildSystemPrompt_AgentMode_Structure(t *testing.T) {
	got := BuildSystemPrompt(fixtureSkills, PromptModeAgent, "")

	mustContain := []string{
		"<skills_system>",
		"</skills_system>",
		"## What are Skills?",
		"## Addressing",
		// The three addressable kinds + their URI shapes.
		"**resource** (read)",
		"`<provider>:/<type>/<id>`",
		"**tool** (call)",
		"`<provider>:<slot>/<method>`",
		"**event** (subscribe)",
		"`<provider>:!<scope>/<path>`",
		"`skills-get-instruction`",
		// Any non-skill resource's content goes through the read-resource
		// tool, addressed by full URI. One method per outcome.
		"**read-resource**",
		// The list is not exhaustive — unlisted resources load by exact URI.
		"loadable by its exact URI even when unlisted",
		"<name>ktw-docs:/skill/doc-create</name>",
		"<name>ktw-example:/skill/example-report</name>",
		"<requires>workspace, skill:example-guide</requires>",
	}
	for _, frag := range mustContain {
		if !strings.Contains(got, frag) {
			t.Errorf("agent prompt missing %q\n--- output ---\n%s", frag, got)
		}
	}
}

func TestBuildSystemPrompt_CLIMode(t *testing.T) {
	got := BuildSystemPrompt(fixtureSkills, PromptModeCLI, "ktw-example")

	mustContain := []string{
		"<skills_system>",
		"</skills_system>",
		"## Addressing",
		"**resource** (read)",
		"**tool** (call)",
		"**event** (subscribe)",
		"`ktw-example llm resources read <provider>:/<type>/<id>`",
		"`ktw-example llm tools call <provider>:<slot>/<method>",
		"`ktw-example llm tools describe <tool-uri>`",
		"`ktw-example llm resources list`",
		"<name>ktw-docs:/skill/doc-create</name>",
		"<name>ktw-example:/skill/example-report</name>",
	}
	for _, frag := range mustContain {
		if !strings.Contains(got, frag) {
			t.Errorf("CLI prompt missing %q\n--- output ---\n%s", frag, got)
		}
	}

	mustNotContain := []string{
		"get_skill_instructions(",
		"get_skill_reference(",
	}
	for _, frag := range mustNotContain {
		if strings.Contains(got, frag) {
			t.Errorf("CLI prompt leaked agent-mode tool name %q", frag)
		}
	}
}

func TestBuildSystemPrompt_CLIMode_DefaultBinary(t *testing.T) {
	got := BuildSystemPrompt(fixtureSkills, PromptModeCLI, "")
	if !strings.Contains(got, "`<binary> llm resources read") {
		t.Fatalf("expected `<binary>` placeholder fallback, got:\n%s", got)
	}
}

func TestBuildSystemPrompt_EmptySkills(t *testing.T) {
	got := BuildSystemPrompt(nil, PromptModeCLI, "ktw-notify")
	if !strings.Contains(got, "## Addressing") {
		t.Fatalf("Addressing section missing on empty list:\n%s", got)
	}
	if !strings.Contains(got, "## Available Skills") {
		t.Fatalf("Available Skills section missing on empty list:\n%s", got)
	}
}

func TestBuildSystemPrompt_CompactIndexAboveThreshold(t *testing.T) {
	// A large attached set switches to the compact provider-grouped index:
	// one line per skill (name + description kept), no per-skill <skill> XML.
	var many []SkillInfo
	for i := 0; i < skillListThreshold+5; i++ {
		many = append(many, SkillInfo{
			Name:        fmt.Sprintf("skill-%02d", i),
			Provider:    "ktw-methods",
			Description: "does a thing",
		})
	}
	got := BuildSystemPrompt(many, PromptModeAgent, "")

	if strings.Contains(got, "<skill>\n") {
		t.Error("large set must NOT use per-skill <skill> XML blocks")
	}
	if !strings.Contains(got, "### ktw-methods") {
		t.Error("compact index must group under a ### <provider> header")
	}
	if !strings.Contains(got, "- `ktw-methods:/skill/skill-00` — does a thing") {
		t.Errorf("compact index must keep the URI + description one-liner; got:\n%s", got)
	}
	if !strings.Contains(got, "skills available, grouped by provider") {
		t.Error("compact index must announce the count + grouping")
	}
}

func TestBuildSystemPrompt_FullEntriesAtThreshold(t *testing.T) {
	// At/below the threshold, the full <skill> form is preserved (the common
	// scoped case — no behavior change).
	var n []SkillInfo
	for i := 0; i < skillListThreshold; i++ {
		n = append(n, SkillInfo{Name: fmt.Sprintf("s%02d", i), Provider: "p", Description: "d"})
	}
	got := BuildSystemPrompt(n, PromptModeAgent, "")
	if !strings.Contains(got, "<skill>\n") {
		t.Error("a set AT the threshold must still use full <skill> blocks")
	}
}

func TestSkillInfo_URI(t *testing.T) {
	tests := []struct {
		info SkillInfo
		want string
	}{
		{SkillInfo{Name: "doc-create", Provider: "ktw-docs"}, "ktw-docs:/skill/doc-create"},
		{SkillInfo{Name: "doc-create", Provider: ""}, ":/skill/doc-create"},
	}
	for _, tt := range tests {
		if got := tt.info.URI(); got != tt.want {
			t.Errorf("URI() = %q, want %q", got, tt.want)
		}
	}
}
