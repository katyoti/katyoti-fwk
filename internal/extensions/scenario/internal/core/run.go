// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// RUN — the stages from a point to a point. Without --from a run continues
// after the last stage that passed; with --from it first restores the
// checkpoint just before that stage. Each stage runs its steps, then its
// checks; a stage that passes is committed (git) and checkpointed, the next
// one starts. The first stage that fails stops the run: its report says which
// step or check failed and what was seen, the universe is left running for
// inspection, and `run --from <that stage>` replays exactly that stage once
// the cause is fixed.

// RunOptions picks the stages to run.
type RunOptions struct {
	From, To     string
	NoCheckpoint bool
	// RefreshBin copies the binaries again first (a fix was just built).
	RefreshBin bool
}

// Run runs stages; it answers the stage that failed ("" when all passed).
func (e *Engine) Run(ctx context.Context, opts RunOptions) (string, error) {
	st, err := loadState(e.RT)
	if err != nil {
		return "", err
	}
	if !st.Installed {
		return "", fmt.Errorf("the case is not installed (scenario install %s)", e.Case.Name)
	}
	if opts.RefreshBin {
		if err := e.RefreshBinaries(ctx); err != nil {
			return "", err
		}
	}
	if err := e.CheckFresh(); err != nil {
		return "", err
	}
	start := 0
	if opts.From != "" {
		start = e.Case.StageIndex(opts.From)
		if start < 0 {
			return "", fmt.Errorf("no stage %q", opts.From)
		}
		if err := e.Restore(ctx, e.checkpointBefore(opts.From)); err != nil {
			return "", err
		}
		if st, err = loadState(e.RT); err != nil {
			return "", err
		}
	} else {
		for start < len(e.Case.Stages) && slices.Contains(st.Passed, e.Case.Stages[start].ID) {
			start++
		}
	}
	end := len(e.Case.Stages) - 1
	if opts.To != "" {
		if end = e.Case.StageIndex(opts.To); end < 0 {
			return "", fmt.Errorf("no stage %q", opts.To)
		}
	}
	if start > end {
		e.logf("nothing to run: every stage up to %s passed", e.Case.Stages[end].ID)
		return "", nil
	}
	if err := e.Start(ctx); err != nil {
		return "", err
	}
	for i := start; i <= end; i++ {
		s := e.Case.Stages[i]
		ok, err := e.runStage(ctx, s, &st)
		if err != nil || !ok {
			st.Failure = fmt.Sprintf("%s: %v", s.ID, errOr(err, "a check did not hold"))
			_ = saveState(e.RT, st)
			e.logf("\nSTAGE %s FAILED: %s\nThe universe is left running for inspection.\nFix the cause, then replay it: ktw-compose scenario run %s --from %s", s.ID, errOr(err, "a check did not hold"), e.Case.Name, s.ID)
			return s.ID, nil
		}
		st.Failure = ""
		if !slices.Contains(st.Passed, s.ID) {
			st.Passed = append(st.Passed, s.ID)
		}
		if err := saveState(e.RT, st); err != nil {
			return s.ID, err
		}
		e.commit("stage " + s.ID + " passed")
		if !opts.NoCheckpoint {
			if err := e.Checkpoint(ctx, s.ID); err != nil {
				return s.ID, err
			}
		}
	}
	e.logf("\nall stages up to %s passed", e.Case.Stages[end].ID)
	return "", nil
}

func errOr(err error, s string) string {
	if err != nil {
		return err.Error()
	}
	return s
}

// runStage runs one stage's steps then checks.
func (e *Engine) runStage(ctx context.Context, s api.Stage, st *State) (bool, error) {
	rep, err := e.newReport(s)
	if err != nil {
		return false, err
	}
	defer rep.close()
	timeout := s.TimeoutMin
	if timeout <= 0 {
		timeout = 30
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Minute)
	b, err := e.connect(ctx)
	if err != nil {
		return false, err
	}
	defer b.Close()
	runsBefore := e.countRuns(ctx, b)

	rep.linef("# Stage %s — %s\n\n%s\n", s.ID, s.Title, strings.TrimSpace(s.Description))
	for i, step := range s.Steps {
		if time.Now().After(deadline) {
			rep.linef("TIMEOUT before step %d", i+1)
			return false, fmt.Errorf("stage timed out (%d min)", timeout)
		}
		label := stepLabel(step)
		rep.linef("\n## step %d — %s", i+1, label)
		if step.Note != "" {
			rep.linef("  %s", step.Note)
		}
		if err := e.runStep(ctx, b, step, st, rep, deadline); err != nil {
			rep.linef("  FAILED: %v", err)
			return false, fmt.Errorf("step %d (%s): %w", i+1, label, err)
		}
		_ = saveState(e.RT, *st)
		if s.MaxAgentRuns > 0 {
			if n := e.countRuns(ctx, b) - runsBefore; n > s.MaxAgentRuns {
				rep.linef("  BUDGET: %d agent runs started (max %d)", n, s.MaxAgentRuns)
				return false, fmt.Errorf("%d agent runs started, more than the stage's %d", n, s.MaxAgentRuns)
			}
		}
	}
	rep.linef("\n## checks")
	all := true
	for _, c := range s.Expect {
		ok, seen := e.check(ctx, b, c, *st)
		mark := "PASS"
		if !ok {
			mark, all = "FAIL", false
		}
		rep.linef("  %s %s — %s", mark, c.Describe(), seen)
	}
	verdict := "PASSED"
	if !all {
		verdict = "FAILED"
	}
	rep.linef("\nstage %s %s", s.ID, verdict)
	return all, nil
}

