// SPDX-License-Identifier: Apache-2.0

package permrules

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// ContributionTypePermissionProfile is the contribution kind a permission
// profile is published as: `<provider>:/permission-profile/<name>`.
// The agent extension declares it; the base profiles are the agent's own
// (workspace-edit, plan-mode, full-access) and any member publishes its own
// (ktw-compose publishes compose-edit).
const ContributionTypePermissionProfile = "permission-profile"

// ProfileSpec is a permission profile as written: its data-type
// (ktw-agents:/data-type/permission-profile) and the body of a
// `permission-profile` contribution. Rules are kept as strings so the YAML
// stays human-editable; NewProfile parses them. The profile's name is its
// contribution's id.
type ProfileSpec struct {
	Description string   `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=What the profile allows and when to use it"`
	AllowRules  []string `json:"allow_rules,omitempty" yaml:"allow_rules,omitempty" jsonschema:"description=Rules pre-approving a tool call that would ask (tool.dimension:glob or tool:** or *) - first match wins"`
	DenyRules   []string `json:"deny_rules,omitempty" yaml:"deny_rules,omitempty" jsonschema:"description=Rules refusing a tool call outright - checked before the allow rules"`
}

// LoadProfile parses a profile YAML document and returns the constructed
// Profile under the given name (the session-additions file is read so).
//
// All rule strings are validated up-front via ParseRule. A malformed
// rule produces an error naming the rule that failed; partial loads
// are not allowed (the user gets a clear failure rather than a
// silently degraded profile).
//
// The decode pipeline goes through config.DecodeBytes per the
// project rule (CLAUDE.md): variable expansion + ProcessConfig run
// on every config file regardless of format.
func LoadProfile(name string, data []byte) (*Profile, error) {
	var pf ProfileSpec
	if err := config.DecodeBytes(data, marshal.YAML, &pf); err != nil {
		return nil, fmt.Errorf("permrules: decode profile %q: %w", name, err)
	}
	return NewProfile(name, pf)
}

// NewProfile builds the named Profile from its spec, every rule validated
// up-front: a malformed rule fails the profile, naming the rule.
func NewProfile(name string, pf ProfileSpec) (*Profile, error) {
	allow, err := parseRuleList(pf.AllowRules)
	if err != nil {
		return nil, fmt.Errorf("permrules: profile %q allow_rules: %w", name, err)
	}
	deny, err := parseRuleList(pf.DenyRules)
	if err != nil {
		return nil, fmt.Errorf("permrules: profile %q deny_rules: %w", name, err)
	}

	return &Profile{
		Name:        name,
		Description: pf.Description,
		AllowRules:  allow,
		DenyRules:   deny,
	}, nil
}

// parseRuleList parses every rule string in the slice or returns the
// first parse error. Used by LoadProfile for both allow and deny
// lists.
func parseRuleList(strs []string) ([]Rule, error) {
	if len(strs) == 0 {
		return nil, nil
	}
	out := make([]Rule, 0, len(strs))
	for i, s := range strs {
		r, err := ParseRule(s)
		if err != nil {
			return nil, fmt.Errorf("rule[%d] %q: %w", i, s, err)
		}
		out = append(out, r)
	}
	return out, nil
}
