// SPDX-License-Identifier: Apache-2.0

package projectcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// shellWords open a command the platform's shell runs without a program of
// that name: PowerShell's and sh's statements, a variable, a call operator.
var shellWords = map[string]bool{
	"if": true, "for": true, "foreach": true, "while": true, "do": true, "try": true,
	"cd": true, "set": true, "export": true, "test": true, "[": true, "&": true, ".": true,
	"set-location": true, "push-location": true, "exit": true, "false": true,
}

// cannotFail are words that print or succeed whatever follows: a check that
// starts with one cannot fail, so it proves nothing.
var cannotFail = map[string]bool{"echo": true, "write-output": true, "write-host": true, "true": true, ":": true}

// scriptExts are what a project file must end in to be run as a command's
// first word (or none: a script with a shebang).
var scriptExts = map[string]bool{"": true, ".exe": true, ".sh": true, ".ps1": true, ".bat": true, ".cmd": true, ".py": true}

// Runnable says whether command is one Run can run: its first word a program
// on PATH, a script or binary of the project, or a word of the shell itself.
// A prose sentence or a document path ("manual-tests.md §6", a card name) is
// not: the shell would open the document, or fail, and neither proves a thing.
func Runnable(command string) error {
	first := firstWord(strings.TrimSpace(command))
	if first == "" {
		return fmt.Errorf("an empty command")
	}
	lower := strings.ToLower(first)
	if cannotFail[strings.TrimSuffix(lower, ".exe")] {
		return fmt.Errorf("%q succeeds whatever follows: a check made of it cannot fail", first)
	}
	switch {
	case shellWords[lower], strings.HasPrefix(first, "$"), strings.HasPrefix(first, "("):
		return nil
	}
	if _, err := exec.LookPath(first); err == nil && !isDocument(first) {
		return nil
	}
	path := first
	if !filepath.IsAbs(path) {
		path = filepath.Join(application.ProjectRoot(), path)
	}
	if filepath.Ext(path) == "" {
		if _, err := os.Stat(path + ".exe"); err == nil {
			return nil // a project binary named without its extension, as PowerShell runs it
		}
	}
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		if scriptExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		return fmt.Errorf("%q is a document, not a program", first)
	}
	return fmt.Errorf("%q is no program on PATH, no script of the project and no shell word", first)
}

// firstWord is a command's first word: up to the first space, or a quoted
// one whole (a path with spaces).
func firstWord(command string) string {
	if command == "" {
		return ""
	}
	if q := command[0]; q == '"' || q == '\'' {
		if end := strings.IndexByte(command[1:], q); end >= 0 {
			return command[1 : 1+end]
		}
	}
	if i := strings.IndexAny(command, " \t"); i >= 0 {
		return command[:i]
	}
	return command
}

// isDocument reports a first word naming a document rather than a program.
func isDocument(word string) bool {
	return !scriptExts[strings.ToLower(filepath.Ext(word))]
}
