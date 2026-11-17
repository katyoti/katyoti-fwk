// SPDX-License-Identifier: Apache-2.0

package process

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSubscriptionGuardEnvRemovesAPIKeys: a worker never inherits an API
// credential (Claude Code would bill it instead of the subscription) nor
// CLAUDECODE; it reports its entrypoint and asks for the session-state signal.
func TestSubscriptionGuardEnvRemovesAPIKeys(t *testing.T) {
	base := []string{"PATH=/bin", "ANTHROPIC_API_KEY=sk-ant-x", "anthropic_auth_token=t", "CLAUDECODE=1",
		"CLAUDE_CODE_ENTRYPOINT=cli", "HOME=/h"}
	env := Env(base, "")
	joined := strings.Join(env, "\n")
	for _, gone := range []string{"ANTHROPIC_API_KEY", "anthropic_auth_token", "CLAUDECODE=", "CLAUDE_CODE_ENTRYPOINT=cli"} {
		if strings.Contains(joined, gone) {
			t.Errorf("env keeps %s:\n%s", gone, joined)
		}
	}
	for _, want := range []string{"PATH=/bin", "HOME=/h", "CLAUDE_CODE_ENTRYPOINT=" + Entrypoint, "CLAUDE_CODE_SDK_READS_SESSION_STATE=1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("env lacks %s:\n%s", want, joined)
		}
	}
	if e := Env([]string{"A=1"}, `C:\Git\bin\bash.exe`); !strings.Contains(strings.Join(e, "\n"), `CLAUDE_CODE_GIT_BASH_PATH=C:\Git\bin\bash.exe`) {
		t.Errorf("Git Bash not named: %v", e)
	}
	if e := Env([]string{"CLAUDE_CODE_GIT_BASH_PATH=X"}, "Y"); strings.Count(strings.Join(e, "\n"), "CLAUDE_CODE_GIT_BASH_PATH") != 1 {
		t.Errorf("a named Git Bash is overridden: %v", e)
	}
}

// TestArgs: the command line the Agent SDKs build - stream-json both ways,
// the host answers permissions, the equals form for ids, never --bare nor
// --system-prompt.
func TestArgs(t *testing.T) {
	a, err := Args(Options{Model: "opus", PermissionMode: "auto", AllowedTools: []string{"Read", "Bash(go test *)"},
		MaxTurns: 7, MaxBudgetUSD: 1.5, Resume: "abc", Fork: true, Name: "w1", SettingSources: []string{"user", "project", "local"},
		AppendSystemPrompt: "be brief"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(a, " ")
	for _, want := range []string{"--output-format stream-json --verbose", "--permission-prompt-tool stdio", "--permission-mode auto",
		"--model opus", "--allowedTools Read,Bash(go test *)", "--max-turns 7", "--max-budget-usd 1.5", "--resume=abc --fork-session",
		"--name w1", "--setting-sources=user,project,local", "--append-system-prompt be brief"} {
		if !strings.Contains(got, want) {
			t.Errorf("args lack %q: %s", want, got)
		}
	}
	if a[len(a)-2] != "--input-format" || a[len(a)-1] != "stream-json" {
		t.Errorf("args do not end with --input-format stream-json: %s", got)
	}
	for _, never := range []string{"--bare", "--system-prompt ", "-p", "--print"} {
		for _, x := range a {
			if x == strings.TrimSpace(never) {
				t.Errorf("args carry %s: %s", never, got)
			}
		}
	}
	b, _ := Args(Options{SessionID: "11111111-2222-3333-4444-555555555555"})
	if !strings.Contains(strings.Join(b, " "), "--session-id=11111111-2222-3333-4444-555555555555") {
		t.Errorf("session id: %v", b)
	}
	if runtime.GOOS == "windows" {
		if _, err := Args(Options{Resume: "x & calc"}); err == nil {
			t.Errorf("a resume id with shell metacharacters was accepted")
		}
	}
}

// TestFindRefusesBatchFiles: a .cmd or .bat claude is refused (cmd.exe
// re-parses its arguments); a configured binary is used as given.
func TestFindRefusesBatchFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"claude.cmd", "claude.BAT", "claude.cmd. "} {
		p := filepath.Join(dir, strings.TrimRight(name, ". "))
		_ = os.WriteFile(p, []byte("@echo off"), 0o755)
		if _, err := Find(filepath.Join(dir, name)); err == nil || !strings.Contains(err.Error(), "batch") {
			t.Errorf("Find(%s) = %v, want a batch refusal", name, err)
		}
	}
	exe := filepath.Join(dir, "claude.exe")
	_ = os.WriteFile(exe, []byte("MZ"), 0o755)
	if p, err := Find(exe); err != nil || p != exe {
		t.Errorf("Find(%s) = %q, %v", exe, p, err)
	}
	if _, err := Find(filepath.Join(dir, "missing.exe")); err == nil {
		t.Errorf("a missing configured binary was accepted")
	}
}
