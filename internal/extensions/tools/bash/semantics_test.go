// SPDX-License-Identifier: Apache-2.0

package bash

import "testing"

func TestInterpretResult_GrepNoMatch(t *testing.T) {
	r := InterpretResult("grep foo bar.txt", 1, "", "")
	if r.IsError {
		t.Error("grep exit 1 should not be an error")
	}
	if r.Message != "No matches found" {
		t.Errorf("Message = %q", r.Message)
	}
}

func TestInterpretResult_GrepError(t *testing.T) {
	r := InterpretResult("grep foo bar.txt", 2, "", "")
	if !r.IsError {
		t.Error("grep exit 2 should be an error")
	}
}

func TestInterpretResult_RipgrepNoMatch(t *testing.T) {
	r := InterpretResult("rg pattern .", 1, "", "")
	if r.IsError {
		t.Error("rg exit 1 should not be an error")
	}
}

func TestInterpretResult_DiffDiffers(t *testing.T) {
	r := InterpretResult("diff a.txt b.txt", 1, "", "")
	if r.IsError {
		t.Error("diff exit 1 should not be an error")
	}
	if r.Message != "Files differ" {
		t.Errorf("Message = %q", r.Message)
	}
}

func TestInterpretResult_DiffError(t *testing.T) {
	r := InterpretResult("diff a.txt b.txt", 2, "", "")
	if !r.IsError {
		t.Error("diff exit 2 should be an error")
	}
}

func TestInterpretResult_FindPartial(t *testing.T) {
	r := InterpretResult("find /root -name '*.go'", 1, "", "")
	if r.IsError {
		t.Error("find exit 1 should not be an error")
	}
	if r.Message != "Some directories were inaccessible" {
		t.Errorf("Message = %q", r.Message)
	}
}

func TestInterpretResult_TestFalse(t *testing.T) {
	r := InterpretResult("test -f nofile", 1, "", "")
	if r.IsError {
		t.Error("test exit 1 should not be an error")
	}
	if r.Message != "Condition is false" {
		t.Errorf("Message = %q", r.Message)
	}
}

func TestInterpretResult_UnknownCommand_NonZero(t *testing.T) {
	r := InterpretResult("mycmd --flag", 1, "", "")
	if !r.IsError {
		t.Error("unknown command exit 1 should be an error")
	}
}

func TestInterpretResult_UnknownCommand_Success(t *testing.T) {
	r := InterpretResult("mycmd --flag", 0, "", "")
	if r.IsError {
		t.Error("exit 0 should not be an error")
	}
}

// ── extractBaseCommand ───────────────────────────────────────────────────

func TestExtractBaseCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{"echo hello", "echo"},
		{"grep foo bar.txt", "grep"},
		{"echo foo | grep bar", "grep"},
		{"cd /tmp && ls -la", "ls"},
		{"VAR=1 mycommand arg", "mycommand"},
		{"", ""},
		{"  ls  ", "ls"},
	}
	for _, tt := range tests {
		got := extractBaseCommand(tt.cmd)
		if got != tt.want {
			t.Errorf("extractBaseCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestInterpretResult_Pipeline_GrepLast(t *testing.T) {
	// In a pipeline, the last command determines exit code.
	r := InterpretResult("cat file.txt | grep pattern", 1, "", "")
	if r.IsError {
		t.Error("pipeline ending in grep exit 1 should not be an error")
	}
	if r.Message != "No matches found" {
		t.Errorf("Message = %q", r.Message)
	}
}
