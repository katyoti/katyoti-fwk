// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBasicLinters(t *testing.T) {
	dir := t.TempDir()
	good := write(t, dir, "good.md", "---\nname: x\ndescription: y\n---\n\nbody here.\n")
	noFM := write(t, dir, "nofm.md", "# just a heading\nno frontmatter\n")
	badFM := write(t, dir, "badfm.md", "---\nname: : : bad\n---\nbody\n")
	noBody := write(t, dir, "nobody.md", "---\nname: x\n---\n")
	empty := write(t, dir, "empty.md", "   \n\t\n")

	if r := LintFileExists(good); r.Status != "pass" {
		t.Errorf("file-exists(good) = %v", r)
	}
	if r := LintFileExists(filepath.Join(dir, "nope.md")); r.Status != "fail" {
		t.Errorf("file-exists(missing) should fail, got %v", r)
	}
	if r := LintMarkdownFrontmatter(good); r.Status != "pass" {
		t.Errorf("frontmatter(good) = %v", r)
	}
	if r := LintMarkdownFrontmatter(noFM); !has(r, "frontmatter-missing") {
		t.Errorf("frontmatter(noFM) should flag missing, got %v", r)
	}
	if r := LintMarkdownFrontmatter(badFM); !has(r, "frontmatter-invalid") {
		t.Errorf("frontmatter(badFM) should flag invalid, got %v", r)
	}
	if r := LintMarkdownFrontmatter(noBody); !has(r, "body-empty") {
		t.Errorf("frontmatter(noBody) should flag empty body, got %v", r)
	}
	if r := LintFrontmatterKeys(good, []string{"name", "description"}); r.Status != "pass" {
		t.Errorf("keys(good, name+description) = %v", r)
	}
	if r := LintFrontmatterKeys(good, []string{"name", "requires"}); !has(r, "frontmatter-key-missing") {
		t.Errorf("keys(good, requires) should flag missing, got %v", r)
	}
	if r := LintNonEmpty(empty); !has(r, "empty") {
		t.Errorf("non-empty(empty) should flag, got %v", r)
	}
	if r := LintNonEmpty(good); r.Status != "pass" {
		t.Errorf("non-empty(good) = %v", r)
	}
}

func has(r LintResult, rule string) bool {
	for _, f := range r.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestResult_StatusDerivation(t *testing.T) {
	if Result().Status != "pass" {
		t.Error("no findings → pass")
	}
	if Result(LintFinding{"x", "m", SeverityWarn}).Status != "pass" {
		t.Error("only warn → pass")
	}
	if Result(LintFinding{"x", "m", SeverityError}).Status != "fail" {
		t.Error("an error → fail")
	}
}

// A type fixes a field through value-in: a goal is asymptotic (unset reads the
// schema default, closable, so it is a finding), a feature closable.
func TestLintValueIn(t *testing.T) {
	goal := func(v string) LintResult {
		return LintValueIn(v, "closable", []string{"asymptotic"}, "goal-asymptotic", "a goal is asymptotic")
	}
	if r := goal("asymptotic"); r.Status != "pass" || len(r.Findings) != 0 {
		t.Errorf("asymptotic goal: %+v", r)
	}
	for _, v := range []string{"closable", ""} {
		r := goal(v)
		if r.Status != "fail" || len(r.Findings) != 1 || r.Findings[0].Rule != "goal-asymptotic" || r.Findings[0].Severity != SeverityError {
			t.Errorf("goal %q: %+v", v, r)
		}
	}
	feature := func(v string) LintResult {
		return LintValueIn(v, "closable", []string{"closable"}, "feature-closable", "a feature is closable")
	}
	if r := feature(""); r.Status != "pass" {
		t.Errorf("unset feature reads closable: %+v", r)
	}
	if r := feature("asymptotic"); r.Status != "fail" || r.Findings[0].Rule != "feature-closable" {
		t.Errorf("asymptotic feature: %+v", r)
	}
	if r := LintValueIn("x", "", nil, "", ""); r.Status != "error" {
		t.Errorf("no allowed values: %+v", r)
	}
}
