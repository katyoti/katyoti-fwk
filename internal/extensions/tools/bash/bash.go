// SPDX-License-Identifier: Apache-2.0

// Package bash implements the bash agent tool for shell command execution.
//
// Spec: docs/agent_tools/tool_bash_spec.md
// Reference: Claude Code BashTool (C:\DISQUE_D\Projets\claude-code\src\tools\BashTool\)
//
// Key capabilities:
//   - Persistent cwd across calls (tracked externally via pwd -P > tmpfile)
//   - Bash-only execution (no cmd fallback — bash is required on all platforms)
//   - Timeout with process-group kill (SIGKILL on Unix, taskkill /T on Windows)
//   - Background execution with output capture to temp file
//   - Command semantics (grep/diff/find/test exit code interpretation)
//   - Destructive command warnings in permission UI
//   - Output truncation at 30K chars + large-output-to-disk persistence
//
// Out of scope (deferred to P2):
//   - Sandbox enforcement
//   - Sed parsing / simulation
//   - Image output detection
//   - Read-only command classification
//   - Sleep blocking
//   - Progress streaming
package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// BashInput matches the tool's JSON schema.
type BashInput struct {
	// Command is the shell command to execute. Required.
	Command string `json:"command" jsonschema:"The command to execute" required:"true"`

	// Timeout is an optional timeout in milliseconds. Default 120000 (2 min), max 600000 (10 min).
	Timeout *int `json:"timeout,omitempty" jsonschema:"Optional timeout in milliseconds (max 600000)"`

	// Description is a clear, concise description of what this command
	// does. Used in the permission UI and progress display.
	Description string `json:"description,omitempty" jsonschema:"Clear, concise description of what this command does in active voice."`

	// RunInBackground runs the command in the background. The tool
	// returns immediately with a task ID; output can be read later.
	RunInBackground bool `json:"run_in_background,omitempty" jsonschema:"Set to true to run this command in the background."`
}

// BashOutput is the result of command execution.
type BashOutput struct {
	Stdout   string `json:"stdout,omitempty"   jsonschema:"Standard output from the command."`
	Stderr   string `json:"stderr,omitempty"   jsonschema:"Standard error from the command."`
	ExitCode int    `json:"exit_code"          jsonschema:"Exit code (0 = success)."`

	// Interrupted is true if the command was killed (timeout or signal).
	Interrupted bool `json:"interrupted,omitempty" jsonschema:"True if the command was killed due to timeout."`

	// Message is a human-readable status (timeout notice, background info, etc.)
	Message string `json:"message,omitempty" jsonschema:"Human-readable status message."`

	// BackgroundTaskID is set when run_in_background is true.
	BackgroundTaskID string `json:"background_task_id,omitempty" jsonschema:"Background task identifier (PID-based)."`

	// OutputFilePath points to the full output on disk (large results).
	OutputFilePath string `json:"output_file_path,omitempty" jsonschema:"Path to full output on disk when output is large."`

	// OutputFileSize is the total size in bytes of the persisted output.
	OutputFileSize int64 `json:"output_file_size,omitempty" jsonschema:"Total output size in bytes (when persisted)."`

	// ReturnCodeInterpretation provides semantic meaning for non-standard exit codes.
	ReturnCodeInterpretation string `json:"return_code_interpretation,omitempty" jsonschema:"Semantic interpretation of the exit code."`
}

// NewBashTool creates the bash tool.
func NewBashTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"bash",
		toolDescription,
		executeBash,
		toolsapi.WithMutating(),
		toolsapi.WithCancelOnInterrupt(),
		toolsapi.WithDescriber(describeBash),
		toolsapi.WithPermissionCheck(bashPermission),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "🐚", Label: "Bash", Color: "#676E95", Template: "{{.command}}"}),
	)
}

