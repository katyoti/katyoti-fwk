// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// The helper process: the test binary run again as the fakes of a clean
// machine — the installer the channel ships, and the installed programs.
const helperEnv = "KTW_MACHINE_TEST_HELPER"

func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) == "" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	os.Exit(helper(args))
}

func helper(args []string) int {
	switch args[0] {
	case "install":
		// What the real installer leaves: an installation (its katyoti.yml,
		// default universe and solution) and a sidecar in its store.
		dir := os.Getenv("KTW_INSTALLATION_DIR")
		if dir == "" {
			dir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Katyoti", "archifactory-v9")
		}
		store := filepath.Join(dir, "root", "bin", "store", "ktw-agents", "v9")
		_ = os.MkdirAll(store, 0o755)
		_ = os.WriteFile(filepath.Join(store, exe("ktw-agents")), nil, 0o755)
		yml := "name: archifactory v9\nsolution: archifactory@v9\nroots:\n  config: root\n  data: data\n  cache: cache\nsettings:\n  default_universe: archifactory\n"
		if err := os.WriteFile(filepath.Join(dir, "katyoti.yml"), []byte(yml), 0o644); err != nil {
			fmt.Println(err)
			return 1
		}
		fmt.Printf("setup args=%s home=%s\n", os.Getenv("KTW_SETUP_ARGS"), os.Getenv("USERPROFILE"))
		return 0
	case "echo":
		fmt.Println(strings.Join(args[1:], " "))
		for _, k := range []string{"USERPROFILE", "LOCALAPPDATA", "KTW_KATYOTI_ROOT", "ANTHROPIC_API_KEY", "STAGE_ENV"} {
			fmt.Printf("%s=%s\n", k, os.Getenv(k))
		}
		return 0
	case "path":
		fmt.Println("PATH=" + os.Getenv("PATH"))
		return 0
	case "sleep":
		ms, _ := strconv.Atoi(args[1])
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return 0
	case "png":
		// A screenshot saved to args[1]: blank (one colour, as a screen
		// that printed nothing) or showing something (a pattern).
		img := image.NewRGBA(image.Rect(0, 0, 320, 200))
		for y := 0; y < 200; y++ {
			for x := 0; x < 320; x++ {
				c := color.RGBA{255, 255, 255, 255}
				if args[2] == "shows" {
					c = color.RGBA{uint8(x * 7), uint8(y * 13), uint8(x ^ y), 255}
				}
				img.Set(x, y, c)
			}
		}
		f, err := os.Create(args[1])
		if err != nil {
			fmt.Println(err)
			return 1
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			fmt.Println(err)
			return 1
		}
		fmt.Println("saved", args[1])
		return 0
	case "exit":
		code, _ := strconv.Atoi(args[1])
		fmt.Println("exiting", code)
		return code
	}
	return 2
}

// call is a stage command running the helper.
func call(args ...string) []string {
	return append([]string{os.Args[0], "-test.run=TestHelperProcess", "--"}, args...)
}

