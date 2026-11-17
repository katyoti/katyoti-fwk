// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/redact"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// guardCommand refuses a command whose words name a secret path (a cat of
// .env, a key file, the hub's keys folder). The allow-listed environment
// (agentCmd) is what keeps a shell's secrets out of reach; this guard
// refuses the plain reads of a secret file.
func guardCommand(command string) error {
	if p := secrets.CommandSecretPath(command); p != "" {
		return secrets.Refusal("bash", p)
	}
	return nil
}

// agentCmd builds the bash process an agent runs: its environment is the
// allow-list (secrets.AgentEnv), never the whole process environment with
// its connector and API keys.
func agentCmd(bashPath string, args ...string) *exec.Cmd {
	cmd := exec.Command(bashPath, args...)
	cmd.Env = secrets.AgentEnv(os.Environ())
	setProcGroup(cmd)
	return cmd
}

// resultRedactor scrubs secrets (secret env values of this process, known
// credential shapes) from what a command prints before it reaches the
// agent. Settings come from Redaction.yaml when an application serves ctx;
// the zero value is enabled with the built-in catalog.
func resultRedactor(ctx context.Context) *redact.Redactor {
	var settings redact.RedactionSettings
	if ctx != nil {
		if s, err := application.UseConfig[redact.RedactionSettings](ctx); err == nil {
			settings = s
		}
	}
	return redact.New(settings)
}

// msysToWindows converts an MSYS/Git Bash path to a Windows path.
// /c/Users/foo → C:/Users/foo
// /d/Projects  → D:/Projects
// Paths that don't match the pattern are returned unchanged.
func msysToWindows(p string) string {
	if len(p) >= 3 && p[0] == '/' && p[2] == '/' {
		drive := strings.ToUpper(string(p[1]))
		if drive[0] >= 'A' && drive[0] <= 'Z' {
			return drive + ":" + p[2:]
		}
	}
	return p
}

const (
	defaultTimeoutMs = 120_000 // 2 minutes
	maxTimeoutMs     = 600_000 // 10 minutes
	maxOutputChars   = 30_000  // truncation threshold (chars)
	maxOutputBytes   = 512_000 // large-output-to-disk threshold (bytes)
)

// cwdSentinel prefixes the line RunCommand appends to a command's stdout to
// report the post-command working directory. Parsed + stripped by
// splitCwdSentinel so it never reaches the caller's output.
const cwdSentinel = "__KTW_CWD__:"

// splitCwdSentinel separates the trailing cwd-sentinel line from real stdout.
// It returns the cleaned stdout (with the one separator newline we prepended
// removed) and the reported cwd (empty if no sentinel — e.g. a killed process).
func splitCwdSentinel(stdout string) (cleaned, cwd string) {
	idx := strings.LastIndex(stdout, cwdSentinel)
	if idx < 0 {
		return stdout, ""
	}
	cwd = strings.TrimRight(stdout[idx+len(cwdSentinel):], "\r\n")
	cleaned = stdout[:idx]
	// Drop only the single separator newline printf prepended, not legitimate
	// trailing newlines in the command's own output.
	cleaned = strings.TrimSuffix(cleaned, "\n")
	cleaned = strings.TrimSuffix(cleaned, "\r")
	return cleaned, cwd
}

// ExecResult is the raw result of a command execution.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
	NewCwd   string // updated cwd after command (empty if unchanged or background)

	// Background fields
	BackgroundPID  int
	OutputFilePath string

	// Large output fields
	PersistedPath string // non-empty if output was persisted to disk
	PersistedSize int64
}

// cwdState tracks the persistent working directory across calls.
var (
	cwdMu   sync.Mutex
	cwdPath string // empty = not initialized, use workspace root
)

// SetCwd sets the persistent working directory. Called at session init.
func SetCwd(path string) {
	cwdMu.Lock()
	defer cwdMu.Unlock()
	cwdPath = path
}

// GetCwd returns the current persistent working directory.
// If not set, returns empty string (caller should use workspace root).
func GetCwd() string {
	cwdMu.Lock()
	defer cwdMu.Unlock()
	return cwdPath
}