// runStep runs one step.
func (e *Engine) runStep(ctx context.Context, b *Bus, step api.Step, st *State, rep *report, deadline time.Time) error {
	switch {
	case step.Call != nil, step.Action != nil:
		cs, action := step.Call, false
		if step.Action != nil {
			cs, action = step.Action, true
		}
		raw, err := b.CallAny(ctx, cs.URI, e.expand(cs.Input, *st), action)
		if err != nil {
			if cs.MayFail {
				rep.linef("  refused (may fail): %s", tail(err.Error(), 400))
				return nil
			}
			return err
		}
		rep.linef("  → %s", tail(string(raw), 400))
		if cs.Save != "" {
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				v = string(raw)
			}
			if st.Saved == nil {
				st.Saved = map[string]any{}
			}
			st.Saved[cs.Save] = v
		}
		return nil
	case step.AnswerInbox != nil:
		return e.answerInbox(ctx, b, *step.AnswerInbox, st, rep, deadline)
	case step.StartTask != nil:
		return e.startTask(ctx, b, *step.StartTask, st, rep)
	case step.AnswerForm != nil:
		return e.answerForm(ctx, b, *step.AnswerForm, st, rep, deadline)
	case step.Wait != nil:
		end := deadline
		if t := step.Wait.TimeoutMin; t > 0 {
			if d := time.Now().Add(time.Duration(t) * time.Minute); d.Before(end) {
				end = d
			}
		}
		for {
			ok, seen := e.check(ctx, b, step.Wait.Until, *st)
			if ok {
				rep.linef("  holds: %s — %s", step.Wait.Until.Describe(), seen)
				return nil
			}
			if time.Now().After(end) {
				return fmt.Errorf("timed out: %s — last seen: %s", step.Wait.Until.Describe(), seen)
			}
			time.Sleep(5 * time.Second)
		}
	case step.Find != nil:
		f := step.Find
		items, err := e.listing(ctx, b, f.URI, f.Input, f.Items, f.Where, *st)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return fmt.Errorf("find: nothing in %s %s matches %v", f.URI, f.Items, f.Where)
		}
		if st.Saved == nil {
			st.Saved = map[string]any{}
		}
		st.Saved[f.Save] = items[0]
		rep.linef("  found: %s", tail(jsonText(items[0]), 300))
		return nil
	case step.Each != nil:
		ea := step.Each
		items, err := e.listing(ctx, b, ea.URI, ea.Input, ea.Items, ea.Where, *st)
		if err != nil {
			return err
		}
		rep.linef("  %d element(s)", len(items))
		if st.Saved == nil {
			st.Saved = map[string]any{}
		}
		defer delete(st.Saved, eachItem)
		for i, it := range items {
			st.Saved[eachItem] = it
			rep.linef("  ### element %d — %s", i+1, tail(jsonText(it), 160))
			for j, sub := range ea.Steps {
				rep.linef("  - %s", stepLabel(sub))
				if err := e.runStep(ctx, b, sub, st, rep, deadline); err != nil {
					return fmt.Errorf("element %d, step %d: %w", i+1, j+1, err)
				}
			}
		}
		return nil
	case step.Open != "":
		// A headless run has no GUI: the address is recorded, already
		// checked at load; a GUI-driven stage hands it to its driver.
		rep.linef("  open %s (headless: recorded)", api.WithUniverse(step.Open, e.universeName()))
		return nil
	case step.Run != nil:
		code, out := e.runCmd(ctx, *step.Run)
		want := 0
		if step.Run.Exit != nil {
			want = *step.Run.Exit
		}
		rep.linef("  exit %d: %s", code, tail(out, 600))
		if code != want {
			return fmt.Errorf("exit %d, want %d", code, want)
		}
		return nil
	}
	return fmt.Errorf("empty step")
}

