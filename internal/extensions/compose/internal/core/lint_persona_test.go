// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
)

// TestLintPersonaRefusesAnUnknownConfigKey: lint-persona refuses a persona
// whose config carries a key the agent strategy does not accept (the run
// would refuse it at start), and passes one whose keys it knows.
func TestLintPersonaRefusesAnUnknownConfigKey(t *testing.T) {
	dir := t.TempDir()
	write := func(name, cfg string) string {
		p := filepath.Join(dir, name, "TASK.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		src := "---\nname: " + name + "\ntype: agent-loop\ncategory: persona\ndescription: A persona.\nskills: [a-skill]\nconfig:\n" + cfg + "---\n\nDo the work. Call complete when done.\n"
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	s := &Service{}
	ctx := context.Background()

	bad, err := s.LintPersona(ctx, api.LintPersonaInput{File: write("bad", "  capability: smart\n  max_turns: 3\n")})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range bad.Findings {
		if f.Rule == contentlint.RulePersonaConfigUnknownKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("lint-persona passed `max_turns` (the json tag is maxTurns): %+v", bad.Findings)
	}

	good, err := s.LintPersona(ctx, api.LintPersonaInput{File: write("good", "  capability: smart\n  maxTurns: 3\n  plugins: [augmented]\n  _mode: x\n")})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range good.Findings {
		if f.Rule == contentlint.RulePersonaConfigUnknownKey {
			t.Fatalf("lint-persona refused a known key: %s", f.Message)
		}
	}
}