// RunCommand executes a shell command in bash and returns the result.
func RunCommand(ctx context.Context, bashPath, command, cwd string, timeoutMs int) (*ExecResult, error) {
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("bash: command is required")
	}

	if err := guardCommand(command); err != nil {
		return nil, err
	}

	timeout := resolveTimeout(timeoutMs)

	// Validate cwd exists.
	if cwd != "" {
		if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("bash: working directory %q no longer exists — restart from a valid directory", cwd)
		}
	}

	// Build the command: run the user command, then emit the post-command cwd
	// on a STDOUT sentinel line (parsed + stripped below). A temp file was used
	// here before, but its system-temp path is unwritable from Git-Bash on
	// Windows when %TEMP% is an 8.3 short name (e.g. CHRIST~1) — every command
	// then failed on the redirect. The stdout sentinel has no path dependency.
	// `pwd -P` resolves symlinks for a physical path.
	wrappedCommand := command +
		"\n__exit_code=$?\nprintf '\\n" + cwdSentinel + "%s\\n' \"$(pwd -P)\"\nexit $__exit_code"

	cmd := agentCmd(bashPath, "-c", wrappedCommand)

	if cwd != "" {
		cmd.Dir = cwd
	}

	// Use explicit pipes so we can close them on timeout (unblocks reads).
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("bash: stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("bash: stderr pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("bash: start: %w", err)
	}

	// Read stdout/stderr in goroutines.
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(&stdout, stdoutPipe) }()
	go func() { defer wg.Done(); io.Copy(&stderr, stderrPipe) }()

	// Wait for process exit in a goroutine.
	procDone := make(chan error, 1)
	go func() { procDone <- cmd.Wait() }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	exitCode := 0
	timedOut := false

	select {
	case err = <-procDone:
		// Command completed (success or failure).
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				// cmd.Wait() may return an error if the process was killed.
				// Check if it's a "signal: killed" type error.
				exitCode = 1
			}
		}
	case <-timer.C:
		// Timeout — kill the entire process group.
		timedOut = true
		exitCode = -1
		if cmd.Process != nil {
			_ = killProcessGroup(cmd.Process.Pid)
			// Also kill the direct process to close pipes on Windows.
			_ = cmd.Process.Kill()
		}
		<-procDone
	case <-ctx.Done():
		// Parent context cancelled.
		timedOut = true
		exitCode = -1
		if cmd.Process != nil {
			_ = killProcessGroup(cmd.Process.Pid)
			_ = cmd.Process.Kill()
		}
		<-procDone
	}

	// Wait for pipe readers to drain.
	wg.Wait()

	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	if r := resultRedactor(ctx); r != nil {
		stdoutStr = r.RedactString(stdoutStr)
		stderrStr = r.RedactString(stderrStr)
	}

	// Read back the cwd from the stdout sentinel + strip it from the output.
	newCwd := ""
	if !timedOut {
		var cwdRaw string
		stdoutStr, cwdRaw = splitCwdSentinel(stdoutStr)
		if cwdRaw != "" {
			// Convert MSYS path to Windows path: /c/path → C:/path
			if runtime.GOOS == "windows" {
				cwdRaw = msysToWindows(cwdRaw)
			}
			newCwd = cwdRaw
		}
	}

	// Handle output size.
	result := &ExecResult{
		ExitCode: exitCode,
		TimedOut: timedOut,
		NewCwd:   newCwd,
	}

	// Check if output is large enough to persist to disk.
	totalBytes := len(stdoutStr) + len(stderrStr)
	if totalBytes > maxOutputBytes {
		path, size, err := persistOutput(stdoutStr, stderrStr)
		if err == nil {
			result.PersistedPath = path
			result.PersistedSize = size
		}
		// Truncate for the inline result.
		result.Stdout = truncateOutput(stdoutStr, maxOutputChars)
		result.Stderr = truncateOutput(stderrStr, maxOutputChars)
	} else {
		result.Stdout = truncateOutput(stdoutStr, maxOutputChars)
		result.Stderr = truncateOutput(stderrStr, maxOutputChars)
	}

	return result, nil
}

// RunBackground starts a command in the background. Output is redirected
// to a temp file. Returns immediately with PID and output file path.
func RunBackground(bashPath, command, cwd string) (*ExecResult, error) {
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("bash: command is required")
	}
	if err := guardCommand(command); err != nil {
		return nil, err
	}

	// Create temp file for output capture.
	outFile, err := os.CreateTemp("", "bash-bg-output-*")
	if err != nil {
		return nil, fmt.Errorf("bash: create output temp file: %w", err)
	}
	outFilePath := outFile.Name()

	cmd := agentCmd(bashPath, "-c", command)

	if cwd != "" {
		cmd.Dir = cwd
	}

	// Redirect both stdout and stderr to the temp file.
	cmd.Stdout = outFile
	cmd.Stderr = outFile

	if err := cmd.Start(); err != nil {
		outFile.Close()
		os.Remove(outFilePath)
		return nil, fmt.Errorf("bash: start background command: %w", err)
	}

	pid := cmd.Process.Pid

	// Detach: let the process run, close the file when done.
	go func() {
		_ = cmd.Wait()
		outFile.Close()
	}()

	return &ExecResult{
		ExitCode:       0,
		BackgroundPID:  pid,
		OutputFilePath: outFilePath,
	}, nil
}

// resolveTimeout normalizes the timeout value.
func resolveTimeout(ms int) time.Duration {
	if ms <= 0 {
		ms = defaultTimeoutMs
	}
	if ms > maxTimeoutMs {
		ms = maxTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}

// truncateOutput truncates a string to maxChars and appends a notice.
func truncateOutput(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	truncated := s[:maxChars]
	remaining := s[maxChars:]
	omittedLines := strings.Count(remaining, "\n")
	if !strings.HasSuffix(remaining, "\n") {
		omittedLines++ // partial last line
	}
	return truncated + fmt.Sprintf("\n... [%d lines truncated]", omittedLines)
}

// persistOutput writes combined output to a temp file and returns the path.
func persistOutput(stdout, stderr string) (string, int64, error) {
	f, err := os.CreateTemp("", "bash-output-*")
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	var total int64
	if stdout != "" {
		n, _ := io.WriteString(f, stdout)
		total += int64(n)
	}
	if stderr != "" {
		if stdout != "" {
			io.WriteString(f, "\n--- stderr ---\n")
			total += int64(len("\n--- stderr ---\n"))
		}
		n, _ := io.WriteString(f, stderr)
		total += int64(n)
	}
	return f.Name(), total, nil
}

// shellQuote quotes a string for safe inclusion in a bash command.
// Uses single quotes with proper escaping.
func shellQuote(s string) string {
	// On Windows with Git Bash, paths need forward slashes.
	s = filepath.ToSlash(s)
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
