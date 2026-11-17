// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// lint-universe --project judges a project's contributions with the members:
// a token a member answers to, or the system's, and an object file in the
// contributions folder, are each an error; a project with no contributions
// folder is judged on nothing.
func TestLintUniverse_JudgesAProjectsContributions(t *testing.T) {
	project := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(project, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("project.cwm.yaml", "id: ktw-a\n")
	members := []*mbapi.Manifest{{Provider: "ktw-a"}, {Provider: "ktw-b"}}
	if got := projectFindings(project, members); len(got) != 0 {
		t.Fatalf("no contributions folder: %v; want nothing judged", got)
	}
	write("modules/sys-contributions/skills/greet.md", "---\nname: greet\ndescription: d\n---\nHi.\n")
	write("modules/sys-contributions/skills/memo.cwm.yaml", "id: memo\n")
	// The module's own object, when the folder is the sys-contributions
	// module, is no stray object file.
	write("modules/sys-contributions/objects/sys-contributions.cwm.yaml", "id: sys-contributions\n")
	rules := map[string]string{}
	cwmFiles := 0
	for _, f := range projectFindings(project, members) {
		rules[f.Rule] = f.URI
		if f.Rule == "project-cwm-file" {
			cwmFiles++
		}
	}
	if cwmFiles != 1 {
		t.Fatalf("%d object-file findings; want only the one among the skills", cwmFiles)
	}
	if rules["project-token"] != "ktw-a:/" || rules["project-cwm-file"] != "modules/sys-contributions/skills/memo.cwm.yaml" || len(rules) != 2 {
		t.Fatalf("findings = %v; want the token ktw-a and the object file", rules)
	}
	write("project.cwm.yaml", "id: ktw-a\ncapabilities:\n  contributions:\n    provider: system\n")
	if got := projectFindings(project, members); len(got) != 2 || got[0].Rule != "project-token" {
		t.Fatalf("token system: %v; want it refused", got)
	}
	write("project.cwm.yaml", "id: acme\n")
	if err := os.Remove(filepath.Join(project, "modules", "sys-contributions", "skills", "memo.cwm.yaml")); err != nil {
		t.Fatal(err)
	}
	if got := projectFindings(project, members); len(got) != 0 {
		t.Fatalf("a free token, no object file: %v", got)
	}
}
