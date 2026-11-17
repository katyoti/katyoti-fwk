// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// : a clean machine's case stage runs another case's stages on
// the installed universe - one definition for a scenario universe and a clean
// machine. The plan carries those stages and that case's user file (the
// machine sees only its tools); a Sandbox is given bash for their checks; the
// runner finds the universe, its hub's data and its project through the
// installation and hands them to the case runner.

func writeCaseFile(t *testing.T, dir, body string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, api.CaseFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, s := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestACaseStageCarriesTheOtherCasesStagesAndUser(t *testing.T) {
	cases := t.TempDir()
	writeCaseFile(t, filepath.Join(cases, "study"), "name: study\nuser: user.md\nuniverse: {sidecars: [ktw-agents]}\nstages:\n"+
		"  - id: s1\n    steps: [{run: {cmd: 'true'}}]\n  - id: s2\n    steps: [{run: {cmd: 'true'}}]\n  - id: s3\n    steps: [{run: {cmd: 'true'}}]\n",
		map[string]string{"user.md": "I am the architect."})
	writeCaseFile(t, filepath.Join(cases, "proof"), "name: proof\nmachine:\n  stages:\n    - {id: install, install: true}\n"+
		"    - {id: study, case: {name: study, stages: [s1, s3]}}\n", nil)
	proof, err := api.Load(filepath.Join(cases, "proof"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveCases(proof, proof.Machine.Stages)
	if err != nil {
		t.Fatal(err)
	}
	mc := got[1].Case
	if mc == nil || len(mc.Resolved) != 2 || mc.Resolved[0].ID != "s1" || mc.Resolved[1].ID != "s3" || mc.User != "I am the architect." {
		t.Fatalf("the case stage carries %+v, want s1 and s3 and the user file", mc)
	}
	if proof.Machine.Stages[1].Case.Resolved != nil {
		t.Error("resolving changed the case's own definition")
	}

	// The plan written for the machine keeps them.
	plan := filepath.Join(t.TempDir(), "plan.json")
	if err := WritePlan(plan, Plan{Case: "proof", Stages: got}); err != nil {
		t.Fatal(err)
	}
	back, err := LoadPlan(plan)
	if err != nil || len(back.Stages[1].Case.Resolved) != 2 || back.Stages[1].Case.User == "" {
		t.Fatalf("the plan read back: %v %+v", err, back.Stages[1].Case)
	}

	// A stage the other case lacks, or a case that is not there, is refused.
	proof.Machine.Stages[1].Case.Stages = []string{"s9"}
	if _, err := resolveCases(proof, proof.Machine.Stages); err == nil || !strings.Contains(err.Error(), "no stage s9") {
		t.Errorf("an unknown stage: %v", err)
	}
	proof.Machine.Stages[1].Case = &api.MachineCase{Name: "nowhere"}
	if _, err := resolveCases(proof, proof.Machine.Stages); err == nil {
		t.Error("a missing case was accepted")
	}

	// A Sandbox running case stages is given bash, after winget; a case
	// listing its preparations is taken at its word.
	sb := sandboxStages(&api.CaseMachine{Stages: got})
	if len(sb) < 4 || sb[0].ID != "prepare-winget" || sb[2].ID != "prepare-bash" || !strings.Contains(strings.Join(sb[2].Cmd, " "), "--id Git.Git") {
		t.Errorf("the preparation of a case stage run: %+v", sb[:3])
	}
	only := []string{api.MachinePrepareWinget}
	if sb := sandboxStages(&api.CaseMachine{Stages: got, Prepare: &only}); sb[1].ID != "install" {
		t.Errorf("an explicit preparation was extended: %+v", sb[:2])
	}
}

func TestACaseStageIsChecked(t *testing.T) {
	for name, body := range map[string]string{
		"two kinds": "name: proof\nmachine:\n  stages: [{id: a, cmd: [x], case: {name: study}}]\n",
		"no name":   "name: proof\nmachine:\n  stages: [{id: a, case: {stages: [s1]}}]\n",
		"bash only": "name: proof\nmachine:\n  prepare: [bash]\n  stages: [{id: install, install: true}]\n",
	} {
		dir := t.TempDir()
		writeCaseFile(t, dir, body, nil)
		if _, err := api.Load(dir); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestTheRunnerHandsACaseStageItsInstalledUniverse(t *testing.T) {
	dir := t.TempDir()
	inst := cenv.NewInstallation(filepath.Join(dir, "inst"), "archifactory", "")
	inst.Settings = map[string]any{"default_universe": "archifactory"}
	home := filepath.Join(dir, "universes", "archifactory")
	if err := os.MkdirAll(inst.ConfigRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	reg := "universes:\n  - name: archifactory\n    home_path: " + filepath.ToSlash(home) + "\n"
	if err := os.WriteFile(filepath.Join(inst.ConfigRoot(), "universes.yaml"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen CaseTarget
	var ran api.MachineCase
	results := t.TempDir()
	r := &runner{inst: inst, results: results, world: World{RunCase: func(_ context.Context, c api.MachineCase, tg CaseTarget, out io.Writer) error {
		seen, ran = tg, c
		_, _ = io.WriteString(out, "stage s1 passed\n")
		return nil
	}}}
	stage := api.MachineStage{ID: "study", Case: &api.MachineCase{Name: "study", Resolved: []api.Stage{{ID: "s1"}}}}
	sr := r.stage(context.Background(), stage)
	if sr.Status != Passed || !strings.Contains(sr.Output, "stage s1 passed") {
		t.Fatalf("the case stage: %+v", sr)
	}
	if seen.Universe != "archifactory" || filepath.Clean(seen.Home) != filepath.Clean(home) ||
		seen.Data != filepath.Join(inst.DataRoot(), "katyoti", "universes", "archifactory") || seen.Work != filepath.Join(results, "case-study") || ran.Name != "study" {
		t.Fatalf("the target handed over: %+v (case %s)", seen, ran.Name)
	}

	// A failing stage fails the machine stage with its reason; no runner, or
	// no installation yet, says so.
	r.world.RunCase = func(context.Context, api.MachineCase, CaseTarget, io.Writer) error {
		return errors.New("stage s1: a check did not hold")
	}
	if sr := r.stage(context.Background(), stage); sr.Status != Failed || sr.Failure != "stage s1: a check did not hold" {
		t.Errorf("a failing case stage: %+v", sr)
	}
	r.world.RunCase = nil
	if sr := r.stage(context.Background(), stage); sr.Status != Failed || !strings.Contains(sr.Failure, "cannot run case stages") {
		t.Errorf("no case runner: %+v", sr)
	}
	r = &runner{results: results, world: World{RunCase: func(context.Context, api.MachineCase, CaseTarget, io.Writer) error { return nil }}}
	var buf bytes.Buffer
	r.live = &buf
	if sr := r.stage(context.Background(), stage); sr.Status != Failed || !strings.Contains(sr.Failure, "once the installer ran") {
		t.Errorf("before the installer: %+v", sr)
	}
}

// A case stage keeps, pass or fail, what the installed
// universe wrote during it - its logs and the journal of each task run it
// started - in its work folder, never a session from before the stage.
func TestACaseStageKeepsTheUniverseLogsAndItsRunsJournals(t *testing.T) {
	dir := t.TempDir()
	inst := cenv.NewInstallation(filepath.Join(dir, "inst"), "archifactory", "")
	inst.Settings = map[string]any{"default_universe": "archifactory"}
	home := filepath.Join(dir, "universes", "archifactory")
	if err := os.MkdirAll(inst.ConfigRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	reg := "universes:\n  - name: archifactory\n    home_path: " + filepath.ToSlash(home) + "\n"
	if err := os.WriteFile(filepath.Join(inst.ConfigRoot(), "universes.yaml"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(inst.DataRoot(), "katyoti", "universes", "archifactory")
	write := func(rel, body string, at time.Time) {
		p := filepath.Join(data, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if !at.IsZero() {
			_ = os.Chtimes(p, at, at)
			_ = os.Chtimes(filepath.Dir(p), at, at)
		}
	}
	old := time.Now().Add(-time.Hour)
	write("sessions/before_generate/llm.jsonl", "old run", old)
	write("logs/hub.log", "hub", old)
	results := t.TempDir()
	r := &runner{inst: inst, results: results, world: World{RunCase: func(context.Context, api.MachineCase, CaseTarget, io.Writer) error {
		write("sessions/s4_generate-asis-v1/llm.jsonl", "the run's journal", time.Time{})
		write("logs/ktw-agents.log", "agents", time.Time{})
		return errors.New("stage s4: timed out")
	}}}
	stage := api.MachineStage{ID: "m19", Case: &api.MachineCase{Name: "study", Resolved: []api.Stage{{ID: "s4"}}}}
	if sr := r.stage(context.Background(), stage); sr.Status != Failed {
		t.Fatalf("the failing case stage passed: %+v", sr)
	}
	kept := filepath.Join(results, "case-m19", EvidenceDir)
	for _, rel := range []string{"logs/hub.log", "logs/ktw-agents.log", "sessions/s4_generate-asis-v1/llm.jsonl"} {
		if _, err := os.Stat(filepath.Join(kept, rel)); err != nil {
			t.Errorf("%s not kept: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(kept, "sessions", "before_generate")); err == nil {
		t.Errorf("a session from before the stage was kept")
	}
}

// The evidence leaves the machine, so no secret goes with
// it - secret files, the raw LLM call log and tool results are never copied,
// every copied file is redacted, also of the values of the installation's
// and the universe's .env whatever their key's name.
func TestACaseStageEvidenceCarriesNoSecret(t *testing.T) {
	dir := t.TempDir()
	inst := cenv.NewInstallation(filepath.Join(dir, "inst"), "archifactory", "")
	inst.Settings = map[string]any{"default_universe": "archifactory"}
	home := filepath.Join(dir, "universes", "archifactory")
	const shaped = "sk-ant-fakeFAKEfake0123456789abcdef" // secret-scan: fake
	const plain = "fixture-plain-value-4242424242"
	put := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put(filepath.Join(inst.ConfigRoot(), "universes.yaml"), "universes:\n  - name: archifactory\n    home_path: "+filepath.ToSlash(home)+"\n")
	// Fixture .env files written by this test, fake values only.
	put(filepath.Join(inst.Dir(), ".env"), "ANTHROPIC_API_KEY="+shaped+"\n")
	put(filepath.Join(application.EnvOf(home).HomeDir(), ".env"), "PLAIN_SETTING=\""+plain+"\"\n")
	data := filepath.Join(inst.DataRoot(), "katyoti", "universes", "archifactory")
	results := t.TempDir()
	r := &runner{inst: inst, results: results, world: World{RunCase: func(context.Context, api.MachineCase, CaseTarget, io.Writer) error {
		put(filepath.Join(data, "sessions", "s4", "llm-model-calls.jsonl"), `{"body":"`+shaped+` `+plain+`"}`)
		put(filepath.Join(data, "sessions", "s4", "tool-results", "r1.txt"), "ANTHROPIC_API_KEY="+shaped)
		put(filepath.Join(data, "sessions", "s4", "llm.jsonl"), `{"text":"saw `+plain+`"}`)
		put(filepath.Join(data, "sessions", "s4", "vault.json"), plain)
		put(filepath.Join(data, "logs", "hub.log"), "boot\nkey "+shaped+" setting "+plain+"\n")
		put(filepath.Join(data, "logs", ".env"), "X="+shaped)
		put(filepath.Join(data, "logs", "hub.key"), shaped)
		return errors.New("stage s4: timed out")
	}}}
	stage := api.MachineStage{ID: "m19", Case: &api.MachineCase{Name: "study", Resolved: []api.Stage{{ID: "s4"}}}}
	r.stage(context.Background(), stage)
	kept := filepath.Join(results, "case-m19", EvidenceDir)
	for _, rel := range []string{"logs/hub.log", "sessions/s4/llm.jsonl"} {
		if _, err := os.Stat(filepath.Join(kept, rel)); err != nil {
			t.Errorf("%s not kept: %v", rel, err)
		}
	}
	for _, rel := range []string{"sessions/s4/llm-model-calls.jsonl", "sessions/s4/tool-results", "sessions/s4/vault.json", "logs/.env", "logs/hub.key"} {
		if _, err := os.Stat(filepath.Join(kept, rel)); err == nil {
			t.Errorf("%s was copied", rel)
		}
	}
	_ = filepath.WalkDir(results, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, secret := range []string{shaped, plain} {
			if strings.Contains(string(b), secret) {
				t.Errorf("%s carries a secret", p)
			}
		}
		return nil
	})
}

// A file over the per-file cap keeps its end, and a stage
// copies no more than its total.
func TestEvidenceKeepsALongLogsEndWithinTheTotal(t *testing.T) {
	src := t.TempDir()
	long := strings.Repeat("early line\n", 100) + "the last line\n"
	if err := os.WriteFile(filepath.Join(src, "a.log"), []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	body, ok := readTail(filepath.Join(src, "a.log"), 40)
	if !ok || !strings.HasSuffix(string(body), "the last line\n") || strings.Count(string(body), "early line") > 3 {
		t.Fatalf("tail: %q", body)
	}
	if err := os.WriteFile(filepath.Join(src, "b.log"), []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	ev := &evidence{left: int64(len(long)) + 10}
	ev.copy(src, dst)
	var total int64
	_ = filepath.WalkDir(dst, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			info, _ := d.Info()
			total += info.Size()
		}
		return nil
	})
	if total > int64(len(long))+10+200 {
		t.Errorf("copied %d bytes, over the total", total)
	}
}
