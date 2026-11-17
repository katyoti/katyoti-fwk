// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// System is the runner that installs on this machine: winget and scoop on
// Windows, brew on macOS and Linux, apt on Linux, pip, bun and a sidecar's
// script anywhere. Every step but apt runs in the person's own profile.
type System struct{}

var _ Runner = System{}

// Available says whether a kind's installer is on PATH: one of the commands
// publish.RecipeInstallers names for it; a kind with none needs no installer.
func (System) Available(kind string) bool {
	if kind == publish.RecipePip {
		return pipCommand() != nil
	}
	cmds, ok := publish.RecipeInstallers[kind]
	if !ok {
		return true
	}
	for _, c := range cmds {
		if lookPath(c) {
			return true
		}
	}
	return false
}

// Install runs one user-scope step.
func (System) Install(ctx context.Context, s Step) error {
	argv, err := commandLine(s)
	if err != nil {
		return err
	}
	err = runCommand(ctx, argv)
	if err != nil && s.Recipe.Kind == publish.RecipeWinget {
		return explainWinget(s, err)
	}
	if err == nil && s.Recipe.Kind == publish.RecipePip {
		// pip --user puts a program's commands in Python's user scripts
		// folder, which no PATH holds by default (Windows): the check would
		// not find what was just installed.
		if dir := pipScriptsDir(ctx, pipCommand()); dir != "" {
			return System{}.AddToPath(dir)
		}
	}
	if err == nil && s.Recipe.Kind == publish.RecipeBun {
		// bun add --global puts a package's commands in bun's global bin,
		// which a PATH read before bun was installed does not hold.
		if dir := publish.BunGlobalBin(); dir != "" {
			return System{}.AddToPath(dir)
		}
	}
	return err
}

// userScripts prints Python's user scripts folder: where pip --user puts a
// package's commands (the preferred user scheme on Python 3.10 and later).
const userScripts = "import os, sysconfig; f = getattr(sysconfig, 'get_preferred_scheme', None); " +
	"print(sysconfig.get_path('scripts', f('user') if f else os.name + '_user'))"

