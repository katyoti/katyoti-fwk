// SPDX-License-Identifier: Apache-2.0

// Package projectcmd runs a shell command in the project the active universe
// serves (application.ProjectRoot): what a request's deliverable check and an
// acceptance criterion's command probe both run. One place, so both run a
// command the same way — through bash on every platform (Git for Windows' bash
// on Windows, see FindBash), so a command is POSIX shell (&&, $(...), pipes)
// wherever it runs; the project as the working folder; a bounded time.
package projectcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// DefaultTimeout bounds a command when the caller names none.
const DefaultTimeout = 10 * time.Minute

// MaxTimeout is the ceiling of a bound a check declares (ParseTimeout): a
// slow check runs to its own bound, never an unbounded one.
const MaxTimeout = time.Hour

// ParseTimeout reads the bound a check declares — an acceptance probe's, a
// deliverable check's — the one place both are read: "" is none (the caller
// keeps its default, ok false); otherwise a Go duration ("30m", "1h") above
// zero and at most MaxTimeout, anything else an error naming why.
func ParseTimeout(s string) (d time.Duration, ok bool, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, nil
	}
	d, err = time.ParseDuration(s)
	switch {
	case err != nil:
		return 0, false, fmt.Errorf("timeout %q is not a duration (30m, 1h): %w", s, err)
	case d <= 0:
		return 0, false, fmt.Errorf("timeout %q is not above zero", s)
	case d > MaxTimeout:
		return 0, false, fmt.Errorf("timeout %q is above the ceiling of %s", s, MaxTimeout)
	}
	return d, true, nil
}

// ErrTimedOut is the error of a command stopped at its bound; Run wraps it
// with the bound in seconds ("timed out after 600s").
var ErrTimedOut = errors.New("timed out")

// waitDelay is how long Run waits, once the command is stopped, for the
// processes it started to let go of its output (on Windows a stopped shell's
// children — go test, a build — keep the pipe open and would hold Run until
// they end).
const waitDelay = 5 * time.Second

// Run runs command through bash in the project root and answers its combined
// output (what it wrote before it was stopped, when it overran); the error is
// the command's (a non-zero exit), ErrTimedOut when it overran timeout, or why
// it could not run. With no bash it runs nothing and answers ErrNoBash, which
// says how to get one (: never another shell).
func Run(ctx context.Context, command string, timeout time.Duration) (string, error) {
	return runEnv(ctx, application.ProjectRoot(), command, timeout, probeEnv(os.Environ()))
}

// hubIdentity are the variables that make a process the hub's own - its
// installation, instance, root, bus, workspace, cache and the keys of its
// .env files. A probe the hub runs (an acceptance criterion, a request's
// check) does not inherit them: the tools and scenario cases it starts resolve
// their own installation, never the hub's (: a live GUI
// check run by the hub drove the dev installation).
var hubIdentity = []string{"KTW_INSTALLATION", "KTW_INSTANCE", "KTW_KATYOTI_ROOT", "KTW_NATS_URL", "KTW_WORKSPACE", "KTW_CACHE", "KTW_DOTENV_KEYS"}

// probeEnv is environ without the hub's identity (hubIdentity).
func probeEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.ContainsFunc(hubIdentity, func(h string) bool { return strings.EqualFold(h, name) }) {
			out = append(out, kv)
		}
	}
	return out
}

// RunIn is Run in the folder dir (a scenario case's project, a test's
// folder).
func RunIn(ctx context.Context, dir, command string, timeout time.Duration) (string, error) {
	return run(ctx, dir, command, timeout)
}

func run(ctx context.Context, dir, command string, timeout time.Duration) (string, error) {
	return runEnv(ctx, dir, command, timeout, nil)
}

// runEnv is run with env as the command's environment (nil: the process's).
func runEnv(ctx context.Context, dir, command string, timeout time.Duration, env []string) (string, error) {
	bash, err := bashFor()
	if err != nil {
		return "", err
	}
	return execEnv(ctx, dir, bash, []string{"-c", command}, timeout, env)
}

// Exec runs a program with its arguments in dir, bounded like Run: stopped at
// timeout with every process it started (a launcher's node, a browser it
// boots), its combined output answered, ErrTimedOut when it overran.
func Exec(ctx context.Context, dir, name string, args []string, timeout time.Duration) (string, error) {
	return execEnv(ctx, dir, name, args, timeout, nil)
}

func execEnv(ctx context.Context, dir, name string, args []string, timeout time.Duration, env []string) (string, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = waitDelay
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	tree := newProcTree(cmd)
	err := cmd.Start()
	if err == nil {
		tree.started(cmd)
		err = cmd.Wait()
	}
	tree.close()
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("%w after %ds", ErrTimedOut, int(timeout.Round(time.Second)/time.Second))
	}
	return out.String(), err
}

// bashFor is the bash a command runs through (FindBash; a test replaces it).
var bashFor = FindBash

// Tail is the end of a command's output, at most max bytes — for a note or an
// evidence line; "" when there is none.
func Tail(out string, max int) string {
	out = strings.TrimSpace(out)
	if len(out) > max {
		out = "…" + out[len(out)-max:]
	}
	return out
}
