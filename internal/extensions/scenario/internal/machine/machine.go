// SPDX-License-Identifier: Apache-2.0

// Package machine runs a scenario case on a clean machine — the proof that a
// person, given only a channel folder, goes from the installer's double-click
// to a working installation. The case's machine stages (api.CaseMachine) run
// in order through the programs the installer put there, each timed, and the
// results — one per stage, with its duration — are written as JSON after
// every stage.
//
// Two kinds of machine: a fresh Windows Sandbox (wsb.go: the tool writes its
// .wsb, launches it and waits for the results the bootstrap writes there), and
// this machine closed in a world of its own (local.go: its home, its
// installation folder), which runs the same stages, a dry run where the clean
// machine installs programs for real.
package machine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// ResultsFile is the results' name in the results folder.
const ResultsFile = "results.json"

// Stage outcomes.
const (
	Passed  = "passed"
	Failed  = "failed"
	Skipped = "skipped"
)

// Plan is what the runner on the machine is given: the case's stages, the
// channel as the machine sees it, the variables passed in.
type Plan struct {
	Case string `json:"case"`
	Kind string `json:"kind"`
	// Channel is the infra channel folder, as the machine sees it.
	Channel string             `json:"channel"`
	Env     map[string]string  `json:"env,omitempty"`
	Stages  []api.MachineStage `json:"stages"`
}

