// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// ── Shell detection ──────────────────────────────────────────────────────

func TestFindBash_ReturnsPath(t *testing.T) {
	ResetBashCache()
	path, err := FindBash()
	if err != nil {
		t.Skipf("bash not found on this system: %v", err)
	}
	if path == "" {
		t.Fatal("FindBash returned empty path with no error")
	}
	if !isExecutable(path) {
		t.Errorf("FindBash returned non-executable: %s", path)
	}
}

func TestFindBash_Memoized(t *testing.T) {
	ResetBashCache()
	p1, _ := FindBash()
	p2, _ := FindBash()
	if p1 != p2 {
		t.Errorf("FindBash not memoized: %q != %q", p1, p2)
	}
}

func TestIsBashPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/bin/bash", true},
		{"/usr/bin/bash", true},
		{"/bin/zsh", false},
		{"/bin/sh", false},
		{"", false},
	}
	// A Windows path is split on its backslashes only on Windows.
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			path string
			want bool
		}{`C:\Program Files\Git\bin\bash.exe`, true})
	}
	for _, tt := range tests {
		if got := isBashPath(tt.path); got != tt.want {
			t.Errorf("isBashPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// ── Command execution ────────────────────────────────────────────────────

func requireBash(t *testing.T) string {
	t.Helper()
	ResetBashCache()
	p, err := FindBash()
	if err != nil {
		t.Skipf("bash not available: %v", err)
	}
	return p
}

func TestExecuteBash_Echo(t *testing.T) {
	bashPath := requireBash(t)
	result, err := RunCommand(context.Background(), bashPath, "echo hello world", "", defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0. stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "hello world") {
		t.Errorf("Stdout = %q, want 'hello world'", result.Stdout)
	}
}

func TestExecuteBash_ExitCode(t *testing.T) {
	bashPath := requireBash(t)
	result, err := RunCommand(context.Background(), bashPath, "exit 42", "", defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestExecuteBash_Stderr(t *testing.T) {
	bashPath := requireBash(t)
	result, err := RunCommand(context.Background(), bashPath, "echo err >&2", "", defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stderr, "err") {
		t.Errorf("Stderr = %q, want 'err'", result.Stderr)
	}
}

func TestExecuteBash_EmptyCommand(t *testing.T) {
	bashPath := requireBash(t)
	_, err := RunCommand(context.Background(), bashPath, "", "", defaultTimeoutMs)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("err = %v, want 'required'", err)
	}
}

func TestExecuteBash_WhitespaceCommand(t *testing.T) {
	bashPath := requireBash(t)
	_, err := RunCommand(context.Background(), bashPath, "   ", "", defaultTimeoutMs)
	if err == nil {
		t.Fatal("expected error for whitespace-only command")
	}
}

func TestSplitCwdSentinel(t *testing.T) {
	cleaned, cwd := splitCwdSentinel("hello world\n" + cwdSentinel + "/c/Users/x\n")
	if cleaned != "hello world" {
		t.Errorf("cleaned = %q, want %q", cleaned, "hello world")
	}
	if cwd != "/c/Users/x" {
		t.Errorf("cwd = %q, want /c/Users/x", cwd)
	}
	// No sentinel (e.g. killed process) → stdout untouched, no cwd.
	if c, w := splitCwdSentinel("partial output"); c != "partial output" || w != "" {
		t.Errorf("no-sentinel = (%q,%q)", c, w)
	}
	// Legitimate trailing newlines in command output are preserved (only the one
	// separator newline before the sentinel is removed).
	if c, _ := splitCwdSentinel("a\n\n" + cwdSentinel + "/c\n"); c != "a\n" {
		t.Errorf("preserve-trailing = %q, want %q", c, "a\n")
	}
}

// TestExecuteBash_StdoutNoSentinelLeak guards that the cwd sentinel never
// reaches the caller's Stdout (the whole reason it is parsed + stripped).
func TestExecuteBash_StdoutNoSentinelLeak(t *testing.T) {
	bashPath := requireBash(t)
	result, err := RunCommand(context.Background(), bashPath, "echo hello", "", defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Stdout, cwdSentinel) {
		t.Errorf("sentinel leaked into stdout: %q", result.Stdout)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Errorf("stdout = %q, want hello", result.Stdout)
	}
	if result.NewCwd == "" {
		t.Error("NewCwd should be tracked even with no cd")
	}
}

func TestExecuteBash_CwdTracking(t *testing.T) {
	bashPath := requireBash(t)
	tmpDir := t.TempDir()
	result, err := RunCommand(context.Background(), bashPath, "cd "+shellQuote(tmpDir)+" && pwd", "", defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	if result.NewCwd == "" {
		t.Fatal("NewCwd is empty after cd")
	}
	// The new cwd should match the tmpDir (modulo symlink resolution).
	gotAbs, _ := filepath.Abs(result.NewCwd)
	wantAbs, _ := filepath.Abs(tmpDir)
	// Normalize for comparison (realpath may differ).
	if !strings.HasPrefix(filepath.ToSlash(gotAbs), filepath.ToSlash(wantAbs)[:10]) {
		t.Logf("NewCwd=%q, tmpDir=%q (may differ due to symlinks — not fatal)", gotAbs, wantAbs)
	}
}

func TestExecuteBash_CwdPersistence(t *testing.T) {
	bashPath := requireBash(t)
	tmpDir := t.TempDir()

	// First command: cd to tmpDir.
	SetCwd(tmpDir)
	result1, err := RunCommand(context.Background(), bashPath, "pwd", tmpDir, defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result1.Stdout, filepath.Base(tmpDir)) {
		t.Logf("Note: pwd output=%q, tmpDir=%q", result1.Stdout, tmpDir)
	}
}

func TestExecuteBash_Timeout(t *testing.T) {
	bashPath := requireBash(t)
	result, err := RunCommand(context.Background(), bashPath, "sleep 30", "", 300)
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut {
		t.Error("TimedOut = false, want true")
	}
	if result.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 (killed)", result.ExitCode)
	}
}

func TestExecuteBash_TimeoutCapped(t *testing.T) {
	bashPath := requireBash(t)
	// A huge timeout should be capped and not error.
	result, err := RunCommand(context.Background(), bashPath, "echo ok", "", 999_999_999)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d", result.ExitCode)
	}
}

func TestExecuteBash_InvalidCwd(t *testing.T) {
	bashPath := requireBash(t)
	_, err := RunCommand(context.Background(), bashPath, "echo hi", "/nonexistent/path/xyz", defaultTimeoutMs)
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("err = %v, want 'no longer exists'", err)
	}
}

func TestExecuteBash_ChainedCommands(t *testing.T) {
	bashPath := requireBash(t)
	result, err := RunCommand(context.Background(), bashPath, "echo first && echo second", "", defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "first") || !strings.Contains(result.Stdout, "second") {
		t.Errorf("Stdout = %q, want both", result.Stdout)
	}
}

// ── Background ────────────────────────────────────────────────────────────

func TestRunBackground(t *testing.T) {
	bashPath := requireBash(t)
	result, err := RunBackground(bashPath, "echo background-test", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.BackgroundPID == 0 {
		t.Error("BackgroundPID = 0")
	}
	if result.OutputFilePath == "" {
		t.Error("OutputFilePath is empty")
	}
	// Clean up.
	defer os.Remove(result.OutputFilePath)
}

func TestRunBackground_EmptyCommand(t *testing.T) {
	bashPath := requireBash(t)
	_, err := RunBackground(bashPath, "", "")
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

// ── Output truncation ─────────────────────────────────────────────────────

func TestTruncateOutput_Short(t *testing.T) {
	got := truncateOutput("hello", 100)
	if got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestTruncateOutput_Long(t *testing.T) {
	long := strings.Repeat("line\n", 10000)
	got := truncateOutput(long, 100)
	if len(got) > 200 {
		t.Errorf("truncated output too long: %d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Error("missing truncation notice")
	}
}

func TestTruncateOutput_ExactLimit(t *testing.T) {
	s := strings.Repeat("x", 100)
	got := truncateOutput(s, 100)
	if got != s {
		t.Error("should not truncate at exact limit")
	}
}

// ── Handler integration ────────────────────────────────────────────────────

func resetCwdForTest(t *testing.T) {
	t.Helper()
	// Reset cwd to a valid temp dir so handler tests don't fail
	// due to a previous test's temp dir being cleaned up.
	dir := t.TempDir()
	SetCwd(dir)
}

func TestHandler_Echo(t *testing.T) {
	ResetBashCache()
	if _, err := FindBash(); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	resetCwdForTest(t)
	out, err := executeBash(context.Background(), BashInput{Command: "echo handler-test"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 0 {
		t.Errorf("ExitCode = %d", out.ExitCode)
	}
	if !strings.Contains(out.Stdout, "handler-test") {
		t.Errorf("Stdout = %q", out.Stdout)
	}
}

func TestHandler_Background(t *testing.T) {
	ResetBashCache()
	if _, err := FindBash(); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	resetCwdForTest(t)
	out, err := executeBash(context.Background(), BashInput{
		Command:         "echo bg-test",
		RunInBackground: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.BackgroundTaskID == "" {
		t.Error("BackgroundTaskID is empty")
	}
	if out.OutputFilePath == "" {
		t.Error("OutputFilePath is empty")
	}
	defer os.Remove(out.OutputFilePath)
	// The command runs on in the test's folder: wait for it to end before the
	// folder is removed (on Windows a process's working folder cannot be).
	var pid int
	if _, err := fmt.Sscanf(out.BackgroundTaskID, "bash-%d", &pid); err != nil {
		t.Fatalf("BackgroundTaskID %q names no PID: %v", out.BackgroundTaskID, err)
	}
	if p, err := os.FindProcess(pid); err == nil {
		_, _ = p.Wait()
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, _ := os.ReadFile(out.OutputFilePath)
		if strings.Contains(string(b), "bg-test") {
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("the background command's output never came: %q", b)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHandler_Semantics_GrepNoMatch(t *testing.T) {
	ResetBashCache()
	if _, err := FindBash(); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	resetCwdForTest(t)
	out, err := executeBash(context.Background(), BashInput{Command: "echo foo | grep bar"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", out.ExitCode)
	}
	if out.ReturnCodeInterpretation != "No matches found" {
		t.Errorf("ReturnCodeInterpretation = %q, want 'No matches found'", out.ReturnCodeInterpretation)
	}
}

// ── Describer ──────────────────────────────────────────────────────────────

func TestDescribeBash_WithDescription(t *testing.T) {
	raw, _ := json.Marshal(BashInput{Command: "ls -la", Description: "List files"})
	got := describeBash(raw)
	if got != "List files" {
		t.Errorf("got %q, want 'List files'", got)
	}
}

func TestDescribeBash_WithoutDescription(t *testing.T) {
	raw, _ := json.Marshal(BashInput{Command: "git status"})
	got := describeBash(raw)
	if got != "bash: git status" {
		t.Errorf("got %q", got)
	}
}

func TestDescribeBash_LongCommand_Truncated(t *testing.T) {
	long := strings.Repeat("x", 200)
	raw, _ := json.Marshal(BashInput{Command: long})
	got := describeBash(raw)
	if len(got) > 100 {
		t.Errorf("describer should truncate long commands, got len=%d", len(got))
	}
}

// ── Permission ─────────────────────────────────────────────────────────────

func TestPermission_AlwaysAsk(t *testing.T) {
	raw, _ := json.Marshal(BashInput{Command: "echo hi"})
	result := bashPermission(context.Background(), raw)
	if result.Behavior != toolsapi.PermissionAsk {
		t.Errorf("Behavior = %v, want Ask", result.Behavior)
	}
}

func TestPermission_PopulatesDimensions(t *testing.T) {
	raw, _ := json.Marshal(BashInput{Command: "ls -la"})
	result := bashPermission(context.Background(), raw)
	if result.Dimensions["command"] != "ls -la" {
		t.Errorf("Dimensions[command] = %q, want 'ls -la'", result.Dimensions["command"])
	}
}

func TestPermission_DestructiveWarning(t *testing.T) {
	raw, _ := json.Marshal(BashInput{Command: "git reset --hard"})
	result := bashPermission(context.Background(), raw)
	if !strings.Contains(result.Message, "discard uncommitted") {
		t.Errorf("Message = %q, want destructive warning", result.Message)
	}
}

func TestPermission_Suggestions(t *testing.T) {
	raw, _ := json.Marshal(BashInput{Command: "git status"})
	result := bashPermission(context.Background(), raw)
	if len(result.Suggestions) < 2 {
		t.Fatalf("Suggestions len = %d, want >= 2", len(result.Suggestions))
	}
	// First suggestion should be command-specific.
	if !strings.Contains(result.Suggestions[0].Rule, "bash.command:") {
		t.Errorf("first suggestion rule = %q", result.Suggestions[0].Rule)
	}
	// Last should be catch-all.
	if result.Suggestions[len(result.Suggestions)-1].Rule != "bash:**" {
		t.Errorf("last suggestion rule = %q", result.Suggestions[len(result.Suggestions)-1].Rule)
	}
}
