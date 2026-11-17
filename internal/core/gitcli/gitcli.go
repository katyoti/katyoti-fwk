// SPDX-License-Identifier: Apache-2.0

// Package gitcli runs the git command line: the one helper every package that
// reads or writes a repository goes through (cwm's catch-up, the events code
// source, compose's promote), never a private copy in each.
package gitcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// repoVars are the environment variables that aim git at another repository
// or index: a git hook running KaTyoti sets them, and a command run for a
// given folder must not follow them.
var repoVars = []string{"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE=", "GIT_OBJECT_DIRECTORY=", "GIT_COMMON_DIR=", "GIT_PREFIX="}

// Run runs git in dir (`git -C dir args...`) and answers its standard output,
// untouched: a caller trims what it reads. A failure carries the command and
// git's standard error.
func Run(dir string, args ...string) (string, error) {
	return RunInput(dir, nil, args...)
}

// RunInput is Run with stdin fed to git when it is not nil.
func RunInput(dir string, stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = env()
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// Lines splits an output into its non-empty lines, each trimmed.
func Lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Paths splits a `-z` output (paths separated by NUL, never quoted) into its
// non-empty paths.
func Paths(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env() []string {
	out := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		keep := true
		for _, v := range repoVars {
			if strings.HasPrefix(strings.ToUpper(e), v) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, e)
		}
	}
	return append(out, "GIT_TERMINAL_PROMPT=0")
}
