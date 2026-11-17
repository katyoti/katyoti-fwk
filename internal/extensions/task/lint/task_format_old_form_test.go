// SPDX-License-Identifier: Apache-2.0

package tasklint

import (
	"strings"
	"testing"
)

// The old form (an inline contract, the snake keys) is still read by the
// engine and named by the lint, with what to write instead.
func TestLintTaskFormat_NamesTheOldForm(t *testing.T) {
	data := []byte("---\nname: x\ntype: agent\ndescription: d\npermission_profile: full\ninput-schema:\n  name: x-input\n---\nDo it. Call complete when done.\n")
	var rules []string
	for _, f := range LintTaskFormatBytes(data).Findings {
		if f.Rule == "task-old-form" {
			rules = append(rules, f.Message)
		}
	}
	got := strings.Join(rules, " | ")
	if len(rules) != 2 || !strings.Contains(got, "input.dsl.yaml") || !strings.Contains(got, "permissionProfile") {
		t.Fatalf("old-form findings = %q; want the inline contract and the snake key named", got)
	}
}
