// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// `solution show` lists what a solution needs before it is installed: its
// programs (required or optional, version, platforms), its keys and its logins.
func TestSolutionShow_ListsNeedsBeforeInstall(t *testing.T) {
	r := mktapi.NeedsOf("ktw-kb",
		[]publish.ToolRequirement{
			{Name: "Pandoc", Required: true, Version: "3.6", Recipes: []publish.InstallRecipe{{Platform: "windows", Kind: "winget", Package: "p"}}},
			{Name: "ffmpeg", InstallSkill: "ktw-kb:/skill/install-ffmpeg"},
		},
		[]publish.APIKeyRequirement{{EnvName: "OPENAI_API_KEY", Required: true}},
		[]publish.AuthRequirement{{Name: "google", LoginHint: "ktw-example auth login"}})
	var b bytes.Buffer
	printNeeds(&b, r)
	out := b.String()
	for _, want := range []string{
		"needs before install:",
		"program  Pandoc (required, 3.6)  recipes: windows  for ktw-kb",
		"program  ffmpeg (optional)  manual: ktw-kb:/skill/install-ffmpeg  for ktw-kb",
		"key      OPENAI_API_KEY (required)  for ktw-kb",
		"login    google  (ktw-example auth login)  for ktw-kb",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	b.Reset()
	printNeeds(&b, mktapi.Requires{})
	if b.Len() != 0 {
		t.Errorf("a solution needing nothing prints %q", b.String())
	}
}