// fakeChannel is a channel folder whose installer double-click runs the
// helper's install.
func fakeChannel(t *testing.T) string {
	t.Helper()
	ch := filepath.Join(t.TempDir(), "market", "infra")
	if err := os.MkdirAll(ch, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(ch, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("install-on-windows.cmd", "@echo off\r\n\"%KTW_MACHINE_TEST_BIN%\" -test.run=TestHelperProcess -- install\r\n")
	write("install.sh", "#!/bin/sh\n\"$KTW_MACHINE_TEST_BIN\" -test.run=TestHelperProcess -- install\n")
	return ch
}

func helperEnvList() []string {
	return []string{helperEnv + "=1", "KTW_MACHINE_TEST_BIN=" + os.Args[0]}
}

// The local kind runs the stages on this machine closed in a world of its
// own: the installer double-clicked headless with its installation in the
// world, every later stage reading the installation it made (${katyoti},
// ${universe}, ${solution}, ${bin:…}), a local_cmd replacing a real install,
// nothing of the caller's installation or keys reaching a stage, each stage
// timed; the first failure skips the rest.
func TestTheLocalKindRunsTheStagesInScopedRoots(t *testing.T) {
	long.Test(t)
	hostRoot := filepath.Join(t.TempDir(), "host-root")
	c := &api.Case{Name: "proof", Dir: t.TempDir(), Machine: &api.CaseMachine{
		Kind: api.MachineLocal, PassEnv: []string{helperEnv, "KTW_MACHINE_TEST_BIN", "PROOF_PASSWORD"},
		Stages: []api.MachineStage{
			{ID: "install", Title: "double-click", Install: true, Contains: []string{"setup args=--no-gui"}},
			{ID: "vars", Cmd: call("echo", "${katyoti}", "${universe}", "${solution}", "${bin:ktw-agents}", "${env:PROOF_PASSWORD}"),
				Env:      map[string]string{"STAGE_ENV": "in ${install}"},
				Contains: []string{"archifactory archifactory@v9", "s3cret", "STAGE_ENV=in ${install}", "USERPROFILE=${home}"},
				Excludes: []string{hostRoot, "sk-host"}},
			{ID: "prereqs", Cmd: call("exit", "3"), LocalCmd: call("echo", "dry-run")},
			{ID: "slow", Cmd: call("sleep", "300")},
			{ID: "broken", Cmd: call("exit", "4")},
			{ID: "never", Cmd: call("echo", "never")},
		},
	}}
	env := append(os.Environ(), helperEnvList()...)
	env = append(env, "PROOF_PASSWORD=s3cret", "KTW_KATYOTI_ROOT="+hostRoot, "ANTHROPIC_API_KEY=sk-host")
	dir := filepath.Join(t.TempDir(), "clean-machine")
	res, err := Prove(context.Background(), Options{Case: c, Channel: fakeChannel(t), Dir: dir, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{Passed, Passed, Passed, Passed, Failed, Skipped}
	for i, s := range res.Stages {
		if s.Status != want[i] {
			t.Errorf("stage %s: %s, want %s (%s)\n%s", s.ID, s.Status, want[i], s.Failure, s.Output)
		}
	}
	if res.Passed || !res.Done {
		t.Errorf("a run with a failed stage: passed=%v done=%v", res.Passed, res.Done)
	}
	world := filepath.Join(dir, "world")
	if _, err := os.Stat(filepath.Join(world, "install", "katyoti.yml")); err != nil {
		t.Errorf("the installation should be in the world: %v", err)
	}
	vars := res.Stages[1].Output
	for _, w := range []string{filepath.Join(world, "install", "root", "control-plane", exe("katyoti")),
		filepath.Join(world, "install", "root", "bin", "store", "ktw-agents", "v9", exe("ktw-agents"))} {
		if !strings.Contains(vars, w) {
			t.Errorf("the stage should read %s:\n%s", w, vars)
		}
	}
	if !strings.Contains(res.Stages[2].Output, "dry-run") {
		t.Errorf("the local kind should run local_cmd:\n%s", res.Stages[2].Output)
	}
	if !strings.Contains(res.Stages[4].Failure, "exit 4, want 0") {
		t.Errorf("failure = %q", res.Stages[4].Failure)
	}
	if d := res.Stages[3].Duration(); d < 300*time.Millisecond {
		t.Errorf("the slow stage took %s: its duration is not measured", d)
	}
	// The results file is what a reader gets: the same stages, timed.
	got, err := LoadResults(filepath.Join(dir, "results", ResultsFile))
	if err != nil || len(got.Stages) != 6 || got.Stages[3].DurationMS < 300 || !got.Done {
		t.Errorf("results file: %v %+v", err, got)
	}
}

// On the clean machine itself (a Sandbox) the installation is where a person's
// double-click puts it: found under the programs folder, not named by the
// world. Its variables are unknown before the installer ran.
func TestOnTheMachineItselfTheInstallationIsFound(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the programs folder searched is Windows'")
	}
	home := t.TempDir()
	local := filepath.Join(home, "AppData", "Local")
	w := World{Env: append(Clean(os.Environ()), append(helperEnvList(), "LOCALAPPDATA="+local)...), Home: home}
	plan := Plan{Case: "proof", Kind: api.MachineWindowsSandbox, Channel: fakeChannel(t), Stages: []api.MachineStage{
		{ID: "too-early", Cmd: call("echo", "${katyoti}")},
	}}
	res, err := Run(context.Background(), plan, w, t.TempDir(), nil)
	if err != nil || res.Stages[0].Status != Failed || !strings.Contains(res.Stages[0].Failure, "unknown until the installer ran") {
		t.Fatalf("a variable of the installation before it: %v %+v", err, res.Stages[0])
	}
	plan.Stages = []api.MachineStage{{ID: "install", Install: true}, {ID: "u", Cmd: call("echo", "${install}"), Contains: []string{filepath.Join(local, "Programs", "Katyoti", "archifactory-v9")}}}
	res, err = Run(context.Background(), plan, w, t.TempDir(), nil)
	if err != nil || !res.Passed {
		t.Fatalf("installation not found: %v %+v", err, res.Stages)
	}
}

// The .wsb a run writes maps the channel's marketplace and the tools
// read-only, the results writable, and runs the bootstrap at logon; the plan
// names the channel as the Sandbox sees it.
func TestTheSandboxRunMapsTheChannelAndRunsTheBootstrap(t *testing.T) {
	ch := fakeChannel(t)
	dir := filepath.Join(t.TempDir(), "clean-machine")
	runner := filepath.Join(t.TempDir(), "ktw-compose.exe")
	if err := os.WriteFile(runner, []byte("runner"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := SandboxRun{Dir: dir, Market: filepath.Dir(ch), Runner: runner, Plan: Plan{Case: "proof", Kind: api.MachineWindowsSandbox,
		Channel: SandboxMarket + `\infra`, Stages: []api.MachineStage{{ID: "install", Install: true}}}}
	file, err := run.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var c wsb
	if err := xml.Unmarshal(raw, &c); err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	want := map[string]struct {
		host string
		ro   bool
	}{SandboxMarket: {filepath.Dir(ch), true}, SandboxTools: {run.Tools(), true}, SandboxResults: {run.Results(), false}}
	if len(c.MappedFolders) != len(want) {
		t.Fatalf("mapped folders: %+v", c.MappedFolders)
	}
	for _, m := range c.MappedFolders {
		w, ok := want[m.SandboxFolder]
		if !ok || m.HostFolder != w.host || m.ReadOnly != w.ro {
			t.Errorf("mapped %+v, want %+v", m, w)
		}
	}
	if c.LogonCommand != `cmd.exe /c C:\Proof\tools\bootstrap.cmd` || c.Networking != "Default" {
		t.Errorf("logon %q networking %q", c.LogonCommand, c.Networking)
	}
	boot, _ := os.ReadFile(filepath.Join(run.Tools(), BootstrapFile))
	for _, w := range []string{`"C:\Proof\tools\ktw-compose.exe" scenario machine-stages "C:\Proof\tools\plan.json" --results "C:\Proof\results"`,
		`set "KTW_KATYOTI_ROOT=C:\Proof\runner\root"`, `> "C:\Proof\results\runner.log" 2>&1`} {
		if !strings.Contains(string(boot), w) {
			t.Errorf("bootstrap lacks %s:\n%s", w, boot)
		}
	}
	plan, err := LoadPlan(filepath.Join(run.Tools(), PlanFile))
	if err != nil || plan.Channel != `C:\Proof\market\infra` || len(plan.Stages) != 1 {
		t.Errorf("plan: %v %+v", err, plan)
	}
	if b, _ := os.ReadFile(filepath.Join(run.Tools(), RunnerExe)); string(b) != "runner" {
		t.Error("the runner was not copied into the tools")
	}
}

// A Windows Sandbox is given winget before the case's stages, as a person's
// Windows 10/11 desktop has it - timed as a "prepare-" stage, not the proof -
// unless the case says `prepare: []`; an unknown preparation is refused.
func TestASandboxIsPreparedWithWingetFirst(t *testing.T) {
	own := []api.MachineStage{{ID: "install", Install: true}}
	got := sandboxStages(&api.CaseMachine{Stages: own})
	if len(got) != 3 || got[0].ID != "prepare-winget" || got[1].ID != "prepare-vcredist" || got[2].ID != "install" {
		t.Fatalf("stages: %+v", got)
	}
	// The VC++ runtime a desktop usually has, machine-wide, through winget.
	if vc := strings.Join(got[1].Cmd, " "); !strings.Contains(vc, "winget install --id Microsoft.VCRedist.2015+.x64 --exact --source winget") || strings.Contains(vc, "--scope user") {
		t.Errorf("the runtime's preparation: %s", vc)
	}
	script := strings.Join(got[0].Cmd, " ")
	for _, w := range []string{"powershell", "Install-Module -Name Microsoft.WinGet.Client", "Repair-WinGetPackageManager -AllUsers", `WindowsApps\winget.exe" --version`} {
		if !strings.Contains(script, w) {
			t.Errorf("the preparation should run %q:\n%s", w, script)
		}
	}
	none := []string{}
	if got := sandboxStages(&api.CaseMachine{Stages: own, Prepare: &none}); len(got) != 1 {
		t.Errorf("prepare: [] should give the Sandbox nothing: %+v", got)
	}
	dir := t.TempDir()
	body := "name: proof\nmachine:\n  prepare: [chocolatey]\n  stages: [{id: install, install: true}]\n"
	if err := os.WriteFile(filepath.Join(dir, api.CaseFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Load(dir); err == nil || !strings.Contains(err.Error(), "machine.prepare") {
		t.Errorf("an unknown preparation: %v", err)
	}
	body = "name: proof\nmachine:\n  prepare: [vcredist]\n  stages: [{id: install, install: true}]\n"
	if err := os.WriteFile(filepath.Join(dir, api.CaseFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Load(dir); err == nil || !strings.Contains(err.Error(), "before it") {
		t.Errorf("vcredist without winget first: %v", err)
	}
}

// Without Windows Sandbox, the run fails before anything is written, saying
// how to enable it.
func TestAMissingSandboxSaysHowToEnableIt(t *testing.T) {
	c := &api.Case{Name: "proof", Machine: &api.CaseMachine{Stages: []api.MachineStage{{ID: "install", Install: true}}}}
	dir := filepath.Join(t.TempDir(), "clean-machine")
	_, err := Prove(context.Background(), Options{Case: c, Channel: fakeChannel(t), Dir: dir, Sandbox: filepath.Join(t.TempDir(), "WindowsSandbox.exe")})
	if !errors.Is(err, ErrNoSandbox) {
		t.Fatalf("err = %v", err)
	}
	for _, w := range []string{"Enable-WindowsOptionalFeature -Online -FeatureName Containers-DisposableClientVM -All", "restart Windows", "--kind local"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("the error should say %q:\n%v", w, err)
		}
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("a run without Sandbox wrote its folder")
	}
}

// Await reads the results the Sandbox writes, stage by stage, until done;
// past its timeout it fails with the end of the runner's log.
func TestAwaitReadsTheResultsWithTheirDurations(t *testing.T) {
	dir := t.TempDir()
	r := Results{Case: "proof", Stages: []StageResult{{ID: "install", Status: Passed, DurationMS: 61500}, {ID: "keys", Status: Skipped}}}
	if err := writeResults(dir, r); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		r.Stages[1] = StageResult{ID: "keys", Status: Passed, DurationMS: 2300}
		r.Done, r.Passed = true, true
		if err := writeResults(dir, r); err != nil {
			t.Errorf("the second write: %v", err)
		}
	}()
	var out strings.Builder
	got, err := Await(context.Background(), dir, 5*time.Second, 20*time.Millisecond, &out)
	if err != nil || !got.Done || got.Stages[0].Duration() != 61500*time.Millisecond || got.Stages[1].Duration() != 2300*time.Millisecond {
		t.Fatalf("%v %+v", err, got)
	}
	if !strings.Contains(out.String(), "install passed in 1m1.5s") || !strings.Contains(out.String(), "keys passed in 2.3s") {
		t.Errorf("each stage should be said with its duration:\n%s", out.String())
	}
	var rep strings.Builder
	Report(&rep, got)
	if !strings.Contains(rep.String(), "1m1.5s") || !strings.Contains(rep.String(), "passed in") {
		t.Errorf("report:\n%s", rep.String())
	}

	empty := t.TempDir()
	_ = os.WriteFile(filepath.Join(empty, RunnerLog), []byte("setup: no network\n"), 0o644)
	if _, err := Await(context.Background(), empty, 50*time.Millisecond, 10*time.Millisecond, nil); err == nil || !strings.Contains(err.Error(), "setup: no network") {
		t.Errorf("a timeout should end with the runner's log: %v", err)
	}
}

// A case's machine section is checked at load: its kind, its stages (one of
// install and cmd, ids unique); a machine-only case needs no universe.
func TestAMachineSectionIsChecked(t *testing.T) {
	load := func(body string) error {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, api.CaseFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := api.Load(dir)
		return err
	}
	if err := load("name: proof\nmachine:\n  stages:\n    - {id: install, install: true}\n    - {id: doctor, cmd: [katyoti, doctor]}\n"); err != nil {
		t.Errorf("a machine-only case: %v", err)
	}
	for name, body := range map[string]string{
		"kind":          "name: proof\nmachine:\n  kind: vm\n  stages: [{id: install, install: true}]\n",
		"both":          "name: proof\nmachine:\n  stages: [{id: install, install: true, cmd: [x]}]\n",
		"neither":       "name: proof\nmachine:\n  stages: [{id: install}]\n",
		"twice":         "name: proof\nmachine:\n  stages: [{id: a, install: true}, {id: a, cmd: [x]}]\n",
		"no stages":     "name: proof\nmachine: {kind: local}\n",
		"no universe":   "name: proof\nstages: [{id: s1}]\nmachine:\n  stages: [{id: install, install: true}]\n",
		"local install": "name: proof\nmachine:\n  stages: [{id: install, install: true, local_cmd: [x]}]\n",
	} {
		if err := load(body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// On the clean machine itself each stage gets the PATH a process started now
// gets - a person opens a new window after a stage installed programs - and a
// world closed in a folder keeps its own.
func TestEachStageGetsTheFreshPathOnTheMachine(t *testing.T) {
	fresh := filepath.FromSlash("/installed/by/a/stage")
	keep := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", keep) })
	plan := Plan{Case: "proof", Stages: []api.MachineStage{{ID: "p1", Cmd: call("path"), Contains: []string{fresh}}}}
	w := World{Env: append(os.Environ(), helperEnvList()...), Home: t.TempDir(),
		FreshPath: func(cur string) string { return fresh + string(os.PathListSeparator) + cur }}
	res, err := Run(context.Background(), plan, w, t.TempDir(), nil)
	if err != nil || !res.Passed {
		t.Fatalf("the stage should see the fresh PATH: %v %+v", err, res.Stages)
	}
	w.FreshPath = nil
	_ = os.Setenv("PATH", keep)
	res, _ = Run(context.Background(), plan, w, t.TempDir(), nil)
	if res.Passed {
		t.Errorf("without FreshPath the stage keeps the PATH it was given: %+v", res.Stages)
	}
}

// The host leak check is armed only where the run can write to the host: a
// Windows Sandbox cannot (what changes there meanwhile is someone else's), the
// local kind runs on this machine; --kind wins over the case.
func TestOnlyALocalRunWatchesTheHost(t *testing.T) {
	sandboxCase := &api.Case{Machine: &api.CaseMachine{Kind: api.MachineWindowsSandbox}}
	localCase := &api.Case{Machine: &api.CaseMachine{Kind: api.MachineLocal}}
	for _, c := range []struct {
		name string
		cs   *api.Case
		flag string
		want bool
	}{
		{"a sandbox case", sandboxCase, "", false},
		{"no kind: a sandbox", &api.Case{Machine: &api.CaseMachine{}}, "", false},
		{"a local case", localCase, "", true},
		{"--kind local over a sandbox case", sandboxCase, api.MachineLocal, true},
		{"--kind windows-sandbox over a local case", localCase, api.MachineWindowsSandbox, false},
	} {
		if got := WatchesHost(c.cs, c.flag); got != c.want {
			t.Errorf("%s: watches the host %v, want %v", c.name, got, c.want)
		}
	}
}

// A stage that saves a screenshot passes on what the image shows, never on
// what the command printed: a blank image (one colour, a screen that printed
// nothing) fails the stage though its output says "saved", a missing one
// fails, one that shows something passes; ${results} is the run's results
// folder.
func TestAScreenshotStagePassesOnlyOnAnImageThatShowsSomething(t *testing.T) {
	results := t.TempDir()
	w := World{Env: append(os.Environ(), helperEnvList()...), Home: t.TempDir()}
	shot := filepath.Join("${results}", "shot.png")
	for _, c := range []struct {
		name, kind string
		images     []string
		want       string
		failure    string
	}{
		{"blank", "blank", []string{shot}, Failed, "is blank"},
		{"shows", "shows", []string{shot}, Passed, ""},
		{"missing", "shows", []string{filepath.Join("${results}", "none.png")}, Failed, "none.png"},
	} {
		plan := Plan{Case: "proof", Stages: []api.MachineStage{{ID: "m9", Cmd: call("png", filepath.Join(results, "shot.png"), c.kind),
			Contains: []string{"saved"}, Images: c.images}}}
		res, err := Run(context.Background(), plan, w, results, nil)
		if err != nil {
			t.Fatal(err)
		}
		if s := res.Stages[0]; s.Status != c.want || !strings.Contains(s.Failure, c.failure) {
			t.Errorf("%s: %s (%s), want %s with %q\n%s", c.name, s.Status, s.Failure, c.want, c.failure, s.Output)
		}
	}
}

// A window's title bar over an empty body is blank too: content in a strip
// at the top is not a screen that shows something.
func TestATitleBarOverAnEmptyBodyIsBlank(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 1200; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if y < 38 {
				c = color.RGBA{uint8(x * 3), uint8(y * 5), uint8(x ^ y), 255}
			}
			img.Set(x, y, c)
		}
	}
	if s := ContentShare(img); s >= contentShare {
		t.Errorf("a title bar over a white body holds content in %.2f of its tiles, under %.2f expected", s, contentShare)
	}
}

// Whoever watches the Sandbox sees the run as it goes: the bootstrap opens a
// window following the runner's log, and each stage's own output reaches that
// log while the stage runs, not only its results.
func TestTheRunIsShownAsItGoes(t *testing.T) {
	b := Bootstrap()
	if !strings.Contains(b, `start "Clean-machine proof" powershell`) || !strings.Contains(b, "Get-Content -Wait '"+SandboxResults+`\`+RunnerLog+"'") {
		t.Errorf("the bootstrap opens no window following the runner's log:\n%s", b)
	}
	home := t.TempDir()
	w := World{Env: append(Clean(os.Environ()), helperEnvList()...), Home: home}
	plan := Plan{Case: "proof", Kind: api.MachineLocal, Channel: fakeChannel(t), Stages: []api.MachineStage{
		{ID: "say", Cmd: call("echo", "installing python (download)...")},
	}}
	var live bytes.Buffer
	if _, err := Run(context.Background(), plan, w, t.TempDir(), &live); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(live.String(), "installing python (download)...") {
		t.Errorf("the stage's output did not reach the run's log: %q", live.String())
	}
}

// : a local world names its own registry of installations, in
// its own config folder, and carries no config folder of the runner's: an
// installer of the case never records itself in the person's registry.
func TestALocalWorldKeepsItsOwnRegistryOfInstallations(t *testing.T) {
	dir := t.TempDir()
	w, err := LocalWorld(dir, []string{"KTW_INSTANCES=/person/katyoti-instances.yaml", "XDG_CONFIG_HOME=/home/person/.config", "PATH=x"})
	if err != nil {
		t.Fatal(err)
	}
	var instances []string
	for _, kv := range w.Env {
		k, v, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "KTW_INSTANCES":
			instances = append(instances, v)
		case "XDG_CONFIG_HOME":
			t.Errorf("the runner's config folder is carried: %s", kv)
		}
	}
	if len(instances) != 1 || !strings.HasPrefix(strings.ToLower(instances[0]), strings.ToLower(dir)) {
		t.Fatalf("KTW_INSTANCES = %v, want one file under the world %s", instances, dir)
	}
}