func stepLabel(s api.Step) string {
	switch {
	case s.Call != nil:
		return "call " + s.Call.URI
	case s.Action != nil:
		return "action " + s.Action.URI
	case s.AnswerInbox != nil:
		return "the simulated user answers the Inbox"
	case s.StartTask != nil:
		return "start " + s.StartTask.Task + " attended"
	case s.AnswerForm != nil:
		return "answer the form " + s.AnswerForm.Question
	case s.Wait != nil:
		return "wait for " + s.Wait.Until.Describe()
	case s.Run != nil:
		return "run " + s.Run.Cmd
	case s.Find != nil:
		return "find in " + s.Find.URI + " -> " + s.Find.Save
	case s.Each != nil:
		return "each in " + s.Each.URI
	case s.Open != "":
		return "open " + s.Open
	}
	return "?"
}

// eachItem is where an each step keeps its current element: `${item.<path>}`.
const eachItem = "item"

// listing calls a method and answers the elements of the list at the dot path
// items in its answer that match where (each field contains the value,
// case-insensitive).
func (e *Engine) listing(ctx context.Context, b *Bus, uri string, input any, items string, where map[string]string, st State) ([]any, error) {
	raw, err := b.Call(ctx, uri, e.expand(input, st))
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	list, ok := walk(v, items).([]any)
	if !ok {
		return nil, fmt.Errorf("%s: no list at %q", uri, items)
	}
	var out []any
	for _, it := range list {
		if matches(it, where, func(s string) string { return fmt.Sprint(e.expand(s, st)) }) {
			out = append(out, it)
		}
	}
	return out, nil
}

// matches is whether each field of where (a dot path) of it contains its
// value, case-insensitive.
func matches(it any, where map[string]string, expand func(string) string) bool {
	for k, want := range where {
		got := walk(it, k)
		s, isStr := got.(string)
		if !isStr {
			s = jsonText(got)
		}
		if !strings.Contains(strings.ToLower(s), strings.ToLower(expand(want))) {
			return false
		}
	}
	return true
}

// walk follows a dot path through maps (and list indexes); nil when absent.
func walk(v any, path string) any {
	for _, p := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[p]
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// countRuns counts the universe's runs (all groups) that are work: the
// stage's spend guard.
func (e *Engine) countRuns(ctx context.Context, b *Bus) int {
	raw, err := b.Call(ctx, "ktw-agents:tasks/Runs", map[string]any{})
	if err != nil {
		return 0
	}
	var out struct {
		Runs []struct {
			Origin *taskapi.RunOrigin `json:"origin"`
		} `json:"runs"`
	}
	_ = json.Unmarshal(raw, &out)
	origins := make([]*taskapi.RunOrigin, len(out.Runs))
	for i, r := range out.Runs {
		origins[i] = r.Origin
	}
	return workRuns(origins)
}

// workRuns counts the runs that are work — started by a person, the planner,
// an agent or a workflow — leaving out the universe's own: the planner's
// machine (system), started again at each start of the universe (every
// checkpoint restarts it), and the tasks it keeps alive. A stage's budget
// counts what the stage made run, not the universe restarting.
func workRuns(origins []*taskapi.RunOrigin) int {
	n := 0
	for _, o := range origins {
		if o != nil && (o.Kind == taskapi.OriginSystem || o.Kind == taskapi.OriginKeepAlive) {
			continue
		}
		n++
	}
	return n
}

// report is a stage's report, written as it goes (runs/<time>-<stage>.md)
// and echoed to the output.
type report struct {
	f   *os.File
	e   *Engine
	Pth string
}

func (e *Engine) newReport(s api.Stage) (*report, error) {
	if err := os.MkdirAll(e.RT.Runs(), 0o755); err != nil {
		return nil, err
	}
	p := filepath.Join(e.RT.Runs(), time.Now().Format("20060102-150405")+"-"+s.ID+".md")
	f, err := os.Create(p)
	if err != nil {
		return nil, err
	}
	e.logf("report: %s", p)
	return &report{f: f, e: e, Pth: p}, nil
}

func (r *report) linef(format string, a ...any) {
	line := fmt.Sprintf(format, a...)
	fmt.Fprintln(r.f, line)
	fmt.Fprintln(r.e.Out, line)
}

func (r *report) close() { _ = r.f.Close() }
