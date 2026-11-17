// SPDX-License-Identifier: Apache-2.0

package plugin

import "testing"

// TestSkillTools_Surface pins the agent-surface read bindings: get-instruction a
// skill + read any other resource. read-resource was missing before — agents
// then file-read source for references.
func TestSkillTools_Surface(t *testing.T) {
	names := map[string]bool{}
	for _, tl := range SkillTools() {
		names[tl.ToolDef().Name] = true
	}
	for _, want := range []string{"skills-get-instruction", "read-resource"} {
		if !names[want] {
			t.Errorf("SkillTools missing %q; got %v", want, names)
		}
	}
}
