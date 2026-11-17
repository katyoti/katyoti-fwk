// SPDX-License-Identifier: Apache-2.0

package api

// Skill is the internal interface for a single loaded skill.
//
// Progressive discovery:
//   - L0: Title → name + description + requires
//   - L1: Instructions → full SKILL.md body
//   - L2+: Reference(path) — read any file under the skill's directory
//
// Skill instances are NOT published. They are held internally by
// SkillManager (in-process accessor only). Reference and instructions
// fetches at the published surface go through `ResourceService` —
// `Reference` here is the in-process equivalent that the agent plugin
// uses when it needs to read a file inside a skill's own folder
// (without going through the Contribution machinery).
//
// 2026-05-24: `RunScript` dropped from this interface. Skill-bundled
// scripts had no live consumer; standalone scripts live as their own
// `<provider>:scripts/<name>` slot.
type Skill interface {
	Title() SkillInfo
	Instructions() string
	Reference(path string) (string, error)
}

// SkillInfo is the L0 metadata returned by List.
type SkillInfo struct {
	Name        string   `json:"name"              desc:"Skill name"`
	Provider    string   `json:"provider"          desc:"Provider (sidecar name that hosts this skill)"`
	Description string   `json:"description"       desc:"When to use this skill"`
	Requires    []string `json:"requires,omitempty" desc:"Plugin dependencies (PluginRef format)"`

	// Display metadata (optional, for GUI/CLI rendering).
	Icon     string   `json:"icon,omitempty"     desc:"Emoji icon for terminal"`
	IconName string   `json:"iconName,omitempty" desc:"Tabler icon name for GUI"`
	Hashtags []string `json:"hashtags,omitempty" desc:"Categorization tags"`
	Category string   `json:"category,omitempty" desc:"Primary grouping category"`

	// Exposure declarations (from 'provides' field in SKILL.md frontmatter).
	Provides []string `json:"provides,omitempty" desc:"Exposure: task, cli, mcp-prompt, mcp-tool, mcp-resource"`

	// Advertised is the visibility flag (SkillBody.Advertised, default true).
	// A bulk selector (`*`, glob) matches only advertised skills; an EXACT
	// name reaches an unadvertised one (visibility ≠ callability).
	Advertised bool `json:"advertised" desc:"Whether the skill shows in bulk listings (exact name reaches it regardless)"`
}

// URI returns the skill's full URI (`<provider>:/skill/<name>` form).
// This is the canonical address per the command-surface decision — the
// short `provider:name` form was retired with the 2026-05-24 skill
// normalization. Callers that addressed skills by `provider:name`
// should switch to this URI.
func (s SkillInfo) URI() string {
	if s.Provider == "" {
		return ":/skill/" + s.Name
	}
	return s.Provider + ":/skill/" + s.Name
}
