// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
)

// stubSkill implements api.Skill for testing.
type stubSkill struct {
	name     string
	desc     string
	requires []string
	body     string
}

func (s *stubSkill) Title() api.SkillInfo {
	return api.SkillInfo{Name: s.name, Description: s.desc, Requires: s.requires}
}

func (s *stubSkill) Instructions() string { return s.body }

func (s *stubSkill) Reference(path string) (string, error) { return "ref:" + path, nil }

func makeManager() api.SkillManager {
	return &skillManager{skills: map[string]api.Skill{
		"doc-create": &stubSkill{
			name: "doc-create", desc: "Create documents",
			requires: []string{"workspace", "skill:template-engine"}, body: "# Doc Create\nInstructions.",
		},
		"flow-spec": &stubSkill{
			name: "flow-spec", desc: "Collect flow specs",
			body: "# Flow Spec\nGuide user.",
		},
	}}
}

// TestList / TestListWithFilter / TestListShowsRequires / TestGetInstructions
// removed in step 13 of bus-split-aggregator-to-catalog — SkillManager no
// longer has List or GetInstructions. Equivalent coverage in
// skills/api/catalog_view_test.go (TestSkillsFromCatalog +
// TestInstructionsFromCatalog).

func TestGetSkill(t *testing.T) {
	mgr := makeManager()
	skill, err := mgr.GetSkill("flow-spec")
	if err != nil {
		t.Fatalf("GetSkill failed: %v", err)
	}
	info := skill.Title()
	if info.Name != "flow-spec" {
		t.Errorf("got %q, want %q", info.Name, "flow-spec")
	}
}

func TestGetSkillNotFound(t *testing.T) {
	mgr := makeManager()
	_, err := mgr.GetSkill("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

// GetReference now uses provider-scoped loaders (not per-skill).
// Tested via integration tests with actual embed.FS loaders.
