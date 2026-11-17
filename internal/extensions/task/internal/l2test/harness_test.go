// SPDX-License-Identifier: Apache-2.0

//go:build tasks_l2

// Package l2test proves the task runtime's restart guarantees
// (modules/tasks/runs.md — a restart, one serving process, a clean stop pauses
// and a quit stops, the bus reconnect) on the REAL binaries: a
// ktw-hub, a `ktw-agents sc ktsp nats` serving process and a `ktw-agents run`
// CLI, in a sandbox universe, killed (-9), stopped (a console signal) and
// started again — the L2 layer (app/ktw-code/embed/references/modules/
// test-layers.md).
//
// The universe is a scenario case (testdata/tasks-l2) installed by the real
// `ktw-compose scenario install` — its own global root, data folder and
// attached project — but its processes are started by the harness itself, not
// by `scenario start`: a scenario's hub spawns ktw-agents as its child (killed
// with it, never by itself), while these guarantees need the serving process
// killed alone, a second one started beside it, a CLI interrupted, and the hub
// restarted under a serving process that lives on. The harness reaches the
// universe as any client does: through the hub (a plain KTSP sender, which
// never asks the hub to spawn a member).
//
// No LLM: the agent's model is the harness's fake OpenAI endpoint
// (fakellm_test.go). It builds and runs binaries, so it is no part of
// `go test ./...`:
//
//	make tasks-l2
//	go test -tags tasks_l2 -count=1 -v -timeout 15m ./internal/extensions/task/internal/l2test/
//
// Each test installs its own case under a temporary folder ($KTW_TEST_ROOT
// when set) and stops every process of it when it ends.
package l2test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

const caseName = "tasks-l2"

var binDir string

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// repoRoot is the repository's root, from this package's folder.
func repoRoot() string {
	wd, _ := os.Getwd()
	return filepath.Clean(filepath.Join(wd, "..", "..", "..", "..", ".."))
}

func TestMain(m *testing.M) {
	prebuilt := os.Getenv("KTW_L2_BIN") // prebuilt binaries (ktw-hub, ktw-agents, ktw-compose)
	if prebuilt != "" {
		binDir = prebuilt
	} else {
		dir, err := os.MkdirTemp("", "tasks-l2-bin")
		if err != nil {
			fmt.Fprintln(os.Stderr, "mktemp:", err)
			os.Exit(1)
		}
		binDir = dir
		for _, app := range []string{"ktw-hub", "ktw-agents", "ktw-compose"} {
			cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", filepath.Join(binDir, exe(app)), "./app/"+app+"/")
			cmd.Dir, cmd.Stderr = repoRoot(), os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "build", app, ":", err)
				os.Exit(1)
			}
		}
	}
	// No LLM key and no universe of the machine reach the case's processes.
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasSuffix(k, "_API_KEY") {
			_ = os.Unsetenv(k)
		}
	}
	for _, k := range []string{"KTW_UNIVERSE", "KTW_UNIVERSE_ID", "KTW_NATS_URL", "KTW_HOME", "KTW_WORKSPACE",
		"KTW_WORKSPACE_DIR", "KTW_TENANT_ID", "KTW_SESSION", "KTW_ROOM", "KTW_CONTENT_ROOT", "KTW_DATA"} {
		_ = os.Unsetenv(k)
	}
	ensureConsole()
	code := m.Run()
	if prebuilt == "" {
		_ = os.RemoveAll(binDir)
	}
	os.Exit(code)
}

// ── the universe ─────────────────────────────────────────────────────────

// universe is one installed case: its folders, its processes, its model.
type universe struct {
	t                                   *testing.T
	base, cases, root, dir              string
	universeDir, data, project, caseBin string
	logs                                string
	tenant, name                        string
	llm                                 *fakeLLM
	hub, serving                        *proc
	procs                               []*proc
	seq                                 int
}

// newUniverse installs the case in a temporary folder, its own global root
// and cache, its model the fake endpoint; then starts its hub and its
// serving process.
func newUniverse(t *testing.T) *universe {
	t.Helper()
	return newUniverseWith(t, true)
}

