// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveIONoFlags(t *testing.T) {
	cfg, err := ResolveIO("", "", false, false, false, false, "", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InputMode != InputNone {
		t.Errorf("InputMode = %v, want InputNone", cfg.InputMode)
	}
	if cfg.OutputMode != OutputStdout {
		t.Errorf("OutputMode = %v, want OutputStdout", cfg.OutputMode)
	}
	if cfg.Raw {
		t.Error("Raw should be false")
	}
	if cfg.IsPipe {
		t.Error("IsPipe should be false")
	}
}

func TestResolveIOPipe(t *testing.T) {
	cfg, err := ResolveIO("", "", false, false, false, false, "", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.IsPipe {
		t.Error("IsPipe should be true")
	}
	if !cfg.Raw {
		t.Error("Raw should be true in pipe mode")
	}
}

func TestResolveIOInputFile(t *testing.T) {
	// Create a temp file with content
	dir := t.TempDir()
	path := filepath.Join(dir, "input.md")
	os.WriteFile(path, []byte("# Hello\nWorld"), 0o644)

	cfg, err := ResolveIO(path, "", false, false, false, false, "", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InputMode != InputFile {
		t.Errorf("InputMode = %v, want InputFile", cfg.InputMode)
	}
	if cfg.InputData != "# Hello\nWorld" {
		t.Errorf("InputData = %q, want %q", cfg.InputData, "# Hello\nWorld")
	}
	if cfg.TaskInput["input_file"] != "input.md" {
		t.Errorf("TaskInput[input_file] = %v, want %q", cfg.TaskInput["input_file"], "input.md")
	}
	if cfg.TaskInput["input_content"] != "# Hello\nWorld" {
		t.Errorf("TaskInput[input_content] mismatch")
	}
}

func TestResolveIOOutputFile(t *testing.T) {
	cfg, err := ResolveIO("", "", false, false, false, true, "/tmp/out.txt", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OutputMode != OutputFile {
		t.Errorf("OutputMode = %v, want OutputFile", cfg.OutputMode)
	}
	if cfg.OutputFile != "/tmp/out.txt" {
		t.Errorf("OutputFile = %q, want %q", cfg.OutputFile, "/tmp/out.txt")
	}
}

func TestResolveIOMutuallyExclusiveInput(t *testing.T) {
	_, err := ResolveIO("file.txt", "", true, false, false, false, "", false, false)
	if err == nil {
		t.Error("expected error for mutually exclusive --if and --ic")
	}
}

func TestResolveIOContextInlineJSON(t *testing.T) {
	cfg, err := ResolveIO("", `{"format":"brief","count":3}`, false, false, false, false, "", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TaskInput["format"] != "brief" {
		t.Errorf("TaskInput[format] = %v, want %q", cfg.TaskInput["format"], "brief")
	}
	// JSON numbers unmarshal as float64
	if cfg.TaskInput["count"] != float64(3) {
		t.Errorf("TaskInput[count] = %v, want 3", cfg.TaskInput["count"])
	}
}

func TestResolveIOContextJSONFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ctx.json")
	os.WriteFile(path, []byte(`{"project":"ktw-agents","version":1}`), 0o644)

	cfg, err := ResolveIO("", path, false, false, false, false, "", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TaskInput["project"] != "ktw-agents" {
		t.Errorf("TaskInput[project] = %v, want %q", cfg.TaskInput["project"], "ktw-agents")
	}
}

func TestResolveIOContextYAMLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ctx.yaml")
	os.WriteFile(path, []byte("project: ktw-agents\nversion: 1\n"), 0o644)

	cfg, err := ResolveIO("", path, false, false, false, false, "", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TaskInput["project"] != "ktw-agents" {
		t.Errorf("TaskInput[project] = %v, want %q", cfg.TaskInput["project"], "ktw-agents")
	}
}

func TestResolveIOContextMergedWithInput(t *testing.T) {
	// Input file + context should merge
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "data.md")
	os.WriteFile(inputPath, []byte("some content"), 0o644)

	cfg, err := ResolveIO(inputPath, `{"extra":"info"}`, false, false, false, false, "", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Input metadata should be present
	if cfg.TaskInput["input_file"] != "data.md" {
		t.Errorf("TaskInput[input_file] = %v, want %q", cfg.TaskInput["input_file"], "data.md")
	}
	// User context should be merged
	if cfg.TaskInput["extra"] != "info" {
		t.Errorf("TaskInput[extra] = %v, want %q", cfg.TaskInput["extra"], "info")
	}
}

func TestWriteOutputFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	cfg := &IOConfig{
		OutputMode: OutputFile,
		OutputFile: path,
	}

	err := cfg.WriteOutput("hello world")
	if err != nil {
		t.Fatalf("WriteOutput error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("output = %q, want %q", string(data), "hello world")
	}
}
