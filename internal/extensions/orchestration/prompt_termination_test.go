// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"regexp"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// ordersCompleteTool matches a sentence that orders the agent to call the
// `complete` TOOL by name — "call the **complete** tool", "use the
// `complete` tool". A bare "call complete" is not enough: a model reads it
// as a state, not a tool, and ends on plain text (the loop's silent
// fallback, no `complete` tool call in the journal).
var ordersCompleteTool = regexp.MustCompile("(?i)\\b(call|use) the (\\*\\*complete\\*\\*|`complete`) tool\\b")

// Every mode whose run finishes itself (Finish=self) is told, by name, to
// call the `complete` tool — through the prompt the agent actually receives
// (GetModePrompt, which prefers the orchestration texts) AND through the
// preset's own template. A driven mode (chat) hides `complete`, so its prompt
// must not order it. Rule:.
func TestSelfFinishingModePromptsOrderTheCompleteTool(t *testing.T) {
	cm := NewCoordinatorManager()
	for _, m := range presets.KnownModes() {
		cfg, err := presets.ResolvePreset(m, nil)
		if err != nil {
			t.Fatalf("resolve %s: %v", m, err)
		}
		texts := map[string]string{
			"effective": cm.GetModePrompt(string(m)),
			"preset":    presets.PromptTemplate(m),
		}
		selfFinishing := presets.EffectiveFinish(cfg.Finish, "") == presets.FinishSelf
		for where, text := range texts {
			if text == "" {
				t.Errorf("mode %s has no %s prompt", m, where)
				continue
			}
			got := ordersCompleteTool.MatchString(text)
			if selfFinishing && !got {
				t.Errorf("mode %s finishes itself but its %s prompt does not order the `complete` tool by name:\n%s", m, where, text)
			}
			if !selfFinishing && got {
				t.Errorf("mode %s is driven (complete hidden) but its %s prompt orders the `complete` tool", m, where)
			}
		}
	}
}