// newUniverseWith installs the case and starts its hub — and its serving
// process when serving.
func newUniverseWith(t *testing.T, serving bool) *universe {
	t.Helper()
	base := t.TempDir()
	if r := os.Getenv("KTW_TEST_ROOT"); r != "" {
		base = filepath.Join(r, t.Name())
		_ = os.RemoveAll(base)
	}
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(base, "global"))
	t.Setenv("KTW_CACHE", filepath.Join(base, "cache"))
	u := &universe{t: t, base: base, cases: filepath.Join(base, "cases"), root: filepath.Join(base, "root"),
		logs: filepath.Join(base, "logs"), llm: startFakeLLM(t)}
	u.dir = filepath.Join(u.root, caseName)
	u.universeDir = filepath.Join(u.dir, "universe")
	u.data = filepath.Join(u.dir, "data")
	u.project = filepath.Join(u.dir, "project")
	u.caseBin = filepath.Join(u.dir, "bin")
	if err := os.MkdirAll(u.logs, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(repoRoot(), "internal", "extensions", "task", "internal", "l2test", "testdata", caseName)
	if err := copyDir(src, filepath.Join(u.cases, caseName), map[string]string{"__FAKE_LLM_URL__": u.llm.url}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(u.stopAll)
	out, err := u.compose("install", caseName, "--bin", binDir, "--force")
	if err != nil {
		t.Fatalf("scenario install: %v\n%s", err, out)
	}
	u.tenant, _ = cenv.ResolveTenantUniverse(u.universeDir)
	u.name = "scn-" + caseName // the case's universe (scenario api.Case.UniverseName)
	t.Logf("sandbox universe %s (tenant %s) at %s", u.name, u.tenant, u.dir)
	u.startHub()
	if serving {
		u.serving = u.startServing()
	}
	return u
}

// compose runs `ktw-compose scenario <args>` on the case.
func (u *universe) compose(args ...string) (string, error) {
	args = append(append([]string{"scenario"}, args...), "--cases", u.cases, "--root", u.root)
	cmd := exec.Command(filepath.Join(binDir, exe("ktw-compose")), args...)
	cmd.Dir = repoRoot()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// stopAll ends every process of the case: the harness's own first, then
// whatever runs from the case's bin folder (`scenario stop`).
func (u *universe) stopAll() {
	if u.t.Failed() {
		u.dumpLogs()
	}
	for _, p := range u.procs {
		p.kill()
	}
	_, _ = u.compose("stop", caseName)
}

// dumpLogs shows the end of every process log, and of the universe's own.
func (u *universe) dumpLogs() {
	for _, p := range u.procs {
		u.t.Logf("── %s (pid %d) log tail:\n%s", p.tag, p.pid(), tail(p.log, 40))
	}
	u.t.Logf("── data/logs/ktw-agents.log tail:\n%s", tail(filepath.Join(u.data, "logs", "ktw-agents.log"), 60))
	u.t.Logf("── fake model requests: %v", u.llm.Requests())
}

// ── processes ────────────────────────────────────────────────────────────

// proc is one process the harness started from the case's bin folder.
type proc struct {
	tag  string
	cmd  *exec.Cmd
	log  string
	done chan struct{}
}

func (p *proc) pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// exited reports whether the process has ended.
func (p *proc) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// wait waits up to d for the process to end; its exit code, or an error.
func (p *proc) wait(d time.Duration) (int, error) {
	select {
	case <-p.done:
		return p.cmd.ProcessState.ExitCode(), nil
	case <-time.After(d):
		return 0, fmt.Errorf("%s (pid %d) still runs after %s", p.tag, p.pid(), d)
	}
}

// kill ends the process outright (kill -9: TerminateProcess on Windows).
func (p *proc) kill() {
	if p.cmd.Process != nil && !p.exited() {
		_ = p.cmd.Process.Kill()
		_, _ = p.wait(15 * time.Second)
	}
}

// output is what the process wrote so far.
func (p *proc) output() string {
	b, _ := os.ReadFile(p.log)
	return string(b)
}

// start runs one of the case's binaries in its project, with the universe's
// environment (what `scenario start` gives its hub), in a process group of
// its own, its output in the harness's logs.
func (u *universe) start(tag, binary string, args ...string) *proc {
	u.t.Helper()
	u.seq++
	tag = fmt.Sprintf("%02d-%s", u.seq, tag)
	logPath := filepath.Join(u.logs, tag+".log")
	f, err := os.Create(logPath)
	if err != nil {
		u.t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(u.caseBin, exe(binary)), args...)
	cmd.Dir = u.project
	cmd.Env = append(hubclient.SpawnEnvForWorkspace(u.universeDir), "KTW_DATA="+u.data)
	cmd.Stdout, cmd.Stderr = f, f
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		u.t.Fatalf("start %s: %v", tag, err)
	}
	p := &proc{tag: tag, cmd: cmd, log: logPath, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); _ = f.Close(); close(p.done) }()
	u.procs = append(u.procs, p)
	u.t.Logf("started %s: pid %d (%s %s)", tag, p.pid(), binary, strings.Join(args, " "))
	return p
}

