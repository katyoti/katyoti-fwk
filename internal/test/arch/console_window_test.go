// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// No console window flashes on Windows. A console program
// started by a process that has no console of its own gets a new terminal
// window. Two rules keep every process of the platform from being that parent:
//
//   - a background process is started with a hidden console of its own
//     (publish.Detach / publish.NoConsoleWindow), never with none
//     (DETACHED_PROCESS): what it starts in turn inherits the hidden console;
//   - the GUI is the one process with no console: every program it starts,
//     in its own code or in any package it links, is started with
//     publish.NoConsoleWindow — but those in consoleWindowAllowed.

// consoleWindowAllowed lists the functions of the GUI's code that start a
// program without hiding a console, each with why. "<repo-relative file>:<func>".
var consoleWindowAllowed = map[string]string{
	"app/ktw-gui/app_terminal_windows.go:launchTerminalWindow": "a terminal is a window on purpose",
	"app/ktw-gui/app_terminal_other.go:launchTerminalWindow":   "a terminal is a window on purpose",
	"app/ktw-gui/app_object_files.go:openWithSystem":           "rundll32 / open / xdg-open hand a file to its desktop application: no console",
	"internal/appcore/dirs_cmd.go:openInFileManager":           "explorer / open / xdg-open: a file manager, no console",
}

// detachedProcessRE matches the DETACHED_PROCESS creation flag, by name or value.
var detachedProcessRE = regexp.MustCompile(`DETACHED_PROCESS|(?i:detachedProcess)\b|=\s*0x0*8\b`)

// TestNoProcessIsStartedWithoutAConsole: no Go code sets DETACHED_PROCESS.
func TestNoProcessIsStartedWithoutAConsole(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	for _, top := range []string{"app", "internal", "tools"} {
		_ = filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == "testdata" || n == "frontend" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for i, line := range strings.Split(string(b), "\n") {
				code := line
				if c := strings.Index(code, "//"); c >= 0 {
					code = code[:c]
				}
				if detachedProcessRE.MatchString(code) {
					problems = append(problems, rel(root, p)+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
	}
	if len(problems) > 0 {
		t.Errorf("a process started with no console gives each console program it starts a window; use publish.Detach (a hidden console of its own):\n  %s", strings.Join(problems, "\n  "))
	}
}

// TestTheGUIStartsNoConsoleWindow: every function of the GUI's code and of the
// packages it links that calls exec.Command or exec.CommandContext also calls
// NoConsoleWindow or Detach, unless consoleWindowAllowed lists it.
func TestTheGUIStartsNoConsoleWindow(t *testing.T) {
	root := repoRoot(t)
	// The main module's packages only: a dependency (the module cache, a
	// replaced fork) is not this repository's code, wherever its folder is.
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if .Module}}{{if .Module.Main}}{{.Dir}}{{end}}{{end}}", "./app/ktw-gui")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list the GUI's packages: %v", err)
	}
	var problems []string
	seen := map[string]bool{}
	for _, dir := range strings.Fields(string(out)) {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			for _, fn := range unhiddenStarts(t, f) {
				key := rel(root, f) + ":" + fn
				seen[key] = true
				if _, ok := consoleWindowAllowed[key]; !ok {
					problems = append(problems, key)
				}
			}
		}
	}
	for key := range consoleWindowAllowed {
		if !seen[key] {
			problems = append(problems, "stale allowance: "+key+" (it hides its console now, or is gone: remove it)")
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("the GUI has no console: a program it starts gets a window unless started with publish.NoConsoleWindow (or listed in consoleWindowAllowed, with why):\n  %s", strings.Join(problems, "\n  "))
	}
}

// unhiddenStarts names the functions of file that call exec.Command or
// exec.CommandContext and neither NoConsoleWindow nor Detach.
func unhiddenStarts(t *testing.T, file string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var names []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		starts, hides := false, false
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if x, ok := fun.X.(*ast.Ident); ok && x.Name == "exec" && (fun.Sel.Name == "Command" || fun.Sel.Name == "CommandContext") {
					starts = true
				}
				if fun.Sel.Name == "NoConsoleWindow" || fun.Sel.Name == "Detach" {
					hides = true
				}
			case *ast.Ident:
				if fun.Name == "NoConsoleWindow" || fun.Name == "Detach" {
					hides = true
				}
			}
			return true
		})
		if starts && !hides {
			names = append(names, fd.Name.Name)
		}
	}
	return names
}

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}
