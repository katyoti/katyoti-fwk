// SPDX-License-Identifier: Apache-2.0

package contentlint

import (
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// TestUnknownAgentConfigKeysInTheCensusLint: the body the census runs on a
// persona entry (LintBytes) reports a config key the agent strategy refuses,
// through the same check as lint-persona.
func TestUnknownAgentConfigKeysInTheCensusLint(t *testing.T) {
	src := []byte("---\nname: p\ncategory: persona\nskills: [s]\nconfig:\n  capability: smart\n  permission_mode: auto\n---\n\nBody.\n")
	res := LintBytes(mbapi.ResourceTypePersona, src, "ktw-x", nil, ScopeSelf)
	found := false
	for _, f := range res.Findings {
		if f.Rule == RulePersonaConfigUnknownKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("census lint passed `permission_mode`: %+v", res.Findings)
	}
	ok := []byte("---\nname: p\ncategory: persona\nconfig:\n  capability: smart\n  plugins: [cwm]\n---\n\nBody.\n")
	for _, f := range LintBytes(mbapi.ResourceTypePersona, ok, "ktw-x", nil, ScopeSelf).Findings {
		if f.Rule == RulePersonaConfigUnknownKey {
			t.Fatalf("census lint refused a known key: %s", f.Message)
		}
	}
}
