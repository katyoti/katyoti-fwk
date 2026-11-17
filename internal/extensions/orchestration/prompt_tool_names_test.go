// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"regexp"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// A prompt names a tool as it is registered: a model told to call
// `send_message` calls a tool that does not exist (they are kebab-case).
func TestPromptsNameToolsAsRegistered(t *testing.T) {
	names := []string{"task-status", "delegate", "complete"} // the task plugin's
	for _, d := range orchToolDefs() {
		names = append(names, d.Name)
	}
	texts := map[string]string{
		"worker": workerPrompt, "background": backgroundPrompt, "coordinator": coordinatorPrompt,
		"team-leader": teamLeaderPrompt, "teammate": teammatePrompt,
	}
	for _, m := range presets.KnownModes() {
		texts["preset "+string(m)] = presets.PromptTemplate(m)
	}
	for _, name := range names {
		if !strings.Contains(name, "-") {
			continue
		}
		wrong := regexp.MustCompile(`\b` + strings.ReplaceAll(name, "-", "_") + `\b`)
		for where, text := range texts {
			if wrong.MatchString(text) {
				t.Errorf("the %s prompt names %q; the tool is %q", where, wrong.String(), name)
			}
		}
	}
}
