// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// Options are a clean-machine run of a case, as the machine running the tool
// starts it.
type Options struct {
	Case *api.Case
	// Kind overrides the case's (default windows-sandbox).
	Kind string
	// Channel overrides the case's channel folder.
	Channel string
	// Dir is the run's folder (<root>/<case>/clean-machine), made afresh.
	Dir string
	// Runner is this tool's executable (copied into a Sandbox).
	Runner string
	// Sandbox is WindowsSandbox.exe (default SandboxExe()).
	Sandbox string
	// Timeout overrides the case's (default 120 minutes).
	Timeout time.Duration
	// Env is the environment of the machine running the tool.
	Env []string
	Out io.Writer
	// Cache is the preparation cache folder (default <root>/sandbox-cache).
	Cache string
	// RunCase runs a case stage's steps on an installed universe (the local
	// kind; a Sandbox's runner is given its own).
	RunCase CaseRunner
	// Only runs just these of the case's stages, in the case's order (a
	// hand-over: the installer and the GUI opened, then the machine is the
	// person's); the Sandbox's preparation still runs first. A pass_env
	// variable not set is then left out, not refused: the stages that need
	// it are not run.
	Only []string
}

// stagesOf is the case's stages a run takes: every one, or those Only names -
// an id it names that the case lacks is refused.
func stagesOf(m *api.CaseMachine, only []string) ([]api.MachineStage, error) {
	if len(only) == 0 {
		return m.Stages, nil
	}
	want := map[string]bool{}
	for _, id := range only {
		want[id] = true
	}
	var out []api.MachineStage
	for _, s := range m.Stages {
		if want[s.ID] {
			out = append(out, s)
			delete(want, s.ID)
		}
	}
	if len(want) > 0 {
		var missing []string
		for _, id := range only {
			if want[id] {
				missing = append(missing, id)
			}
		}
		return nil, fmt.Errorf("the case has no machine stage %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// Prove runs the case's machine stages on a clean machine and answers their
// results; a failed stage is in them, the error is the run's own.
func Prove(ctx context.Context, o Options) (Results, error) {
	m := o.Case.Machine
	if m == nil {
		return Results{}, fmt.Errorf("case %s has no machine: section", o.Case.Name)
	}
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	kind := KindOf(o.Case, o.Kind)
	if kind != api.MachineWindowsSandbox && kind != api.MachineLocal {
		return Results{}, fmt.Errorf("kind %q: %s or %s", kind, api.MachineWindowsSandbox, api.MachineLocal)
	}
	channel, err := o.channel()
	if err != nil {
		return Results{}, err
	}
	stages, err := stagesOf(m, o.Only)
	if err != nil {
		return Results{}, err
	}
	if stages, err = resolveCases(o.Case, stages); err != nil {
		return Results{}, err
	}
	plan := Plan{Case: o.Case.Name, Kind: kind, Stages: stages, Env: map[string]string{}}
	for _, k := range m.PassEnv {
		v := lookup(o.Env, k)
		if v == "" {
			if len(o.Only) > 0 {
				continue
			}
			return Results{}, fmt.Errorf("machine.pass_env: %s is not set", k)
		}
		plan.Env[k] = v
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = time.Duration(m.TimeoutMin) * time.Minute
	}
	if timeout <= 0 {
		timeout = 120 * time.Minute
	}

	if kind == api.MachineLocal {
		world := filepath.Join(o.Dir, "world")
		StopUnder(ctx, world)
		if _, err := KeepPreviousResults(o.Dir); err != nil {
			return Results{}, fmt.Errorf("keep the previous run's results: %w", err)
		}
		if err := os.RemoveAll(world); err != nil {
			return Results{}, fmt.Errorf("a previous run's world: %w", err)
		}
		w, err := LocalWorld(world, o.Env)
		if err != nil {
			return Results{}, err
		}
		w.RunCase = o.RunCase
		// Everything the stages started is stopped: the world is left as files.
		defer StopUnder(context.Background(), world)
		plan.Channel = channel
		fmt.Fprintf(out, "clean machine: this one, closed in %s\n", world)
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return Run(cctx, plan, w, filepath.Join(o.Dir, "results"), out)
	}

	exe := firstOf(o.Sandbox, SandboxExe())
	if err := RequireSandbox(exe); err != nil {
		return Results{}, err
	}
	// The preparation from the cache beside the runs (fetched once,
	// refreshed when stale); the network when it cannot be made.
	cache := firstOf(o.Cache, filepath.Join(filepath.Dir(filepath.Dir(o.Dir)), CacheDir))
	cached := EnsureCache(ctx, cache, nil, out)
	if !cached {
		cache = ""
	}
	plan.Stages = sandboxStagesOf(&api.CaseMachine{Prepare: m.Prepare, Stages: stages}, cached)
	plan.Channel = SandboxMarket + `\` + filepath.Base(channel)
	run := SandboxRun{Dir: o.Dir, Market: filepath.Dir(channel), Runner: o.Runner, Plan: plan, Cache: cache}
	file, err := run.Prepare()
	if err != nil {
		return Results{}, err
	}
	if err := Launch(exe, file); err != nil {
		return Results{}, fmt.Errorf("start Windows Sandbox on %s: %w", file, err)
	}
	fmt.Fprintf(out, "clean machine: Windows Sandbox on %s (the marketplace %s mapped read-only); waiting for %s\n", file, run.Market, filepath.Join(run.Results(), ResultsFile))
	res, err := Await(ctx, run.Results(), timeout, 5*time.Second, out)
	if err == nil {
		fmt.Fprintln(out, "close the Sandbox window (and confirm) to discard the machine, or look around in it first; Windows runs one Sandbox at a time: the next run starts only once it is closed")
	}
	return res, err
}

// KindOf is the machine a case runs on: the flag's kind, else the case's,
// else a Windows Sandbox.
func KindOf(c *api.Case, flag string) string {
	caseKind := ""
	if c != nil && c.Machine != nil {
		caseKind = c.Machine.Kind
	}
	return firstOf(flag, caseKind, api.MachineWindowsSandbox)
}

// WatchesHost says whether a clean-machine run can write to the machine
// running the tool, so that its leak check means something: a Windows
// Sandbox is given the marketplace and the tools read-only and only its
// results folder writable - what changes on the host while it runs is
// someone else's (another session, the dev hub) - while the local kind runs
// on this machine.
func WatchesHost(c *api.Case, flag string) bool {
	return KindOf(c, flag) != api.MachineWindowsSandbox
}

// sandboxStages are a Sandbox run's stages: its preparation, then the case's.
func sandboxStages(m *api.CaseMachine) []api.MachineStage { return sandboxStagesOf(m, false) }

// sandboxStagesOf are a Sandbox run's stages, its preparation taken from the
// preparation cache when cached.
func sandboxStagesOf(m *api.CaseMachine, cached bool) []api.MachineStage {
	prepare := m.Prepare
	if prepare == nil && needsBash(m.Stages) {
		p := []string{api.MachinePrepareWinget, api.MachinePrepareVCRedist, api.MachinePrepareBash}
		prepare = &p
	}
	return append(prepareStages(prepare, cached), m.Stages...)
}

// resolveCases gives each case stage the stages it runs and that case's user
// file, read beside the case running now (the clean machine sees only its
// tools): a stage id that case lacks is refused.
func resolveCases(c *api.Case, stages []api.MachineStage) ([]api.MachineStage, error) {
	out := make([]api.MachineStage, len(stages))
	for i, s := range stages {
		out[i] = s
		if s.Case == nil {
			continue
		}
		other, err := api.Load(filepath.Join(filepath.Dir(c.Dir), s.Case.Name))
		if err != nil {
			return nil, fmt.Errorf("stage %s: case %s: %w", s.ID, s.Case.Name, err)
		}
		mc := *s.Case
		mc.Resolved = nil
		if len(mc.Stages) == 0 {
			mc.Resolved = append(mc.Resolved, other.Stages...)
		}
		for _, id := range mc.Stages {
			k := other.StageIndex(id)
			if k < 0 {
				return nil, fmt.Errorf("stage %s: case %s has no stage %s", s.ID, s.Case.Name, id)
			}
			mc.Resolved = append(mc.Resolved, other.Stages[k])
		}
		if other.User != "" {
			raw, err := os.ReadFile(filepath.Join(other.Dir, other.User))
			if err != nil {
				return nil, fmt.Errorf("stage %s: case %s's user file: %w", s.ID, s.Case.Name, err)
			}
			mc.User = string(raw)
		}
		out[i].Case = &mc
	}
	return out, nil
}

// channel is the infra channel folder the run is given: it must hold the
// installer a person double-clicks there.
func (o Options) channel() (string, error) {
	ch := o.Channel
	if ch == "" {
		ch = o.Case.Machine.Channel
		if ch != "" && !filepath.IsAbs(ch) {
			ch = filepath.Join(o.Case.Dir, ch)
		}
	}
	if ch == "" {
		return "", fmt.Errorf("no channel: give --channel <marketplace>/infra (or machine.channel in the case)")
	}
	ch, err := filepath.Abs(ch)
	if err != nil {
		return "", err
	}
	launcher := "install.sh"
	if runtime.GOOS == "windows" {
		launcher = "install-on-windows.cmd"
	}
	if _, err := os.Stat(filepath.Join(ch, launcher)); err != nil {
		return "", fmt.Errorf("%s is not an infra channel folder: no %s in it", ch, launcher)
	}
	return ch, nil
}

// Report prints each stage with its outcome and duration, then the total.
func Report(w io.Writer, r Results) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "STAGE\tSTATUS\tDURATION\tTITLE")
	for _, s := range r.Stages {
		d := "-"
		if s.Status != Skipped {
			d = s.Duration().Round(100 * time.Millisecond).String()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.ID, s.Status, d, s.Title)
	}
	_ = tw.Flush()
	verdict := "FAILED"
	if r.Passed {
		verdict = "passed"
	}
	fmt.Fprintf(w, "%s on %s (%s): %s in %s\n", r.Case, r.Kind, r.Machine, verdict, (time.Duration(r.DurationMS) * time.Millisecond).Round(100*time.Millisecond))
	for _, s := range r.Stages {
		if s.Status == Failed {
			fmt.Fprintf(w, "\nstage %s failed: %s\n  command: %s\n%s\n", s.ID, s.Failure, s.Command, s.Output)
		}
	}
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
