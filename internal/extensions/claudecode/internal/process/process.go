// SPDX-License-Identifier: Apache-2.0

// Package process starts the claude CLI for a worker: it finds the binary
// (the native claude.exe on Windows, never a .cmd or .bat shim), builds the
// command line the Agent SDKs build, prepares an environment that keeps the
// worker on the person's subscription, and owns the process until it ends.
package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// Spec is what Start needs.
type Spec struct {
	Path string   // the claude binary
	Args []string // without the binary
	Env  []string
	Dir  string
}

// Proc is a started claude process: its pipes and its end.
type Proc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	once sync.Once
	done chan struct{}
	err  error
}

// Start starts the process, hidden console, joined to this process's job so
// it dies with it.
func Start(s Spec) (*Proc, error) {
	cmd := exec.Command(s.Path, s.Args...)
	cmd.Env = s.Env
	cmd.Dir = s.Dir
	publish.NoConsoleWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("claude: stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude: stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("claude: stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude: start %s: %w", s.Path, err)
	}
	_ = publish.AttachToParentJob(cmd.Process)
	p := &Proc{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *Proc) Stdin() io.WriteCloser { return p.stdin }
func (p *Proc) Stdout() io.Reader     { return p.stdout }
func (p *Proc) Stderr() io.Reader     { return p.stderr }

// Done is closed when the process has exited.
func (p *Proc) Done() <-chan struct{} { return p.done }

// ExitCode is the process's exit code once Done; -1 before.
func (p *Proc) ExitCode() int {
	select {
	case <-p.done:
	default:
		return -1
	}
	if p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

// Shutdown ends the process the clean way: end of input, then a terminate,
// then a kill, each given grace to work. The CLI flushes its session file
// after end of input.
func (p *Proc) Shutdown(grace time.Duration) {
	p.once.Do(func() {
		_ = p.stdin.Close()
		if p.wait(grace) {
			return
		}
		if runtime.GOOS != "windows" {
			_ = p.cmd.Process.Signal(os.Interrupt)
			if p.wait(grace) {
				return
			}
		}
		_ = p.cmd.Process.Kill()
		p.wait(grace)
	})
}

func (p *Proc) wait(d time.Duration) bool {
	select {
	case <-p.done:
		return true
	case <-time.After(d):
		return false
	}
}

// ErrNoClaude: no usable claude binary was found.
var ErrNoClaude = errors.New("claude: the Claude Code CLI is not installed (no claude.exe on PATH nor in ~/.local/bin)")

// Find returns the claude binary to run: configured, else on PATH, else the
// native installer's place. A .cmd or .bat is refused: cmd.exe re-parses its
// arguments (CVE-2024-27980, "BatBadBut").
func Find(configured string) (string, error) {
	if configured != "" {
		if err := refuseBatch(configured); err != nil {
			return "", err
		}
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("claude: %s: %w", configured, err)
		}
		return configured, nil
	}
	names := []string{"claude"}
	if runtime.GOOS == "windows" {
		names = []string{"claude.exe"}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil && refuseBatch(p) == nil {
			return p, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".local", "bin", names[0])
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", ErrNoClaude
}

func refuseBatch(path string) error {
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' || r == ':' }) {
		p := strings.ToLower(strings.TrimRight(part, ". "))
		if strings.HasSuffix(p, ".cmd") || strings.HasSuffix(p, ".bat") {
			return fmt.Errorf("claude: %s is a batch file, refused (cmd.exe re-parses its arguments: CVE-2024-27980); use the native claude.exe", path)
		}
	}
	return nil
}

// Options is the command line of a worker.
type Options struct {
	Model              string
	PermissionMode     string
	AllowedTools       []string
	DisallowedTools    []string
	AppendSystemPrompt string
	MaxTurns           int
	MaxBudgetUSD       float64
	Resume             string
	Fork               bool
	SessionID          string
	Name               string
	SettingSources     []string
}

// Args builds the command line the Agent SDKs build: stream-json both ways,
// permissions answered by the host (stdio), never --bare (it ignores the
// subscription sign-in), no --system-prompt (Claude Code keeps its own).
func Args(o Options) ([]string, error) {
	if runtime.GOOS == "windows" {
		for _, v := range []string{o.Resume, o.SessionID} {
			if strings.ContainsAny(v, "&|<>^%!\"\r\n") {
				return nil, fmt.Errorf("claude: refused argument %q (shell metacharacters)", v)
			}
		}
	}
	a := []string{"--output-format", "stream-json", "--verbose"}
	if o.AppendSystemPrompt != "" {
		a = append(a, "--append-system-prompt", o.AppendSystemPrompt)
	}
	if len(o.AllowedTools) > 0 {
		a = append(a, "--allowedTools", strings.Join(o.AllowedTools, ","))
	}
	if len(o.DisallowedTools) > 0 {
		a = append(a, "--disallowedTools", strings.Join(o.DisallowedTools, ","))
	}
	if o.MaxTurns > 0 {
		a = append(a, "--max-turns", strconv.Itoa(o.MaxTurns))
	}
	if o.MaxBudgetUSD > 0 {
		a = append(a, "--max-budget-usd", strconv.FormatFloat(o.MaxBudgetUSD, 'f', -1, 64))
	}
	if o.Model != "" {
		a = append(a, "--model", o.Model)
	}
	a = append(a, "--permission-prompt-tool", "stdio")
	if o.PermissionMode != "" {
		a = append(a, "--permission-mode", o.PermissionMode)
	}
	if o.Resume != "" {
		a = append(a, "--resume="+o.Resume)
		if o.Fork {
			a = append(a, "--fork-session")
		}
	} else if o.SessionID != "" {
		a = append(a, "--session-id="+o.SessionID)
	}
	if o.Name != "" {
		a = append(a, "--name", o.Name)
	}
	if len(o.SettingSources) > 0 {
		a = append(a, "--setting-sources="+strings.Join(o.SettingSources, ","))
	}
	return append(a, "--input-format", "stream-json"), nil
}

// Entrypoint is the CLAUDE_CODE_ENTRYPOINT a worker reports.
const Entrypoint = "sdk-katyoti"

// removed are the variables a worker never inherits: CLAUDECODE (set when the
// parent itself runs under Claude Code) and the API credentials, which Claude
// Code would bill instead of the subscription.
var removed = []string{"CLAUDECODE", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}

// Env builds a worker's environment from base: the API credentials and
// CLAUDECODE removed; the entrypoint and the session-state signal set; on
// Windows, Claude Code's shell named (Git Bash) when gitBash is found.
func Env(base []string, gitBash string) []string {
	out := make([]string, 0, len(base)+4)
	for _, kv := range base {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if isRemoved(k) || strings.EqualFold(k, "CLAUDE_CODE_ENTRYPOINT") || strings.EqualFold(k, "CLAUDE_CODE_SDK_READS_SESSION_STATE") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "CLAUDE_CODE_ENTRYPOINT="+Entrypoint, "CLAUDE_CODE_SDK_READS_SESSION_STATE=1")
	if gitBash != "" && !has(out, "CLAUDE_CODE_GIT_BASH_PATH") {
		out = append(out, "CLAUDE_CODE_GIT_BASH_PATH="+gitBash)
	}
	return out
}

func isRemoved(k string) bool {
	for _, r := range removed {
		if strings.EqualFold(k, r) {
			return true
		}
	}
	return false
}

func has(env []string, key string) bool {
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i >= 0 && strings.EqualFold(kv[:i], key) {
			return true
		}
	}
	return false
}

// GitBash is the Git Bash Claude Code needs on Windows when it is not named:
// configured, else the usual install places; "" when none (and off Windows).
func GitBash(configured string) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	if configured != "" {
		return configured
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Git", "bin", "bash.exe"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
