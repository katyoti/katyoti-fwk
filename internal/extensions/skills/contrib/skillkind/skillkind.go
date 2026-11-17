// SPDX-License-Identifier: Apache-2.0

// Package skillkind is the `skill` kind as the skills pack declares it: its
// contribution-type (skill.yaml, the schema provider-relative) and its
// data-type, the shape of mbapi.SkillBody. A leaf: it imports the leaf shape
// and the Go-type helper only, so the built-in kind reader
// (internal/core/contribution/builtin) can import it without a cycle.
package skillkind

import (
	_ "embed"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed skill.yaml
var declaration []byte

// Leaf is the skill kind's declaration: its YAML and its schema.
func Leaf() leaf.Declaration {
	return leaf.Declaration{
		Kind: mbapi.ResourceTypeSkill,
		YAML: declaration,
		Schema: manifestcontrib.GoTypeOf[mbapi.SkillBody]("skill", "3.0.0", "Skill",
			"A progressively-disclosed capability: L0 meta + L1 instructions + L2\n"+
				"references/scripts. This schema is the SKILL.md frontmatter: identity and\n"+
				"picker fields, plus the skill's dependencies and exposure. Pair with the\n"+
				"skill_access plugin tools (get-skill-instructions / get-skill-reference /\n"+
				"run-skill-script) for runtime access.\n"),
	}
}
