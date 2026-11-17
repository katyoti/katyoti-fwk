// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "scenario.tasktest")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoad_RequiredFields(t *testing.T) {
	dir := t.TempDir()

	// Missing uc → error.
	path := writeFixture(t, dir, `title: missing-uc
invoke:
  cmd: echo hi
`)
	if _, err := Load(path); err == nil {
		t.Error("expected error for missing uc")
	}

	// Missing invoke.cmd → error.
	path = writeFixture(t, dir, `uc: UC-001
`)
	if _, err := Load(path); err == nil {
		t.Error("expected error for missing invoke.cmd")
	}

	// Invalid uc shape → error.
	path = writeFixture(t, dir, `uc: NOT-A-UC
invoke:
  cmd: echo hi
`)
	if _, err := Load(path); err == nil {
		t.Error("expected error for invalid uc")
	}
}

func TestLoad_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, `uc: UC-150
title: hello
severity: core
setup:
  home: ./fixtures/canary
  env:
    KTW_WORKSPACE: $TMP_HOME
invoke:
  cmd: echo $TMP_HOME
  timeout: 5s
assertions:
  expect_exit: 0
  stdout_contains:
    - hello
  stdout_regex:
    - 'Test\d+'
`)
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.UC != "UC-150" || s.Title != "hello" || s.Severity != "core" {
		t.Errorf("unexpected fields: %+v", s)
	}
	if s.Setup.Home != "./fixtures/canary" {
		t.Errorf("setup.home = %q", s.Setup.Home)
	}
	if want := "go test -count=1"; !strings.Contains(s.Invoke.Cmd, "echo") {
		t.Errorf("invoke.cmd = %q, want to contain echo (got %s)", s.Invoke.Cmd, want)
	}
	if exp := s.Assertions.ExpectExit; exp == nil || *exp != 0 {
		t.Errorf("assertions.expect_exit = %v, want 0", exp)
	}
}

func TestSubstVars(t *testing.T) {
	vars := map[string]string{"TMP_HOME": "/tmp/xyz", "NOW": "2026-05-08"}
	got := substVars("home=$TMP_HOME at $NOW", vars)
	want := "home=/tmp/xyz at 2026-05-08"
	if got != want {
		t.Errorf("substVars = %q, want %q", got, want)
	}
}

func TestApplySubstitutions(t *testing.T) {
	exitZero := 0
	s := &Scenario{
		Setup: Setup{
			Home: "./fixtures/x",
			Env:  map[string]string{"KTW_WORKSPACE": "$TMP_HOME", "OTHER": "literal"},
		},
		Invoke: Invoke{Cmd: "go run ./app/foo --home $TMP_HOME"},
		Assertions: Assertions{
			ExpectExit:     &exitZero,
			StdoutContains: []string{"$TMP_HOME marker", "literal-string"},
		},
	}
	s.applySubstitutions(map[string]string{"TMP_HOME": "/tmp/xyz"})
	if s.Setup.Env["KTW_WORKSPACE"] != "/tmp/xyz" {
		t.Errorf("KTW_WORKSPACE = %q", s.Setup.Env["KTW_WORKSPACE"])
	}
	if s.Setup.Env["OTHER"] != "literal" {
		t.Errorf("OTHER changed unexpectedly: %q", s.Setup.Env["OTHER"])
	}
	if !strings.Contains(s.Invoke.Cmd, "/tmp/xyz") {
		t.Errorf("Cmd substitution missed: %q", s.Invoke.Cmd)
	}
	if s.Assertions.StdoutContains[0] != "/tmp/xyz marker" {
		t.Errorf("StdoutContains[0] = %q", s.Assertions.StdoutContains[0])
	}
}

func TestSplitCmd(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"echo hi", []string{"echo", "hi"}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
		{`echo 'a b c'`, []string{"echo", "a b c"}},
		{"go run ./app/foo --pipe", []string{"go", "run", "./app/foo", "--pipe"}},
		{"  spaced   args  ", []string{"spaced", "args"}},
	}
	for _, c := range cases {
		got, err := splitCmd(c.in)
		if err != nil {
			t.Errorf("splitCmd(%q) err = %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("splitCmd(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCmd(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
	// Unclosed quote → error.
	if _, err := splitCmd(`echo "unclosed`); err == nil {
		t.Error("expected error for unclosed quote")
	}
}

func TestRunner_RunsEchoEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, `uc: UC-001
title: echo canary
invoke:
  cmd: go version
  timeout: 30s
assertions:
  expect_exit: 0
  stdout_contains:
    - go version
`)
	r := &Runner{}
	res, err := r.Run(context.Background(), path)
	if err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if res.Verdict != VerdictPass {
		t.Errorf("Verdict = %v, want pass — failures: %v\nstdout: %s",
			res.Verdict, res.Failures, res.Stdout)
	}
}

func TestRunner_AssertsExitFailure(t *testing.T) {
	dir := t.TempDir()
	// `go version --no-such-flag` returns non-zero exit.
	path := writeFixture(t, dir, `uc: UC-001
title: expected zero but exit nonzero
invoke:
  cmd: go nonexistent-subcommand
  timeout: 30s
assertions:
  expect_exit: 0
`)
	r := &Runner{}
	res, err := r.Run(context.Background(), path)
	if err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if res.Verdict != VerdictFail {
		t.Errorf("Verdict = %v, want fail (exit was %d)", res.Verdict, res.ExitCode)
	}
}

func TestCheckAssertions_AllKinds(t *testing.T) {
	exitZero := 0
	exitOne := 1
	scenario := &Scenario{
		Assertions: Assertions{
			ExpectExit:     &exitZero,
			StdoutContains: []string{"hello"},
			StdoutRegex:    []string{`world\d+`},
			StderrContains: []string{"warn"},
		},
	}

	// All hold.
	res := &Result{ExitCode: 0, Stdout: "hello world42 done", Stderr: "warn: trivial"}
	if got := checkAssertions(scenario, res); len(got) != 0 {
		t.Errorf("expected 0 failures, got %v", got)
	}

	// Exit mismatch → fail.
	res = &Result{ExitCode: 7, Stdout: "hello world42 done", Stderr: "warn: trivial"}
	failures := checkAssertions(scenario, res)
	if len(failures) == 0 || !strings.Contains(failures[0], "expect_exit") {
		t.Errorf("expected expect_exit failure, got %v", failures)
	}

	// stdout_contains miss + stdout_regex miss + stderr_contains miss.
	res = &Result{ExitCode: 0, Stdout: "nope", Stderr: ""}
	failures = checkAssertions(scenario, res)
	if len(failures) != 3 {
		t.Errorf("expected 3 failures, got %d: %v", len(failures), failures)
	}

	// expect_exit nil → ignored even on nonzero.
	scenarioNoExit := &Scenario{
		Assertions: Assertions{StdoutContains: []string{"hi"}},
	}
	res = &Result{ExitCode: 99, Stdout: "hi"}
	if failures := checkAssertions(scenarioNoExit, res); len(failures) != 0 {
		t.Errorf("expected nil expect_exit to be ignored, got %v", failures)
	}

	_ = exitOne // for linter
}
