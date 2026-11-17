// SPDX-License-Identifier: Apache-2.0

package tasklint

import (
	"strings"
	"testing"

	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"
)

// G31: the lint reads a task through the provider's own parser. A file the
// provider refuses is refused by the lint with the same reason; a file the
// provider accepts never gets a parse finding from the lint.
func TestLintTaskFormat_RefusesWhatTheProviderDrops(t *testing.T) {
	cases := map[string]string{
		"unknown root key":    "---\nname: t\ntype: agent\ndescription: T.\nowner: me\n---\n\nCall complete when done.\n",
		"runtime key at root": "---\nname: t\ntype: agent\ndescription: T.\nplugins: [standard]\n---\n\nCall complete when done.\n",
		"missing type":        "---\nname: t\ndescription: T.\n---\n\nbody\n",
		"node without types":  "---\nname: t\ntype: agent\ndescription: T.\nkind: node\n---\n\nCall complete when done.\n",
		"no frontmatter":      "# just markdown\n",
	}
	for label, src := range cases {
		_, perr := taskcontrib.Parse([]byte(src))
		res := LintTaskFormatBytes([]byte(src))
		if perr == nil {
			t.Fatalf("%s: the provider accepts this fixture; pick one it refuses", label)
		}
		if res.Status == "pass" {
			t.Errorf("%s: the provider refuses (%v) but the lint passes: %v", label, perr, res.Findings)
		}
	}
	clean := "---\nname: t\ntype: agent\ndescription: T.\nconfig:\n  plugins: [standard]\n---\n\nDo the thing. Call complete when done.\n"
	if _, perr := taskcontrib.Parse([]byte(clean)); perr != nil {
		t.Fatal(perr)
	}
	for _, f := range LintTaskFormatBytes([]byte(clean)).Findings {
		if strings.HasPrefix(f.Rule, "frontmatter-") || strings.HasPrefix(f.Rule, "required-") || f.Rule == "runtime-config-at-root" {
			t.Errorf("the provider accepts the file but the lint raised %s", f.Rule)
		}
	}
}
