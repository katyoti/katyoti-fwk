// SPDX-License-Identifier: Apache-2.0

package contentlint

import (
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

// RulePersonaConfigUnknownKey names the finding of a persona `config:` key
// the agent strategy does not accept.
const RulePersonaConfigUnknownKey = "persona-config-unknown-key"

// lintPersonaConfigBytes checks the top-level keys of a persona's `config:`
// block through the agent strategy's own key check (agentapi.UnknownConfigKeys,
// the rule the run applies at start) — never a copy of it. Only the config
// keys: a persona's other frontmatter (skills, augments, applies_to) is its
// binding metadata, not a task's.
func lintPersonaConfigBytes(src []byte) []verify.LintFinding {
	fm, _, ok := verify.SplitFrontmatter(string(src))
	if !ok {
		return nil
	}
	var head struct {
		Config map[string]any `json:"config" yaml:"config"`
	}
	if err := config.DecodeBytes([]byte(fm), marshal.YAML, &head); err != nil || len(head.Config) == 0 {
		return nil
	}
	keys := make([]string, 0, len(head.Config))
	for k := range head.Config {
		keys = append(keys, k)
	}
	unknown := agentapi.UnknownConfigKeys(keys)
	if len(unknown) == 0 {
		return nil
	}
	return []verify.LintFinding{{
		Rule:     RulePersonaConfigUnknownKey,
		Message:  "config: unknown key(s) " + strings.Join(unknown, ", ") + " — not keys of an agent's config (the agent run refuses them at start: maxTurns, mode, plugins, skills, tools, …)",
		Severity: verify.SeverityError,
	}}
}