// startHub starts the universe's hub and waits for it to serve.
func (u *universe) startHub() *proc {
	u.t.Helper()
	p := u.start("hub", "ktw-hub", "serve")
	u.waitFor(60*time.Second, "the hub serves the universe", func() (bool, string) {
		if p.exited() {
			u.t.Fatalf("the hub exited: %s", tail(p.log, 30))
		}
		lock, err := hubclient.ReadLockFile(u.data)
		if err != nil || lock == nil || lock.PID != p.pid() {
			return false, fmt.Sprintf("lock file %+v (%v)", lock, err)
		}
		hc, err := u.connect(5 * time.Second)
		if err != nil {
			return false, err.Error()
		}
		_ = hc.Close()
		return true, ""
	})
	u.hub = p
	return p
}

// startServing starts `ktw-agents sc ktsp nats` and waits for it to answer
// the task service.
func (u *universe) startServing() *proc {
	u.t.Helper()
	p := u.start("serving", "ktw-agents", "sc", "ktsp", "nats")
	u.waitFor(90*time.Second, "the serving process answers tasks/Groups", func() (bool, string) {
		if p.exited() {
			u.t.Fatalf("the serving process exited (code %d): %s", p.cmd.ProcessState.ExitCode(), tail(p.log, 30))
		}
		if !strings.Contains(p.output(), "subscribed to") {
			return false, "not subscribed yet"
		}
		if _, err := u.call("ktw-agents:tasks/Groups", map[string]any{}); err != nil {
			return false, err.Error()
		}
		return true, ""
	})
	return p
}

// ── the bus ──────────────────────────────────────────────────────────────

func (u *universe) connect(d time.Duration) (*hubclient.HubClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: u.data, Mode: hubclient.ModeReuse})
}

// call calls `<provider>:<slot>/<Method>` through the hub — a plain sender:
// no responder is an error, never a member spawned.
func (u *universe) call(uri string, in any) (json.RawMessage, error) {
	hc, err := u.connect(10 * time.Second)
	if err != nil {
		return nil, err
	}
	defer hc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return messagebus.NewSender(hc.MessageBus(), u.tenant, u.name).Call(ctx, uri, in)
}

func (u *universe) mustCall(uri string, in, out any) {
	u.t.Helper()
	raw, err := u.call(uri, in)
	if err != nil {
		u.t.Fatalf("%s: %v", uri, err)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			u.t.Fatalf("%s: decode %s: %v", uri, raw, err)
		}
	}
}

// runTask starts a task as a person does from the GUI; its run id.
func (u *universe) runTask(name string) string {
	u.t.Helper()
	var out struct {
		RunID  string `json:"runId"`
		Status string `json:"status"`
	}
	u.mustCall("ktw-agents:tasks/Run", map[string]any{"taskName": name, "origin": map[string]any{"kind": "person", "id": "l2"}}, &out)
	if out.RunID == "" {
		u.t.Fatalf("tasks/Run %s answered no run id", name)
	}
	u.t.Logf("run %s started: %s (%s)", name, out.RunID, out.Status)
	return out.RunID
}

type runInfo struct {
	RunID       string `json:"runId"`
	TaskName    string `json:"taskName"`
	Status      string `json:"status"`
	State       string `json:"state"`
	ParentRunID string `json:"parentRunId"`
	SessionID   string `json:"sessionId"`
	Error       string `json:"error"`
}

// runs lists the runs the serving process knows.
func (u *universe) runs() ([]runInfo, error) {
	raw, err := u.call("ktw-agents:tasks/Runs", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Runs []runInfo `json:"runs"`
	}
	err = json.Unmarshal(raw, &out)
	return out.Runs, err
}

// run is one run as the serving process lists it; ok false when unlisted.
func (u *universe) run(id string) (runInfo, bool, error) {
	rs, err := u.runs()
	if err != nil {
		return runInfo{}, false, err
	}
	for _, r := range rs {
		if r.RunID == id {
			return r, true, nil
		}
	}
	return runInfo{}, false, nil
}

