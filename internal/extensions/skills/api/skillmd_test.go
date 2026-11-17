// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

func TestParseSkillMD(t *testing.T) {
	input := `---
name: flow-spec
description: Guides collection of inter-application data flow specifications
---

# Skill: Flow Specification Collector

Guide the user to specify each field of a data flow.

See [schema](references/flow-schema.yaml) for the output format.
`
	s, err := ParseSkillMD([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if s.Frontmatter.Name != "flow-spec" {
		t.Errorf("name = %q, want %q", s.Frontmatter.Name, "flow-spec")
	}
	if s.Frontmatter.Description == "" {
		t.Error("description should not be empty")
	}
	if s.Instructions == "" {
		t.Error("instructions should not be empty")
	}
}

func TestParseSkillMDWithRequires(t *testing.T) {
	input := `---
name: doc-create
description: Creates documents
requires:
  - workspace
  - skill:template-engine
---

Instructions here.
`
	s, err := ParseSkillMD([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(s.Frontmatter.Requires) != 2 {
		t.Fatalf("requires = %v, want 2 entries", s.Frontmatter.Requires)
	}
	if s.Frontmatter.Requires[0] != "workspace" {
		t.Errorf("requires[0] = %q, want %q", s.Frontmatter.Requires[0], "workspace")
	}
	if s.Frontmatter.Requires[1] != "skill:template-engine" {
		t.Errorf("requires[1] = %q, want %q", s.Frontmatter.Requires[1], "skill:template-engine")
	}
}

func TestParseSkillMDMissingName(t *testing.T) {
	_, err := ParseSkillMD([]byte("---\ndescription: No name\n---\nBody"))
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParseSkillMDMissingDescription(t *testing.T) {
	_, err := ParseSkillMD([]byte("---\nname: my-skill\n---\nBody"))
	if err == nil {
		t.Error("expected error for missing description")
	}
}

func TestParseSkillMDInvalidNameChars(t *testing.T) {
	_, err := ParseSkillMD([]byte("---\nname: My_Skill\ndescription: test\n---\nBody"))
	if err == nil {
		t.Error("expected error for uppercase/underscore in name")
	}
}

func TestParseSkillMDNameLeadingHyphen(t *testing.T) {
	_, err := ParseSkillMD([]byte("---\nname: -bad\ndescription: test\n---\nBody"))
	if err == nil {
		t.Error("expected error for leading hyphen")
	}
}

func TestParseSkillMDNameTooLong(t *testing.T) {
	longName := ""
	for i := 0; i < 65; i++ {
		longName += "a"
	}
	_, err := ParseSkillMD([]byte("---\nname: " + longName + "\ndescription: test\n---\nBody"))
	if err == nil {
		t.Error("expected error for name > 64 chars")
	}
}

func TestParseSkillMDDescriptionTooLong(t *testing.T) {
	longDesc := ""
	for i := 0; i < 1025; i++ {
		longDesc += "x"
	}
	_, err := ParseSkillMD([]byte("---\nname: ok\ndescription: " + longDesc + "\n---\nBody"))
	if err == nil {
		t.Error("expected error for description > 1024 chars")
	}
}