// pipScriptsDir is the user scripts folder of the Python pip runs as (pip
// given as `<python> -m pip`); "" when pip runs alone or Python does not say.
func pipScriptsDir(ctx context.Context, pip []string) string {
	if len(pip) < 3 || pip[1] != "-m" {
		return ""
	}
	cmd := exec.CommandContext(ctx, pip[0], "-c", userScripts)
	publish.NoConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// machineOnlyDependency finds, in winget's output (any language: "manifest",
// "manifeste"), the package a step depends on that has no installer for its
// user scope - the Visual C++ runtime typst needs.
var machineOnlyDependency = regexp.MustCompile(`(?i)manifest\w*\s*:?\s*([A-Za-z0-9][\w-]*\.[\w.+-]*[\w+])\s`)

// dependencyNames are what a person (and their help desk) calls a dependency.
var dependencyNames = map[string]string{
	"microsoft.vcredist": "the Microsoft Visual C++ 2015-2022 Redistributable (x64)",
}

// explainWinget turns a winget step that failed on a dependency only an
// administrator can install into what the person can act on: a user who is
// not an administrator asks their help desk, then installs again. Any other
// failure is returned as it is. The command's failure stays underneath, for
// the step's report.
func explainWinget(s Step, err error) error {
	text := err.Error()
	var ce *CommandError
	if errors.As(err, &ce) {
		text = ce.Output
	}
	m := machineOnlyDependency.FindStringSubmatch(text)
	if m == nil || strings.EqualFold(m[1], s.Recipe.Package) {
		return err
	}
	dep := m[1]
	name := dep
	for prefix, friendly := range dependencyNames {
		if strings.HasPrefix(strings.ToLower(dep), prefix) {
			name = friendly + " (" + dep + ")"
			break
		}
	}
	return &explained{msg: fmt.Sprintf("%s needs %s, which only an administrator can install on this computer: ask your help desk to install it, then run the install again", s.Program, name), cause: err}
}

// explained is a failure said in the person's terms, over the one it
// explains.
type explained struct {
	msg   string
	cause error
}

func (e *explained) Error() string { return e.msg }
func (e *explained) Unwrap() error { return e.cause }

// CommandError is a step's command that failed: what ran, how it ended and
// everything it printed - the report a person, or the assistant they ask,
// reads to understand it.
type CommandError struct {
	Command string
	// ExitCode is the command's exit code; -1 when it did not exit on its own
	// (stopped, or never started).
	ExitCode int
	// Output is all it wrote, its output and its errors interleaved.
	Output string
	Cause  error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("%s: %v%s", e.Command, e.Cause, tail(e.Output))
}

func (e *CommandError) Unwrap() error { return e.Cause }

// wingetCodes say what winget's own exit codes mean, in words a person acts
// on (winget's documented return codes).
var wingetCodes = map[uint32]string{
	0x8A150010: "no installer of this package fits this computer for the scope asked: here, an installation for the user only (the package installs only machine-wide, with administrator rights)",
	0x8A150011: "the downloaded installer does not match the hash its manifest declares",
	0x8A150014: "no package with this id was found in the source",
	0x8A150104: "a package this one depends on could not be installed",
}

// WingetLogs is where winget writes its logs on this machine ("" elsewhere).
func WingetLogs() string {
	if runtime.GOOS != "windows" || os.Getenv("LOCALAPPDATA") == "" {
		return ""
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Packages", "Microsoft.DesktopAppInstaller_8wekyb3d8bbwe", "LocalState", "DiagOutputDir")
}

// Explain is a failed step's short message and its full report: the
// command, its exit code (with winget's meaning), all it printed and where
// its logs are. A failure that ran no command reports its message only.
func Explain(err error) (string, *mktapi.StepReport) {
	var ce *CommandError
	if !errors.As(err, &ce) {
		return err.Error(), nil
	}
	r := &mktapi.StepReport{Command: ce.Command, ExitCode: ce.ExitCode, Output: strings.TrimSpace(ce.Output)}
	if r.ExitCode != -1 {
		r.Code = fmt.Sprintf("%d", r.ExitCode)
	}
	if strings.HasPrefix(ce.Command, "winget ") {
		code := uint32(ce.ExitCode)
		if ce.ExitCode != -1 {
			r.Code = fmt.Sprintf("0x%08X", code)
		}
		r.Meaning = wingetCodes[code]
		r.Logs = WingetLogs()
	}
	if ce.Cause != nil && r.ExitCode == -1 {
		r.Meaning = ce.Cause.Error()
	}
	short := lastLine(r.Output)
	switch {
	case err.Error() != ce.Error():
		short = err.Error() // explained in the person's terms
	case r.Meaning != "":
		short = r.Meaning
	case short == "":
		short = ce.Cause.Error()
	}
	if r.Code != "" {
		short = fmt.Sprintf("%s (%s)", short, r.Code)
	}
	return short, r
}

// lastLine is the last line of out that says something: winget's own
// summary of what went wrong, after its progress.
func lastLine(out string) string {
	lines := strings.Split(strings.ReplaceAll(out, "\r", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); len(strings.Trim(l, `-\|/ █▒`)) > 2 {
			return l
		}
	}
	return ""
}

// InstallElevated runs every administrator step in one elevated shell: one
// prompt (pkexec or sudo on Linux, the administrator dialog on macOS, the UAC
// prompt on Windows).
func (System) InstallElevated(ctx context.Context, steps []Step) error {
	if len(steps) == 0 {
		return nil
	}
	var lines []string
	for _, s := range steps {
		argv, err := commandLine(s)
		if err != nil {
			return err
		}
		lines = append(lines, shellJoin(argv))
	}
	return runElevated(ctx, lines)
}

// AddToPath puts dir on the person's PATH, and on this process's.
func (System) AddToPath(dir string) error {
	prependProcessPath(dir)
	return persistUserPath(dir)
}

// RefreshPath reads PATH again from where the system keeps it.
func (System) RefreshPath() error {
	return refreshPath()
}

// commandLine is the command a step runs.
func commandLine(s Step) ([]string, error) {
	r := s.Recipe
	switch r.Kind {
	case publish.RecipeWinget:
		// --source winget: the recipes name the winget repository's ids, and an
		// unreachable Microsoft Store source (a Sandbox, a locked-down PC) would
		// otherwise stop the install (0x8a15003b) though the package is found.
		argv := []string{"winget", "install", "--id", r.Package, "--exact", "--source", "winget"}
		// The validated version, not the latest: a sidecar's check accepts
		// only the version it is validated with (pandoc 3.11, not 3.12.1).
		if s.Version != "" {
			argv = append(argv, "--version", s.Version)
		}
		if r.Custom != "" {
			argv = append(argv, "--custom", r.Custom)
		}
		return append(argv, "--scope", "user", "--silent",
			"--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"), nil
	case publish.RecipeScoop:
		return []string{"scoop", "install", r.Package}, nil
	case publish.RecipeBrew:
		return []string{"brew", "install", r.Package}, nil
	case publish.RecipeApt:
		return []string{"apt-get", "install", "-y", r.Package}, nil
	case publish.RecipePip:
		if pip := pipCommand(); pip != nil {
			return append(pip, "install", "--user", r.Package), nil
		}
		return nil, fmt.Errorf("pip %s: neither python nor pip on PATH", r.Package)
	case publish.RecipeBun:
		return []string{"bun", "add", "--global", r.Package}, nil
	case publish.RecipeScript:
		if s.Script == "" {
			return nil, fmt.Errorf("script %s: not resolved in its sidecar's folder", r.Script)
		}
		switch strings.ToLower(filepath.Ext(s.Script)) {
		case ".ps1":
			return []string{"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", s.Script}, nil
		case ".cmd", ".bat":
			return []string{"cmd", "/c", s.Script}, nil
		}
		return []string{"sh", s.Script}, nil
	}
	return nil, fmt.Errorf("%s: not a step a runner runs", r.Kind)
}

// waitDelay bounds how long a stopped command is waited on once its own
// process is killed: an installer it started may hold its output open.
var waitDelay = 10 * time.Second

func runCommand(ctx context.Context, argv []string) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.WaitDelay = waitDelay
	publish.NoConsoleWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) && ctx.Err() == nil {
			code = ee.ExitCode()
		}
		return &CommandError{Command: strings.Join(argv, " "), ExitCode: code, Output: out.String(), Cause: err}
	}
	return nil
}