func executeBash(ctx context.Context, in BashInput) (BashOutput, error) {
	// Find bash.
	bashPath, err := FindBash()
	if err != nil {
		return BashOutput{}, err
	}

	// Resolve cwd.
	cwd := GetCwd()

	// Resolve timeout.
	timeoutMs := defaultTimeoutMs
	if in.Timeout != nil {
		timeoutMs = *in.Timeout
	}

	// Background mode.
	if in.RunInBackground {
		result, err := RunBackground(bashPath, in.Command, cwd)
		if err != nil {
			return BashOutput{}, err
		}
		taskID := fmt.Sprintf("bash-%d", result.BackgroundPID)
		return BashOutput{
			ExitCode:         0,
			BackgroundTaskID: taskID,
			Message: fmt.Sprintf(
				"Command started in background (PID %d). Output is being captured to %s.",
				result.BackgroundPID, result.OutputFilePath,
			),
			OutputFilePath: result.OutputFilePath,
		}, nil
	}

	// Foreground execution.
	result, err := RunCommand(ctx, bashPath, in.Command, cwd, timeoutMs)
	if err != nil {
		return BashOutput{}, err
	}

	// Update persistent cwd.
	if result.NewCwd != "" {
		SetCwd(result.NewCwd)
	}

	// Build output.
	out := BashOutput{
		Stdout:      result.Stdout,
		Stderr:      result.Stderr,
		ExitCode:    result.ExitCode,
		Interrupted: result.TimedOut,
	}

	if result.TimedOut {
		out.Message = fmt.Sprintf("Command timed out after %d ms", timeoutMs)
	}

	if result.PersistedPath != "" {
		out.OutputFilePath = result.PersistedPath
		out.OutputFileSize = result.PersistedSize
		out.Message = appendMessage(out.Message,
			fmt.Sprintf("Full output persisted to %s (%d bytes)", result.PersistedPath, result.PersistedSize))
	}

	// Command semantics.
	if result.ExitCode != 0 {
		interp := InterpretResult(in.Command, result.ExitCode, result.Stdout, result.Stderr)
		if interp.Message != "" {
			out.ReturnCodeInterpretation = interp.Message
		}
	}

	return out, nil
}

func describeBash(raw json.RawMessage) string {
	var in BashInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "bash <invalid>"
	}
	if in.Description != "" {
		return in.Description
	}
	cmd := in.Command
	if len(cmd) > 80 {
		cmd = cmd[:80] + "..."
	}
	return "bash: " + cmd
}

// bashPermission implements the permission policy. Bash commands are always
// Ask (they can do anything). Populates Dimensions for rule matching and
// attaches destructive warnings to the message.
func bashPermission(_ context.Context, input json.RawMessage) toolsapi.PermissionResult {
	var in BashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return toolsapi.PermissionResult{
			Behavior: toolsapi.PermissionAsk,
			Message:  "Execute bash command",
		}
	}

	msg := "Execute bash command"
	if in.Description != "" {
		msg = in.Description
	}

	// Attach destructive warning.
	if warning := GetDestructiveWarning(in.Command); warning != "" {
		msg = msg + "\n" + warning
	}

	// Populate dimensions for rule matching.
	dims := map[string]string{
		"command": in.Command,
	}

	// Build suggestions at varying specificity.
	var suggestions []toolsapi.PermissionSuggestion
	baseCmd := extractBaseCommand(in.Command)
	if baseCmd != "" {
		suggestions = append(suggestions,
			toolsapi.PermissionSuggestion{
				Label: fmt.Sprintf("Always allow '%s' commands", baseCmd),
				Rule:  fmt.Sprintf("bash.command:%s *", baseCmd),
			},
		)
	}
	suggestions = append(suggestions,
		toolsapi.PermissionSuggestion{
			Label: "Always allow all bash commands",
			Rule:  "bash:**",
		},
	)

	return toolsapi.PermissionResult{
		Behavior:    toolsapi.PermissionAsk,
		Message:     msg,
		Dimensions:  dims,
		Suggestions: suggestions,
	}
}

// appendMessage combines messages with a separator.
func appendMessage(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}

// InitCwd initializes the persistent cwd to the workspace root.
// Called once at tool/plugin setup.
func InitCwd(workspaceRoot string) {
	abs, err := filepath.Abs(workspaceRoot)
	if err == nil {
		SetCwd(abs)
	}
}
