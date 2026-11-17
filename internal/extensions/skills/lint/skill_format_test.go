// SPDX-License-Identifier: Apache-2.0

package skilllint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

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

// defectiveSkill carries every skill-format defect the trend-research artifact
// originally had — a stable fixture so the test does not depend on the live
// in-repo skill (which gets fixed). Modelled on that artifact's real defects.
const defectiveSkill = `---
name: bad-skill
description: Research IT trends for a Dossier d'architecture section and produce a structured markdown trend document. Uses web search tools to gather current, sourced information per trend. Load when the user asks to research trends, produce a trend analysis, or fill the veille technologique section with current technology landscape data.
requires: [web]
---

# Bad skill

There are no new KTSP tools here. This skill drives the LLM agent.

## When to use

- when the user wants a trend analysis
`

// TestLintSkillFormat_DetectsDefects is the deterministic AC: a skill carrying
// the classic defects MUST surface them all — over-long multi-sentence
// description, internal "KTSP" jargon, meta-commentary ("There are no …"), and
// a `When to use` body section. No LLM, no stack: the proof is the finding list.
func TestLintSkillFormat_DetectsDefects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-skill.md")
	if err := os.WriteFile(path, []byte(defectiveSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ruleSet(LintSkillFormat(path))
	for _, rule := range []string{
		"description-too-long",
		"description-multi-sentence",
		"internal-jargon",
		"meta-commentary",
		"when-to-use-section",
	} {
		if _, ok := got[rule]; !ok {
			t.Errorf("expected finding %q — not listed; got rules %v", rule, keys(got))
		}
	}

	// internal-jargon is a warning since 2026-09-22 (a word, not a broken
	// contract): the file's findings are all listed and none blocks the gate.
	if got["internal-jargon"] != verify.SeverityWarn {
		t.Errorf("internal-jargon must be a warning, got %q", got["internal-jargon"])
	}
	if LintSkillFormat(path).Status != "pass" {
		t.Errorf("expected status=pass (warnings only), got %q", LintSkillFormat(path).Status)
	}
}

// TestLintSkillFormat_TaskShaped is the artifact-TYPE AC: a skill that declares
// task frontmatter (a runnable inputs→output) is mis-typed and must be flagged
// with the precise `skill-is-task-shaped` finding (once), not a generic
// extra-key — it should have been authored as a task.
func TestLintSkillFormat_TaskShaped(t *testing.T) {
	taskShaped := `---
name: forecast
description: Produce a forecast — load when asked for a forecast.
type: agent
input-schema:
  type: object
output-schema:
  type: object
---

# Forecast

Run the forecast.
`
	path := filepath.Join(t.TempDir(), "forecast.md")
	if err := os.WriteFile(path, []byte(taskShaped), 0o644); err != nil {
		t.Fatal(err)
	}
	r := LintSkillFormat(path)
	got := ruleSet(r)
	if _, ok := got["skill-is-task-shaped"]; !ok {
		t.Errorf("expected skill-is-task-shaped finding; got %v", keys(got))
	}
	if _, ok := got["frontmatter-extra-key"]; ok {
		t.Error("task-shape keys must get the type diagnosis, not a generic extra-key flag")
	}
	if r.Status != "fail" {
		t.Errorf("a task-shaped skill must fail the gate, got %q", r.Status)
	}
	// Emitted once even though three task keys are present.
	n := 0
	for _, f := range r.Findings {
		if f.Rule == "skill-is-task-shaped" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("skill-is-task-shaped should fire once, fired %d×", n)
	}
}

// TestLintSkillFormat_SurfaceLoader is the vocabulary AC: a skill body that names
// a surface-specific loader (the agent tool `skills-get-instruction`, a CLI
// `<binary> skills get-instructions`, …) must be flagged — bodies name the URI
// with the canonical verb and let the host bind it.
func TestLintSkillFormat_SurfaceLoader(t *testing.T) {
	for _, body := range []string{
		"Load it via `skills-get-instruction` by its exact URI.",
		"Run `ktw-compose skills get-instructions task-creation`.",
		"Use the `get_skill_instructions` tool.",
		"Fetch it with `sc resources read <uri>`.",
	} {
		content := "---\nname: x\ndescription: A skill — load when asked.\n---\n\n# X\n\n" + body + "\n"
		path := filepath.Join(t.TempDir(), "x.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		got := ruleSet(LintSkillFormat(path))
		if _, ok := got["surface-specific-invocation"]; !ok {
			t.Errorf("expected surface-specific-invocation for %q; got %v", body, keys(got))
		}
	}
	// The canonical verb form is clean.
	clean := "---\nname: x\ndescription: A skill — load when asked.\n---\n\n# X\n\nGet-instruction the **ktw-compose:/skill/create-task** skill, then follow it.\n"
	path := filepath.Join(t.TempDir(), "ok.md")
	if err := os.WriteFile(path, []byte(clean), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := ruleSet(LintSkillFormat(path))["surface-specific-invocation"]; ok {
		t.Error("canonical 'get-instruction the <uri> skill' must NOT be flagged")
	}
}

// TestLintSkillFormat_CleanPasses guards the other direction — a contract-clean
// skill yields no findings (so a fixed artifact stops blocking the gate).
func TestLintSkillFormat_CleanPasses(t *testing.T) {
	clean := `---
name: good-skill
description: Research IT trends for a dossier section — load when asked to fill the veille section.
requires: [web]
---

# Good skill

Resolve the inputs, then call the **web:search/query** tool for each trend.
Call **complete** when done.
`
	path := filepath.Join(t.TempDir(), "good-skill.md")
	if err := os.WriteFile(path, []byte(clean), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := LintSkillFormat(path); r.Status != "pass" {
		t.Errorf("clean skill should pass, got %v", r)
	}
}
