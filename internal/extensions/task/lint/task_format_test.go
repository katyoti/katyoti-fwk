// SPDX-License-Identifier: Apache-2.0

package tasklint

import (
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	"os"
	"path/filepath"
	"testing"
)

func TestLintTaskFormat(t *testing.T) {
	dir := t.TempDir()

	// A bad task: invalid type, runtime config at root, kind:object without
	// types, agent body with no `complete`, KTSP jargon.
	bad := `---
name: my-task
type: agentt
description: does a thing
plugins: [standard]
kind: object
---

This agent does work over KTSP. It just responds.
`
	badPath := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(badPath, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ruleSet(LintTaskFormat(badPath))
	for _, want := range []string{"type-invalid", "runtime-config-at-root", "object-kind-without-types", "internal-jargon"} {
		if _, ok := got[want]; !ok {
			t.Errorf("expected finding %q on the bad task; got %v", want, keys(got))
		}
	}
	if LintTaskFormat(badPath).Status != "fail" {
		t.Errorf("bad task should fail")
	}

	// A clean task passes.
	good := `---
name: my-task
type: agent
description: does a thing
config:
  capability: fast
  plugins: [standard]
---

You are an agent. Use your tools. Call **complete** when done.
`
	goodPath := filepath.Join(dir, "good.md")
	if err := os.WriteFile(goodPath, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := LintTaskFormat(goodPath); r.Status != "pass" {
		t.Errorf("clean task should pass, got %v", r)
	}
}

func ruleSet(r verify.LintResult) map[string]verify.LintSeverity {
	m := map[string]verify.LintSeverity{}
	for _, f := range r.Findings {
		m[f.Rule] = f.Severity
	}
	return m
}

func keys(m map[string]verify.LintSeverity) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