// waitStatus waits for a run to be listed in one of the statuses.
func (u *universe) waitStatus(id string, d time.Duration, statuses ...string) runInfo {
	u.t.Helper()
	var got runInfo
	u.waitFor(d, fmt.Sprintf("run %s is %v", id, statuses), func() (bool, string) {
		r, ok, err := u.run(id)
		if err != nil {
			return false, err.Error()
		}
		if !ok {
			return false, "not listed"
		}
		got = r
		for _, s := range statuses {
			if r.Status == s {
				return true, ""
			}
		}
		return false, fmt.Sprintf("status %s (error %q)", r.Status, r.Error)
	})
	return got
}

// result is a run's result (tasks/Result).
func (u *universe) result(id string) string {
	u.t.Helper()
	raw, err := u.call("ktw-agents:tasks/Result", map[string]any{"runId": id})
	if err != nil {
		u.t.Fatalf("tasks/Result %s: %v", id, err)
	}
	return string(raw)
}

// ── the journals ─────────────────────────────────────────────────────────

// transition is one RunLifecycleEvent a run's session journal kept.
type transition struct {
	Transition string    `json:"transition"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason"`
	WokenBy    string    `json:"wokenBy"`
	At         time.Time `json:"at"`
}

// lifecycle reads, from every session journal of the universe, the lifecycle
// of one run, in order.
func (u *universe) lifecycle(runID string) []transition {
	var out []transition
	files, _ := filepath.Glob(filepath.Join(u.data, "sessions", "*", "llm*.jsonl"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil || !bytes.Contains(b, []byte(runID)) {
			continue
		}
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			var line struct {
				Type string `json:"type"`
				Data struct {
					RunID string `json:"runId"`
					transition
				} `json:"data"`
			}
			if json.Unmarshal(sc.Bytes(), &line) != nil || line.Type != "run_lifecycle" || line.Data.RunID != runID {
				continue
			}
			out = append(out, line.Data.transition)
		}
	}
	return out
}

func transitions(ts []transition) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Transition
		if t.Reason != "" {
			out[i] += "(" + t.Reason + ")"
		}
		if t.WokenBy != "" {
			out[i] += "(by " + t.WokenBy + ")"
		}
	}
	return out
}

func hasTransition(ts []transition, name string, after time.Time) bool {
	for _, t := range ts {
		if t.Transition == name && !t.At.Before(after) {
			return true
		}
	}
	return false
}

// ── the runs folder ──────────────────────────────────────────────────────

// manifest is the part of a run's manifest (runs/<id>.run.json) read here.
type manifest struct {
	RunID   string `json:"runId"`
	Request struct {
		TaskName string `json:"taskName"`
	} `json:"request"`
	Owner   string `json:"owner"`
	PID     int    `json:"pid"`
	Ended   string `json:"ended"`
	Stopped string `json:"stopped"`
}

// manifests reads the runs folder's manifests.
func (u *universe) manifests() []manifest {
	var out []manifest
	files, _ := filepath.Glob(filepath.Join(u.data, "runs", "*.run.json"))
	for _, f := range files {
		var m manifest
		if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// endedRecord is how a run ended, kept in runs/ended/<id>.json.
func (u *universe) endedRecord(id string) (map[string]any, bool) {
	b, err := os.ReadFile(filepath.Join(u.data, "runs", "ended", id+".json"))
	if err != nil {
		return nil, false
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m) == nil
}

// ── helpers ──────────────────────────────────────────────────────────────

// waitFor polls cond every half second until it holds; fatal after d, with
// what cond last said.
func (u *universe) waitFor(d time.Duration, what string, cond func() (bool, string)) {
	u.t.Helper()
	deadline := time.Now().Add(d)
	last := ""
	for {
		ok, why := cond()
		if ok {
			return
		}
		last = why
		if time.Now().After(deadline) {
			u.t.Fatalf("after %s, not yet: %s — last: %s", d, what, last)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func tail(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(" + err.Error() + ")"
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if !strings.Contains(l, "one URI published by several providers") { // startup noise
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// copyDir copies a folder, replacing in every file each key of subst by its
// value.
func copyDir(src, dst string, subst map[string]string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for k, v := range subst {
			b = bytes.ReplaceAll(b, []byte(k), []byte(v))
		}
		return os.WriteFile(target, b, 0o644)
	})
}
