// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Result holds the outcome of running one scenario.
type Result struct {
	Scenario *Scenario
	Verdict  Verdict

	ExitCode   int
	Stdout     string
	Stderr     string
	Duration   time.Duration
	TmpHome    string         // populated even on failure for `--keep-temp` debugging
	Failures   []string       // human-readable assertion-failure descriptions
	RunErr     error          // process-level error (timeout, build error, etc.)
	Journal    []JournalEvent // captured session events; nil when invoke produced no session dir
	JournalErr error          // non-nil only when journal capture itself failed (e.g. malformed dir)
}

// Verdict is the per-scenario L2 outcome.
type Verdict int

const (
	VerdictUnknown Verdict = iota
	VerdictPass            // 🟢 — every assertion held
	VerdictFail            // 🔴 — at least one assertion failed
	VerdictError           // 🔴 — process-level error before assertions could run
)

func (v Verdict) String() string {
	switch v {
	case VerdictPass:
		return "pass"
	case VerdictFail:
		return "fail"
	case VerdictError:
		return "error"
	}
	return "unknown"
}

// Runner runs `.tasktest` scenarios.
type Runner struct {
	// ProjectRoot is the working directory the invoke `cmd` runs from.
	// Typically the repository root so binaries like `./bin/ktw-agents` resolve.
	ProjectRoot string

	// KeepTemp leaves $TMP_HOME on disk after the run for debugging.
	// CLI flag wires this from `--keep-temp`.
	KeepTemp bool
}

// Run loads, sets up, invokes, asserts, and cleans up one scenario.
func (r *Runner) Run(ctx context.Context, path string) (*Result, error) {
	scenario, err := Load(path)
	if err != nil {
		return nil, err
	}

	tmpHome, err := os.MkdirTemp("", "tasktest-")
	if err != nil {
		return nil, fmt.Errorf("create tmp home: %w", err)
	}
	if !r.KeepTemp {
		defer os.RemoveAll(tmpHome)
	}

	scenarioDir := filepath.Dir(scenario.Path)
	vars := map[string]string{
		"TMP_HOME":     tmpHome,
		"SCENARIO_DIR": scenarioDir,
		"NOW":          time.Now().UTC().Format(time.RFC3339),
	}
	scenario.applySubstitutions(vars)

	if err := r.setup(scenario, scenarioDir, tmpHome); err != nil {
		return &Result{
			Scenario: scenario, Verdict: VerdictError,
			TmpHome: tmpHome, RunErr: err,
		}, nil
	}

	res := r.invoke(ctx, scenario, tmpHome)

	// Capture the session journal (if the invoke produced one). Best-effort:
	// a capture failure does NOT block assertions — it surfaces via
	// res.JournalErr and lets journal-dependent assertions fail with a
	// descriptive message rather than crashing the whole runner.
	if res.Verdict != VerdictError {
		events, err := captureJournal(tmpHome)
		res.Journal = events
		res.JournalErr = err
	}

	res.Failures = checkAssertions(scenario, res)
	if res.Verdict == VerdictUnknown {
		if len(res.Failures) > 0 {
			res.Verdict = VerdictFail
		} else {
			res.Verdict = VerdictPass
		}
	}
	return res, nil
}

// setup materialises the scenario's fixture home (if any) into $TMP_HOME.
func (r *Runner) setup(scenario *Scenario, scenarioDir, tmpHome string) error {
	if scenario.Setup.Home == "" {
		return nil
	}
	src := scenario.Setup.Home
	if !filepath.IsAbs(src) {
		src = filepath.Join(scenarioDir, src)
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("setup.home %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("setup.home %s: not a directory", src)
	}
	return copyTree(src, tmpHome)
}

// invoke runs the scenario's command, capturing stdout/stderr/exit/duration.
// Returns a partially-populated Result; the caller fills Verdict + Failures
// from the assertion engine.
func (r *Runner) invoke(ctx context.Context, scenario *Scenario, tmpHome string) *Result {
	timeout := scenario.Invoke.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args, err := splitCmd(scenario.Invoke.Cmd)
	if err != nil {
		return &Result{
			Scenario: scenario, Verdict: VerdictError,
			TmpHome: tmpHome, RunErr: err,
		}
	}
	if len(args) == 0 {
		return &Result{
			Scenario: scenario, Verdict: VerdictError,
			TmpHome: tmpHome, RunErr: fmt.Errorf("invoke.cmd is empty"),
		}
	}

	cmd := exec.CommandContext(runCtx, resolveCommand(r.ProjectRoot, args[0]), args[1:]...)
	if r.ProjectRoot != "" {
		cmd.Dir = r.ProjectRoot
	}
	// Build env: inherit parent + apply scenario.Setup.Env (with $TMP_HOME
	// already resolved via substVars).
	cmd.Env = os.Environ()
	for k, v := range scenario.Setup.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	start := time.Now()
	stdout, stderr := &captureBuf{}, &captureBuf{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	duration := time.Since(start)

	res := &Result{
		Scenario: scenario,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
		TmpHome:  tmpHome,
	}
	if runCtx.Err() == context.DeadlineExceeded {
		res.Verdict = VerdictError
		res.RunErr = fmt.Errorf("invoke timed out after %s", timeout)
		return res
	}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if runErr != nil && cmd.ProcessState == nil {
		// Couldn't even start the process.
		res.Verdict = VerdictError
		res.RunErr = runErr
	}
	return res
}

// captureBuf is a thin wrapper around strings.Builder so we can satisfy
// io.Writer + return the captured text via String().
type captureBuf struct{ b strings.Builder }

func (c *captureBuf) Write(p []byte) (int, error) {
	return c.b.Write(p)
}
func (c *captureBuf) String() string { return c.b.String() }

// resolveCommand makes a relative command path (`./bin/ktw-agents.exe`)
// absolute against the project root. exec resolves a path-bearing command
// against the CALLER's working directory, not cmd.Dir — on Windows it never
// starts from another directory. A bare name (`go`) is left to PATH; on
// Windows an extension-less path gains `.exe` when that file exists.
func resolveCommand(root, name string) string {
	if root == "" || filepath.IsAbs(name) || !strings.ContainsAny(name, `/\`) {
		return name
	}
	abs, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return name
	}
	if runtime.GOOS == "windows" && filepath.Ext(abs) == "" {
		if _, err := os.Stat(abs + ".exe"); err == nil {
			return abs + ".exe"
		}
	}
	return abs
}

// splitCmd tokenises a command string the way the shell would. Honest about
// limitations: handles quoted args (single + double) but not env interpolation,
// pipes, or backticks. Scenarios should keep `cmd` simple — for complex
// invocations use a wrapper script.
func splitCmd(cmd string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if cur.Len() > 0 {
			args = append(args, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case (c == ' ' || c == '\t') && !inSingle && !inDouble:
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unclosed quote in cmd")
	}
	flush()
	return args, nil
}

// copyTree recursively copies the contents of src into dst (which must
// already exist). Files are copied with their mode preserved on Unix; on
// Windows mode is best-effort.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// suppressed compile-time hint — Windows test fixtures can't always preserve
// exec bits; we keep the runtime check available for future CI matrix work.
var _ = runtime.GOOS