func runElevated(ctx context.Context, lines []string) error {
	var argv []string
	switch runtime.GOOS {
	case "windows":
		f, err := os.CreateTemp("", "ktw-prereqs-*.cmd")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		body := "@echo off\r\n" + strings.Join(lines, " || exit /b 1\r\n") + " || exit /b 1\r\n"
		if _, err := f.WriteString(body); err != nil {
			f.Close()
			return err
		}
		f.Close()
		argv = []string{"powershell", "-NoProfile", "-Command",
			"$p = Start-Process -FilePath cmd -ArgumentList '/c','\"" + f.Name() + "\"' -Verb RunAs -Wait -PassThru; exit $p.ExitCode"}
	case "darwin":
		script := strings.ReplaceAll(strings.Join(lines, " && "), `"`, `\"`)
		argv = []string{"osascript", "-e", `do shell script "` + script + `" with administrator privileges`}
	default:
		script := strings.Join(lines, " && ")
		switch {
		case os.Geteuid() == 0:
			argv = []string{"sh", "-c", script}
		case lookPath("pkexec"):
			argv = []string{"pkexec", "sh", "-c", script}
		default:
			argv = []string{"sudo", "sh", "-c", script}
		}
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.WaitDelay = waitDelay
	publish.NoConsoleWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) && ctx.Err() == nil {
			code = ee.ExitCode()
		}
		return &CommandError{Command: strings.Join(lines, " && "), ExitCode: code, Output: out.String(), Cause: err}
	}
	return nil
}

// shellJoin quotes argv for the elevated shell line.
func shellJoin(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && !strings.ContainsAny(a, " \t\"'&|;<>()$`\\*?") {
			q[i] = a
			continue
		}
		if runtime.GOOS == "windows" {
			q[i] = `"` + strings.ReplaceAll(a, `"`, `""`) + `"`
		} else {
			q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(q, " ")
}

// pipCommand is how pip runs here: `<python> -m pip` with the Python a
// package is then checked in (publish.PythonCommand: the first that answers as
// one, the Windows Store's python stub does not), else pip itself; nil when
// neither is here.
func pipCommand() []string {
	if p := publish.PythonCommand(); p != "" {
		return []string{p, "-m", "pip"}
	}
	for _, p := range []string{"pip3", "pip"} {
		if lookPath(p) {
			return []string{p}
		}
	}
	return nil
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func prependProcessPath(dir string) {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if samePath(p, dir) {
			return
		}
	}
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// mergePath keeps the process's PATH and adds the entries of fresh it lacks.
func mergePath(current string, fresh ...string) string {
	parts := filepath.SplitList(current)
	for _, f := range fresh {
		for _, p := range filepath.SplitList(f) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			dup := false
			for _, q := range parts {
				if samePath(p, q) {
					dup = true
					break
				}
			}
			if !dup {
				parts = append(parts, p)
			}
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

// userPathWith is the user's PATH (a ;-list) with dir first: a program the
// engine placed must come before an older entry of its name already on it -
// the Windows Store's python.exe stub in WindowsApps, on every user's PATH.
// An entry already naming dir (as written, or once expand resolves its
// variables) is moved to the front, never doubled.
func userPathWith(cur, dir string, expand func(string) string) string {
	out := []string{dir}
	for _, p := range strings.Split(cur, ";") {
		if strings.TrimSpace(p) == "" || samePath(p, dir) || samePath(expand(p), dir) {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ";")
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 600 {
		s = "…" + s[len(s)-600:]
	}
	return ": " + s
}
