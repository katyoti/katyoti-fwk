// SPDX-License-Identifier: Apache-2.0

package skilllint

import (
	"strings"
	"testing"

	skillsapi "github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
)

// G31: the lint reads a skill through the provider's own parser. A file the
// provider refuses is refused by the lint with the same reason; a file the
// provider accepts never gets a parse finding from the lint. Two parsers is
// how a lint says "clean" about a file the sidecar then drops at build.
func TestLintSkillFormat_RefusesWhatTheProviderDrops(t *testing.T) {
	cases := map[string]string{
		"unknown key":         "---\nname: x\ndescription: X.\nowner: me\n---\n\nbody\n",
		"missing name":        "---\ndescription: X.\n---\n\nbody\n",
		"missing description": "---\nname: x\n---\n\nbody\n",
		"bad name":            "---\nname: Not Kebab\ndescription: X.\n---\n\nbody\n",
		"no frontmatter":      "# just markdown\n",
	}
	for label, src := range cases {
		_, perr := skillsapi.ParseSkillMD([]byte(src))
		res := LintSkillFormatBytes([]byte(src))
		if perr == nil {
			t.Fatalf("%s: the provider accepts this fixture; pick one it refuses", label)
		}
		if res.Status == "pass" {
			t.Errorf("%s: the provider refuses (%v) but the lint passes: %v", label, perr, res.Findings)
		}
	}
	clean := "---\nname: x\ndescription: X.\n---\n\nbody\n"
	if _, perr := skillsapi.ParseSkillMD([]byte(clean)); perr != nil {
		t.Fatal(perr)
	}
	for _, f := range LintSkillFormatBytes([]byte(clean)).Findings {
		if strings.HasPrefix(f.Rule, "frontmatter-") || f.Rule == "description-missing" {
			t.Errorf("the provider accepts the file but the lint raised %s", f.Rule)
		}
	}
}
