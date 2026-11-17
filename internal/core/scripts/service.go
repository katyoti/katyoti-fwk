// SPDX-License-Identifier: Apache-2.0

// Package scripts provides script execution with automatic runtime resolution.
//
// Scripts are written to an isolated temp directory and executed via the
// appropriate interpreter (bun for .ts/.js, python for .py, bash for .sh).
// For TypeScript, bun's auto-install resolves npm imports from its global
// cache — no node_modules or package.json needed in the temp dir.
package scripts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunInput describes a script to execute.
type RunInput struct {
	// Name is a human-readable identifier for logging/errors.
	Name string

	// Data is the script content.
	Data []byte

	// Extension is the file extension including the dot (e.g., ".ts", ".py", ".sh").
	Extension string

	// CompanionFiles are additional files written alongside the script in the
	// temp dir before execution. Key is filename, value is content.
	// Use this for package.json or other files the script needs.
	CompanionFiles map[string][]byte

	// InstallDeps runs `bun install` in the temp dir before executing the script.
	// Only meaningful for .ts/.js scripts with a package.json companion file.
	InstallDeps bool

	// Stdin provides input to the script's stdin. May be nil.
	Stdin io.Reader

	// Env is additional environment variables (key=value). Merged with os.Environ().
	Env []string

	// Args are command-line arguments passed after the script path.
	Args []string

	// Timeout overrides the default 5-minute timeout. Zero means default.
	Timeout time.Duration
}

// RunOutput is the result of a script execution.
type RunOutput struct {
	Stdout   string `json:"stdout"           desc:"Execution stdout"`
	Stderr   string `json:"stderr,omitempty" desc:"Execution stderr"`
	ExitCode int    `json:"exitCode"         desc:"Exit code (0 = success)"`
}

// defaultTimeout is 5 minutes.
const defaultTimeout = 5 * time.Minute

// Run executes a script in an isolated temp directory.
//
// The script is written to a temp dir with no node_modules, so bun's
// auto-install resolves npm imports from its global cache. The temp dir
// is cleaned up after execution.
func Run(ctx context.Context, input RunInput) (RunOutput, error) {
	if len(input.Data) == 0 {
		return RunOutput{}, fmt.Errorf("script %q: empty script data", input.Name)
	}

	// Resolve interpreter from extension.
	interpreter, interpreterArgs, err := resolveInterpreter(input.Extension)
	if err != nil {
		return RunOutput{}, fmt.Errorf("script %q: %w", input.Name, err)
	}

	// Write script to an isolated temp directory.
	tmpDir, err := os.MkdirTemp("", "ktw-script-*")
	if err != nil {
		return RunOutput{}, fmt.Errorf("script %q: create temp dir: %w", input.Name, err)
	}
	defer os.RemoveAll(tmpDir)

	filename := sanitizeFilename(input.Name) + input.Extension
	scriptPath := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(scriptPath, input.Data, 0o755); err != nil {
		return RunOutput{}, fmt.Errorf("script %q: write temp file: %w", input.Name, err)
	}

	// Write companion files (e.g., package.json).
	for name, data := range input.CompanionFiles {
		p := filepath.Join(tmpDir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return RunOutput{}, fmt.Errorf("script %q: write companion %q: %w", input.Name, name, err)
		}
	}

	// Install dependencies if requested (bun install in temp dir).
	if input.InstallDeps {
		installCtx, installCancel := context.WithTimeout(ctx, 60*time.Second)
		defer installCancel()
		installCmd := exec.CommandContext(installCtx, interpreter, "install", "--silent")
		installCmd.Dir = tmpDir
		if out, err := installCmd.CombinedOutput(); err != nil {
			return RunOutput{}, fmt.Errorf("script %q: install deps: %w: %s", input.Name, err, string(out))
		}
	}

	// Build command: interpreter [interpreterArgs...] scriptPath [args...]
	cmdArgs := append(interpreterArgs, scriptPath)
	cmdArgs = append(cmdArgs, input.Args...)

	// Set timeout
	timeout := input.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, interpreter, cmdArgs...)
	cmd.Dir = tmpDir // CWD = temp dir (no node_modules → bun auto-install)

	// Environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, input.Env...)

	// I/O
	if input.Stdin != nil {
		cmd.Stdin = input.Stdin
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	runErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return RunOutput{
			Stdout:   stdout.String(),
			Stderr:   "script timed out after " + timeout.String() + "\n" + stderr.String(),
			ExitCode: -1,
		}, nil
	}

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return RunOutput{}, fmt.Errorf("script %q: exec: %w", input.Name, runErr)
		}
	}

	return RunOutput{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

// resolveInterpreter returns the interpreter command and args for a script extension.
func resolveInterpreter(ext string) (string, []string, error) {
	switch strings.ToLower(ext) {
	case ".sh":
		return "bash", nil, nil
	case ".ts":
		// Bun runs TypeScript natively. In an isolated temp dir (no node_modules),
		// bun auto-installs npm imports from its global cache.
		return "bun", []string{"run"}, nil
	case ".js":
		if _, err := exec.LookPath("bun"); err == nil {
			return "bun", []string{"run"}, nil
		}
		return "node", nil, nil
	case ".py":
		if _, err := exec.LookPath("python3"); err == nil {
			return "python3", nil, nil
		}
		if _, err := exec.LookPath("python"); err == nil {
			return "python", nil, nil
		}
		return "", nil, fmt.Errorf("python not found (install python3 or python)")
	case ".go":
		return "go", []string{"run"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported script type %q (supported: .sh, .ts, .js, .py, .go)", ext)
	}
}

// sanitizeFilename removes characters that are invalid in filenames.
func sanitizeFilename(name string) string {
	if name == "" {
		return "script"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