// StageResult is one stage run: its outcome and how long it took.
type StageResult struct {
	ID         string `json:"id"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Exit       int    `json:"exit"`
	// Command is the command run, its variables expanded.
	Command string `json:"command,omitempty"`
	// Output is the end of what it printed.
	Output  string `json:"output,omitempty"`
	Failure string `json:"failure,omitempty"`
}

// Results are a run's stages; Done once the last ran (or one failed).
type Results struct {
	Case       string        `json:"case"`
	Kind       string        `json:"kind"`
	Machine    string        `json:"machine,omitempty"`
	StartedAt  string        `json:"started_at"`
	EndedAt    string        `json:"ended_at,omitempty"`
	DurationMS int64         `json:"duration_ms"`
	Done       bool          `json:"done"`
	Passed     bool          `json:"passed"`
	Stages     []StageResult `json:"stages"`
}

// Duration is a stage's time.
func (s StageResult) Duration() time.Duration { return time.Duration(s.DurationMS) * time.Millisecond }

// World is the machine the stages run on, as the runner reaches it.
type World struct {
	// Env is the stages' environment.
	Env []string
	// Home is the person's home folder.
	Home string
	// Install is the installation folder when the world names it; empty, it
	// is found where an installer run as a person puts it.
	Install string
	// Local runs each stage's local_cmd where it has one.
	Local bool
	// FreshPath, when set, gives each stage the PATH a process started now
	// gets: a person opens a new window after a stage installed programs
	// (the clean machine itself; nil in a world closed in a folder).
	FreshPath func(current string) string
	// RunCase runs a case stage's steps and checks on an installed universe;
	// nil: case stages fail saying this runner cannot run them.
	RunCase CaseRunner
}

// CaseTarget is the installed universe a case stage runs on, as this machine
// reaches it.
type CaseTarget struct {
	// Universe is its name, Home its folder, Data its hub's data folder
	// (where its lockfile is), Project the project it serves.
	Universe, Home, Data, Project string
	// Work is where the stages keep their reports and state.
	Work string
}

// CaseRunner runs stages of a case (their steps, then their checks) on an
// installed universe, writing what each step did to out; the error says the
// first stage that failed and why. The scenario engine is wired by the
// command: this package runs no case step itself.
type CaseRunner func(ctx context.Context, c api.MachineCase, t CaseTarget, out io.Writer) error

// LoadPlan reads a plan written by WritePlan.
func LoadPlan(path string) (Plan, error) {
	var p Plan
	raw, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// WritePlan writes a plan.
func WritePlan(path string, p Plan) error {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// LoadResults reads a results file.
func LoadResults(path string) (Results, error) {
	var r Results
	// Shared with its writer, which replaces it as stages end.
	raw, err := atomicio.ReadAtomic(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// writeResults replaces the results file whole: a reader polling it never
// sees half of one.
func writeResults(dir string, r Results) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Replaced while the host reads it: atomicio's write survives a reader
	// holding the file (on Windows a plain rename over it fails).
	return atomicio.WriteAtomic(filepath.Join(dir, ResultsFile), raw, 0o644)
}

// Run runs the plan's stages on the world, in order, each timed; the first
// that fails stops the run (the rest are skipped). The results are written
// into resultsDir after every stage. The error is the runner's own (a results
// folder it cannot write); a failed stage is in the results.
func Run(ctx context.Context, plan Plan, w World, resultsDir string, out io.Writer) (Results, error) {
	if out == nil {
		out = io.Discard
	}
	host, _ := os.Hostname()
	start := time.Now()
	res := Results{Case: plan.Case, Kind: plan.Kind, Machine: host, StartedAt: start.UTC().Format(time.RFC3339)}
	for _, s := range plan.Stages {
		res.Stages = append(res.Stages, StageResult{ID: s.ID, Title: s.Title, Status: Skipped})
	}
	if err := writeResults(resultsDir, res); err != nil {
		return res, err
	}
	r := &runner{plan: plan, world: w, results: resultsDir, env: append(append([]string{}, w.Env...), envList(plan.Env)...), live: out}
	failed := false
	for i, s := range plan.Stages {
		if failed {
			break
		}
		fmt.Fprintf(out, "==> %s %s\n", s.ID, s.Title)
		sr := r.stage(ctx, s)
		res.Stages[i] = sr
		failed = sr.Status != Passed
		fmt.Fprintf(out, "    %s in %s%s\n", sr.Status, sr.Duration().Round(time.Millisecond), suffix(sr.Failure))
		res.DurationMS = time.Since(start).Milliseconds()
		if err := writeResults(resultsDir, res); err != nil {
			return res, err
		}
	}
	res.Done, res.Passed = true, !failed
	res.EndedAt = time.Now().UTC().Format(time.RFC3339)
	res.DurationMS = time.Since(start).Milliseconds()
	return res, writeResults(resultsDir, res)
}

func suffix(failure string) string {
	if failure == "" {
		return ""
	}
	return ": " + failure
}

type runner struct {
	plan  Plan
	world World
	env   []string
	// results is the run's results folder (${results}).
	results string
	// install is the installation, once the installer ran.
	install string
	inst    *cenv.Installation
	// live gets each stage's output as it comes (the run's log, which a
	// window in the Sandbox follows); nil: kept for the results only.
	live io.Writer
}

func (r *runner) stage(ctx context.Context, s api.MachineStage) (sr StageResult) {
	sr = StageResult{ID: s.ID, Title: s.Title, Status: Failed, Exit: -1}
	start := time.Now()
	defer func() { sr.DurationMS = time.Since(start).Milliseconds() }()

	if s.GUI != nil {
		return r.guiStage(ctx, s, sr)
	}
	if s.Case != nil {
		return r.caseStage(ctx, s, sr)
	}
	argv := s.Cmd
	if r.world.Local && len(s.LocalCmd) > 0 {
		argv = s.LocalCmd
	}
	env := append([]string{}, r.env...)
	if r.world.FreshPath != nil {
		path := r.world.FreshPath(lookup(env, "PATH"))
		env = append(env, "PATH="+path)
		// The program itself is looked up in this process's PATH.
		_ = os.Setenv("PATH", path)
	}
	if s.Install {
		argv = installerCommand(r.plan.Channel)
		// The run is headless: setup would otherwise wait on the GUI it opens.
		if _, set := s.Env["KTW_SETUP_ARGS"]; !set {
			env = append(env, "KTW_SETUP_ARGS=--no-gui")
		}
		if r.world.Install != "" {
			env = append(env, "KTW_INSTALLATION_DIR="+r.world.Install)
		}
	}
	for k, v := range s.Env {
		x, err := r.expand(v)
		if err != nil {
			sr.Failure = "env " + k + ": " + err.Error()
			return sr
		}
		env = append(env, k+"="+x)
	}
	args := make([]string, len(argv))
	for i, a := range argv {
		x, err := r.expand(a)
		if err != nil {
			sr.Failure = err.Error()
			return sr
		}
		args[i] = x
	}
	sr.Command = strings.Join(args, " ")

	timeout := time.Duration(s.TimeoutMin) * time.Minute
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, args[0], args[1:]...)
	cmd.Env = env
	cmd.Dir = r.world.Home
	var buf bytes.Buffer
	var sink io.Writer = &buf
	if r.live != nil {
		sink = io.MultiWriter(&buf, r.live)
	}
	cmd.Stdout, cmd.Stderr = sink, sink
	// A program the stage leaves running (a hub setup started) keeps the
	// output open: the stage ends with its own process.
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	output := buf.String()
	sr.Output = tail(output, 4000)
	sr.Exit = cmd.ProcessState.ExitCode()
	switch {
	case cctx.Err() == context.DeadlineExceeded:
		sr.Failure = fmt.Sprintf("timed out after %s", timeout)
		return sr
	case err != nil && cmd.ProcessState == nil:
		sr.Failure = err.Error()
		return sr
	}
	want := 0
	if s.Exit != nil {
		want = *s.Exit
	}
	if sr.Exit != want {
		sr.Failure = fmt.Sprintf("exit %d, want %d", sr.Exit, want)
		return sr
	}
	if s.Install {
		if err := r.found(); err != nil {
			sr.Failure = err.Error()
			return sr
		}
	}
	for _, c := range s.Contains {
		x, err := r.expand(c)
		if err != nil {
			sr.Failure = err.Error()
			return sr
		}
		if !strings.Contains(output, x) {
			sr.Failure = fmt.Sprintf("the output does not contain %q", x)
			return sr
		}
	}
	for _, c := range s.Excludes {
		x, err := r.expand(c)
		if err != nil {
			sr.Failure = err.Error()
			return sr
		}
		if strings.Contains(output, x) {
			sr.Failure = fmt.Sprintf("the output contains %q", x)
			return sr
		}
	}
	for _, c := range s.Images {
		x, err := r.expand(c)
		if err != nil {
			sr.Failure = err.Error()
			return sr
		}
		if err := CheckImage(x); err != nil {
			sr.Failure = err.Error()
			return sr
		}
	}
	sr.Status = Passed
	return sr
}

// caseStage runs another case's stages on the installed universe through the
// world's case runner; its output is what each step did and each check saw.
func (r *runner) caseStage(ctx context.Context, s api.MachineStage, sr StageResult) StageResult {
	if r.world.RunCase == nil {
		sr.Failure = "this runner cannot run case stages"
		return sr
	}
	t, err := r.caseTarget(s.Case.Universe)
	if err != nil {
		sr.Failure = err.Error()
		return sr
	}
	t.Work = filepath.Join(r.results, "case-"+s.ID)
	sr.Command = fmt.Sprintf("case %s on %s (%d stages)", s.Case.Name, t.Universe, len(s.Case.Resolved))
	timeout := time.Duration(s.TimeoutMin) * time.Minute
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var buf bytes.Buffer
	var sink io.Writer = &buf
	if r.live != nil {
		sink = io.MultiWriter(&buf, r.live)
	}
	began := time.Now()
	err = r.world.RunCase(cctx, *s.Case, t, sink)
	// What the universe wrote meanwhile, pass or fail: its logs, the task
	// runs' journals.
	saveEvidence(t.Data, t.Work, began, evidenceEnvFiles(r.inst, t.Home))
	sr.Output = tail(buf.String(), 4000)
	switch {
	case cctx.Err() == context.DeadlineExceeded:
		sr.Failure = fmt.Sprintf("timed out after %s", timeout)
		return sr
	case err != nil:
		sr.Failure = err.Error()
		return sr
	}
	sr.Exit, sr.Status = 0, Passed
	return sr
}

// caseTarget is the installed universe named (empty: the installation's
// default universe), from the installation's universe registry.
func (r *runner) caseTarget(name string) (CaseTarget, error) {
	if r.inst == nil {
		return CaseTarget{}, fmt.Errorf("a case stage runs once the installer ran")
	}
	if name == "" {
		u, err := r.value("universe", "")
		if err != nil {
			return CaseTarget{}, err
		}
		name = u
	}
	raw, err := os.ReadFile(filepath.Join(r.inst.ConfigRoot(), "universes.yaml"))
	if err != nil {
		return CaseTarget{}, fmt.Errorf("the installation's universe registry: %w", err)
	}
	var reg struct {
		Universes []struct {
			Name     string `yaml:"name"`
			HomePath string `yaml:"home_path"`
		} `yaml:"universes"`
	}
	if err := config.DecodeBytes(raw, marshal.YAML, &reg); err != nil {
		return CaseTarget{}, fmt.Errorf("the installation's universe registry: %w", err)
	}
	for _, u := range reg.Universes {
		if u.Name == name {
			home := u.HomePath
			// An installation's programs take its data root as XDG_DATA_HOME
			// and keep their data in its katyoti folder (cenv.ResolveDataDir).
			return CaseTarget{Universe: name, Home: home, Data: filepath.Join(r.inst.DataRoot(), "katyoti", "universes", name),
				Project: cenv.AttachedProject(application.EnvOf(home).HomeDir())}, nil
		}
	}
	return CaseTarget{}, fmt.Errorf("the installation has no universe %s", name)
}

// guiStage drives the GUI a previous stage started (gui.go); its output is
// what each step did, read by the stage's contains and excludes like a
// command's, and its screenshots are checked with its images.
func (r *runner) guiStage(ctx context.Context, s api.MachineStage, sr StageResult) StageResult {
	timeout := time.Duration(s.TimeoutMin) * time.Minute
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var buf bytes.Buffer
	var sink io.Writer = &buf
	if r.live != nil {
		sink = io.MultiWriter(&buf, r.live)
	}
	port := s.GUI.Port
	if port == 0 {
		port = api.GUIDebugPort
	}
	sr.Command = fmt.Sprintf("gui: %d steps on 127.0.0.1:%d", len(s.GUI.Steps), port)
	shots, err := runGUI(cctx, s.GUI, r.expand, sink)
	output := buf.String()
	sr.Output = tail(output, 4000)
	if err != nil {
		sr.Failure = err.Error()
		return sr
	}
	sr.Exit = 0
	for _, c := range s.Contains {
		x, err := r.expand(c)
		if err != nil {
			sr.Failure = err.Error()
			return sr
		}
		if !strings.Contains(output, x) {
			sr.Failure = fmt.Sprintf("the output does not contain %q", x)
			return sr
		}
	}
	for _, c := range s.Excludes {
		x, err := r.expand(c)
		if err != nil {
			sr.Failure = err.Error()
			return sr
		}
		if strings.Contains(output, x) {
			sr.Failure = fmt.Sprintf("the output contains %q", x)
			return sr
		}
	}
	images := shots
	for _, c := range s.Images {
		x, err := r.expand(c)
		if err != nil {
			sr.Failure = err.Error()
			return sr
		}
		images = append(images, x)
	}
	for _, p := range images {
		if err := CheckImage(p); err != nil {
			sr.Failure = err.Error()
			return sr
		}
	}
	sr.Status = Passed
	return sr
}

// installerCommand is the installer a person double-clicks in the channel
// folder: install-on-windows.cmd on Windows, install.sh elsewhere.
func installerCommand(channel string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", filepath.Join(channel, "install-on-windows.cmd")}
	}
	return []string{"sh", filepath.Join(channel, "install.sh")}
}

// found finds the installation the installer made: the world's folder, else
// the one katyoti.yml under the person's programs folder.
func (r *runner) found() error {
	dir := r.world.Install
	if dir == "" {
		base := filepath.Join(lookup(r.env, "LOCALAPPDATA"), "Programs", "Katyoti")
		if runtime.GOOS != "windows" {
			base = filepath.Join(r.world.Home, ".local", "share", "katyoti", "installations")
		}
		files, _ := filepath.Glob(filepath.Join(base, "*", cenv.InstallationFile))
		if len(files) != 1 {
			return fmt.Errorf("one installation expected under %s after the installer ran, found %d", base, len(files))
		}
		dir = filepath.Dir(files[0])
	}
	inst, err := cenv.LoadInstallation(dir)
	if err != nil {
		return fmt.Errorf("the installer made no installation: %w", err)
	}
	r.install, r.inst = dir, inst
	return nil
}

var varRe = regexp.MustCompile(`\$\{([a-z]+)(?::([^}]+))?\}`)

// expand replaces the stage variables in s.
func (r *runner) expand(s string) (string, error) {
	var bad error
	out := varRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := varRe.FindStringSubmatch(m)
		v, err := r.value(sub[1], sub[2])
		if err != nil && bad == nil {
			bad = fmt.Errorf("%s: %w", m, err)
		}
		return v
	})
	return out, bad
}

func (r *runner) value(name, arg string) (string, error) {
	switch name {
	case "channel":
		return r.plan.Channel, nil
	case "home":
		return r.world.Home, nil
	case "results":
		return r.results, nil
	case "documents":
		return filepath.Join(r.world.Home, "Documents"), nil
	case "env":
		return lookup(r.env, arg), nil
	}
	if r.inst == nil {
		return "", fmt.Errorf("unknown until the installer ran")
	}
	switch name {
	case "install":
		return r.install, nil
	case "katyoti":
		return filepath.Join(r.inst.ConfigRoot(), "control-plane", exe("katyoti")), nil
	case "solution":
		return r.inst.Solution, nil
	case "universe":
		u, _ := r.inst.Settings["default_universe"].(string)
		if u == "" {
			return "", fmt.Errorf("the installation names no default universe")
		}
		return u, nil
	case "bin":
		return installedBinary(filepath.Join(r.inst.ConfigRoot(), "bin", "store", arg), arg)
	}
	return "", fmt.Errorf("unknown variable")
}

// installedBinary is the newest <name> executable under an item's store folder.
func installedBinary(store, name string) (string, error) {
	var best string
	var bestAt time.Time
	_ = filepath.WalkDir(store, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(d.Name(), exe(name)) {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(bestAt) {
			best, bestAt = p, info.ModTime()
		}
		return nil
	})
	if best == "" {
		return "", fmt.Errorf("%s is not installed (no %s under %s)", name, exe(name), store)
	}
	return best, nil
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// lookup is a variable of an environment list (the last one set wins).
func lookup(env []string, key string) string {
	v := ""
	for _, kv := range env {
		if k, val, ok := strings.Cut(kv, "="); ok && sameKey(k, key) {
			v = val
		}
	}
	return v
}

func sameKey(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func envList(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
