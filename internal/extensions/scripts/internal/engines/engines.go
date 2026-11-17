// SPDX-License-Identifier: Apache-2.0

// Package engines provides the built-in script execution engines.
package engines

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
)

// baseEngine implements the common execution logic shared by all engines.
// Concrete engines only need to provide the interpreter command and args.
type baseEngine struct {
	name        string
	extensions  []string
	interpreter string   // e.g., "bun", "python3", "bash"
	interpArgs  []string // e.g., ["run"] for bun
}

func (e *baseEngine) Name() string         { return e.name }
func (e *baseEngine) Extensions() []string { return e.extensions }

func (e *baseEngine) Run(ctx context.Context, in api.ScriptExecInput) (api.ScriptRunOutput, error) {
	if len(in.Data) == 0 {
		return api.ScriptRunOutput{}, fmt.Errorf("script %q: empty script data", in.Name)
	}

	// Write script to isolated temp dir
	tmpDir, err := os.MkdirTemp("", "ktw-script-*")
	if err != nil {
		return api.ScriptRunOutput{}, fmt.Errorf("script %q: create temp dir: %w", in.Name, err)
	}
	defer os.RemoveAll(tmpDir)

	filename := sanitizeFilename(in.Name) + in.Extension
	scriptPath := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(scriptPath, in.Data, 0o755); err != nil {
		return api.ScriptRunOutput{}, fmt.Errorf("script %q: write temp file: %w", in.Name, err)
	}

	// Write companion files
	for name, data := range in.CompanionFiles {
		p := filepath.Join(tmpDir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return api.ScriptRunOutput{}, fmt.Errorf("script %q: write companion %q: %w", in.Name, name, err)
		}
	}

	// Install deps if requested
	if in.InstallDeps {
		if err := e.installDeps(ctx, tmpDir); err != nil {
			return api.ScriptRunOutput{}, fmt.Errorf("script %q: %w", in.Name, err)
		}
	}

	// Build command
	cmdArgs := append(e.interpArgs, scriptPath)
	cmdArgs = append(cmdArgs, in.Args...)

	timeout := in.Timeout
	if timeout == 0 {
		timeout = api.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.interpreter, cmdArgs...)
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, in.Env...)

	if in.Stdin != nil {
		cmd.Stdin = in.Stdin
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return api.ScriptRunOutput{
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
			return api.ScriptRunOutput{}, fmt.Errorf("script %q: exec: %w", in.Name, runErr)
		}
	}

	return api.ScriptRunOutput{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

func (e *baseEngine) installDeps(ctx context.Context, dir string) error {
	installCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(installCtx, e.interpreter, "install", "--silent")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install deps: %w: %s", err, string(out))
	}
	return nil
}

// --- Built-in engines ---

// NewBunEngine creates the bun engine for .ts and .js files.
func NewBunEngine() api.ScriptEngine {
	return &baseEngine{name: "bun", extensions: []string{".ts", ".js"}, interpreter: "bun", interpArgs: []string{"run"}}
}

// NewPythonEngine creates the python engine for .py files.
// Resolves python3 or python on PATH.
func NewPythonEngine() api.ScriptEngine {
	interp := "python3"
	if _, err := exec.LookPath("python3"); err != nil {
		interp = "python"
	}
	return &baseEngine{name: "python", extensions: []string{".py"}, interpreter: interp}
}

// NewBashEngine creates the bash engine for .sh files.
func NewBashEngine() api.ScriptEngine {
	return &baseEngine{name: "bash", extensions: []string{".sh"}, interpreter: "bash"}
}

// NewGoEngine creates the go engine for .go files.
func NewGoEngine() api.ScriptEngine {
	return &baseEngine{name: "go", extensions: []string{".go"}, interpreter: "go", interpArgs: []string{"run"}}
}

// BuiltinEngines returns all built-in script engines.
func BuiltinEngines() []api.ScriptEngine {
	return []api.ScriptEngine{NewBunEngine(), NewPythonEngine(), NewBashEngine(), NewGoEngine()}
}

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
