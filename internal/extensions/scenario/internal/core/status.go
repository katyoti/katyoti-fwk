// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Summary is a case's state in one line.
func Summary(e *Engine) string {
	st, _ := loadState(e.RT)
	if !st.Installed {
		return "not installed"
	}
	s := fmt.Sprintf("%d/%d stages passed", len(st.Passed), len(e.Case.Stages))
	if st.Failure != "" {
		s += " — failed " + st.Failure
	}
	return s
}

// PrintStatus prints where a case stands.
func PrintStatus(ctx context.Context, e *Engine, w io.Writer) error {
	st, err := loadState(e.RT)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "case       %s — %s\n", e.Case.Name, e.Case.Title)
	fmt.Fprintf(w, "runtime    %s\n", e.RT.Dir)
	fmt.Fprintf(w, "universe   %s (%s)\n", e.Case.UniverseName(), map[bool]string{true: "running", false: "stopped"}[e.Running(ctx)])
	if !st.Installed {
		fmt.Fprintf(w, "state      not installed — ktw-compose scenario install %s\n", e.Case.Name)
		return nil
	}
	fmt.Fprintln(w, "stages")
	for _, s := range e.Case.Stages {
		mark := "  "
		for _, p := range st.Passed {
			if p == s.ID {
				mark = "✓ "
			}
		}
		if strings.HasPrefix(st.Failure, s.ID+":") {
			mark = "✗ "
		}
		fmt.Fprintf(w, "  %s%-14s %s\n", mark, s.ID, s.Title)
	}
	if st.Failure != "" {
		fmt.Fprintf(w, "failure    %s\n", st.Failure)
	}
	fmt.Fprintf(w, "checkpoints %s\n", strings.Join(e.Checkpoints(), ", "))
	if r := lastReport(e); r != "" {
		fmt.Fprintf(w, "last report %s\n", r)
	}
	return nil
}

func lastReport(e *Engine) string {
	entries, _ := os.ReadDir(e.RT.Runs())
	var names []string
	for _, d := range entries {
		if strings.HasSuffix(d.Name(), ".md") {
			names = append(names, d.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return filepath.Join(e.RT.Runs(), names[len(names)-1])
}

// CallOnce calls a method or runs an action on the running universe.
func CallOnce(ctx context.Context, e *Engine, uri string, input any, action bool) (json.RawMessage, error) {
	b, err := e.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	return b.CallAny(ctx, uri, input, action)
}

// GUIHelp says how to look at a case in the GUI.
func GUIHelp(e *Engine) string {
	return fmt.Sprintf(`The case's universe is %[1]s (its project: %[2]s).

A person double-clicks the launcher in the case's folder (%[4]s): it starts the
universe as the scenario runs it (its own hub and members from the case's bin) and
opens ktw-gui on it; the GUI joins that hub, so it shows the same universe,
binaries, config and data. Activating "Scenario %[3]s" from the GUI's ribbon
without the launcher spawns the installed control plane's hub instead — not the
case's binaries; 'scenario start' replaces such a hub.

To drive the GUI from an agent (skill ktw-compose:/skill/test-gui):
  ktw-compose scenario start %[3]s          # the case's own hub; the GUI joins it
  cd app/ktw-gui && wails dev               # then open http://localhost:34115
  # with a browser automation tool: navigate to http://localhost:34115,
  # click the ribbon entry "Universe Scenario %[3]s", then Activate;
  # take screenshots, read the console diagnostics, evaluate scripts in the page
Stop the GUI before 'scenario run' again (it restarts the universe headless).
`, e.Case.UniverseName(), e.RT.Project(), e.Case.Name, LauncherName())
}

// Scaffold creates a case folder from the template.
func Scaffold(dir, name string) error {
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s exists", dir)
	}
	files := map[string]string{
		"test-case.yaml":    strings.ReplaceAll(caseTemplate, "{{name}}", name),
		"user.md":           userTemplate,
		"project/README.md": "# " + name + "\n\nThe project's initial content: what the universe's project starts with.\n",
	}
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

const caseTemplate = `# A scenario test case: ktw-compose scenario --help
name: {{name}}
title: What this case proves
description: |
  The intent, from the user's side, and what "done" means.

universe:
  sidecars: [ktw-agents, ktw-methods]
  copy:                 # into the universe at install; to = relative to its home
    - {from: config, to: config, select: ["*.yaml"]}        # a folder, selected
    - {from: ~/.config/katyoti/.env, to: .env, optional: true}  # one file, keys kept out of the case

project:
  from: project         # the project's initial content
  git: true             # a commit per stage: what each stage changed is a diff

user: user.md           # the person the simulated user plays

stages:
  - id: s0-seed
    title: The module exists
    steps:
      - call:
          uri: ktw-agents:cwm/ObjectCreate
          input: {module: app, object: {id: app, type: documentation, name: App, capabilities: {}}}
    expect:
      - objects: {type: documentation, name: App}
`

const userTemplate = `# The user

Who you are, what you want, what you know, what you prefer, and how you answer
(how much you challenge, when you accept).
`
