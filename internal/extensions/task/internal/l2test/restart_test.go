// SPDX-License-Identifier: Apache-2.0

//go:build tasks_l2

package l2test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// The scenarios — each one installs its own universe and asserts on real
// processes (modules/tasks/runs.md).

// S1 — A waiting run survives a kill -9 of the serving process: after a
// restart the SAME run waits again, and the event sent then completes it.
// Two waits: a ledger (an in-process wait no client outside the process can
// meet: proven waiting again under its id) and an agent waiting on the person
// through the inbox (an event of another process: answered after the restart,
// it completes, without the model being called again for the question).
func TestS1_WaitingRunSurvivesKill9(t *testing.T) {
	u := newUniverse(t)

	ledger := u.runTask("l2-ledger")
	ask := u.runTask("l2-ask")
	u.waitStatus(ledger, 60*time.Second, "waiting")
	u.waitStatus(ask, 90*time.Second, "waiting")
	before := len(u.llm.Requests())
	t.Logf("before the kill: ledger %s and agent %s wait; the model was asked %d time(s): %v", ledger, ask, before, u.llm.Requests())

	first := u.serving
	first.kill()
	t.Logf("killed -9 the serving process (pid %d)", first.pid())
	killedAt := time.Now()

	u.serving = u.startServing()
	for _, id := range []string{ledger, ask} {
		r := u.waitStatus(id, 60*time.Second, "waiting")
		ts := u.lifecycle(id)
		t.Logf("after the restart, run %s (%s) is %s under the same id; lifecycle %v", id, r.TaskName, r.Status, transitions(ts))
		if !hasTransition(ts, "resumed", killedAt) {
			t.Errorf("run %s: no `resumed` transition after the restart (lifecycle %v)", id, transitions(ts))
		}
	}
	if n := len(u.llm.Requests()); n != before {
		t.Errorf("the restored agent called the model again while it waits (%d → %d): %v", before, n, u.llm.Requests())
	}

	// The event: the person answers the question in the inbox.
	reqID := u.pendingRequest()
	u.mustCall("ktw-agents:inbox/Answer", map[string]any{"id": reqID, "note": "yes, go on (L2)"}, nil)
	t.Logf("answered inbox request %s", reqID)
	r := u.waitStatus(ask, 60*time.Second, "completed", "failed", "cancelled", "killed")
	if r.Status != "completed" {
		t.Fatalf("the agent ended %s (%q), want completed", r.Status, r.Error)
	}
	res := u.result(ask)
	if !strings.Contains(res, "yes, go on (L2)") {
		t.Errorf("the agent's result does not carry the answer: %s", res)
	}
	t.Logf("the agent %s completed after the restart: %s; model requests %v", ask, res, u.llm.Requests())
}

// pendingRequest is the id of the one pending approval request in the inbox.
func (u *universe) pendingRequest() string {
	u.t.Helper()
	var id string
	u.waitFor(30*time.Second, "a pending request in the inbox", func() (bool, string) {
		raw, err := u.call("ktw-agents:inbox/Triage", map[string]any{})
		if err != nil {
			return false, err.Error()
		}
		var out struct {
			Entries []struct {
				ID     string `json:"id"`
				Type   string `json:"type"`
				Status string `json:"status"`
				Title  string `json:"title"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return false, err.Error()
		}
		for _, e := range out.Entries {
			if e.Type == "approval-request" && e.Status == "pending" && strings.Contains(e.Title, "L2 harness") {
				id = e.ID
				return true, ""
			}
		}
		return false, string(raw)
	})
	return id
}

// S2 — A working run that restarts after a crash (a simulator in its step; a
// workflow in its second step) killed -9 comes back restarted from its last
// step: same run, `restarted`, then completed — not interrupted. The
// workflow's first step is not run again; its second step, a child of its
// own, comes back under its id.
func TestS2_WorkingRunRestartsFromLastStep(t *testing.T) {
	u := newUniverse(t)

	sim := u.runTask("l2-sim")
	wf := u.runTask("l2-wf")
	u.waitStatus(sim, 30*time.Second, "running")
	var children []runInfo
	u.waitFor(30*time.Second, "the workflow's second step works", func() (bool, string) {
		rs, err := u.runs()
		if err != nil {
			return false, err.Error()
		}
		children = children[:0]
		running := false
		for _, r := range rs {
			if r.ParentRunID == wf {
				children = append(children, r)
				if r.Status == "running" && len(children) >= 1 {
					running = true
				}
			}
		}
		return len(children) == 2 && running, fmt.Sprintf("children %+v", children)
	})
	time.Sleep(2 * time.Second) // well inside the steps (12 s each)
	before := childIDs(children)
	t.Logf("before the kill: simulator %s runs; workflow %s has children %v", sim, wf, children)

	u.serving.kill()
	killedAt := time.Now()
	t.Logf("killed -9 the serving process (pid %d)", u.serving.pid())
	u.serving = u.startServing()

	for _, id := range []string{sim, wf} {
		r := u.waitStatus(id, 60*time.Second, "completed", "failed", "cancelled", "killed")
		ts := u.lifecycle(id)
		t.Logf("run %s (%s) ended %s %q; lifecycle %v", id, r.TaskName, r.Status, r.Error, transitions(ts))
		if r.Status != "completed" || r.Error != "" {
			t.Errorf("run %s ended %s (%q), want completed with no error", id, r.Status, r.Error)
		}
		if hasTransition(ts, "interrupted", killedAt) {
			t.Errorf("run %s was reported interrupted", id)
		}
	}
	if ts := u.lifecycle(sim); !hasTransition(ts, "restarted", killedAt) {
		t.Errorf("the simulator has no `restarted` transition after the kill: %v", transitions(ts))
	}
	if res := u.result(sim); !strings.Contains(res, "l2-sim-done") {
		t.Errorf("simulator result: %s", res)
	}
	if res := u.result(wf); !strings.Contains(res, `"b"`) {
		t.Errorf("workflow result does not carry its second step's output: %s", res)
	}
	rs, err := u.runs()
	if err != nil {
		t.Fatal(err)
	}
	var after []runInfo
	for _, r := range rs {
		if r.ParentRunID == wf {
			after = append(after, r)
		}
	}
	t.Logf("after: the workflow's children %v; its lifecycle %v", after, transitions(u.lifecycle(wf)))
	if got := childIDs(after); !slices.Equal(got, before) {
		t.Errorf("the workflow's children changed across the restart: before %v, after %v (a step run again, or a child replaced)", before, got)
	}
	inFlight := map[string]bool{}
	for _, c := range children {
		inFlight[c.RunID] = c.Status == "running"
	}
	for _, c := range after {
		ts := u.lifecycle(c.RunID)
		t.Logf("child %s (%s): %s; lifecycle %v", c.RunID, c.TaskName, c.Status, transitions(ts))
		if c.Status != "completed" {
			t.Errorf("child %s ended %s %q", c.RunID, c.Status, c.Error)
		}
		if inFlight[c.RunID] && !hasTransition(ts, "restarted", killedAt) {
			t.Errorf("the child working at the kill (%s) was not restarted", c.RunID)
		}
		if !inFlight[c.RunID] && hasTransition(ts, "restarted", killedAt) {
			t.Errorf("the child that had ended before the kill (%s) was run again", c.RunID)
		}
	}
}

func childIDs(rs []runInfo) []string {
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.RunID)
	}
	slices.Sort(ids)
	return ids
}

// S3 — One serving process per universe: a second `ktw-agents sc ktsp nats`
// started while the first serves is refused — it exits non-zero, naming the
// process that serves — and the first goes on serving.
func TestS3_SecondServingProcessRefused(t *testing.T) {
	u := newUniverse(t)
	first := u.serving

	second := u.start("serving-second", "ktw-agents", "sc", "ktsp", "nats")
	code, err := second.wait(60 * time.Second)
	if err != nil {
		t.Fatalf("the second serving process was not refused: %v\n%s", err, tail(second.log, 20))
	}
	out := second.output()
	t.Logf("the second serving process exited %d:\n%s", code, tail(second.log, 5))
	if code == 0 {
		t.Errorf("the second serving process exited 0, want non-zero")
	}
	if !strings.Contains(out, "already serves this universe") || !strings.Contains(out, fmt.Sprintf("pid %d", first.pid())) {
		why := ""
		if strings.Contains(out, "workspace lock held by another process") {
			why = "PRODUCT: refused at boot by the CWM workspace lock (the attached project, held by the serving process), " +
				"before the serving lock is tried — the message names no process. "
		}
		t.Errorf("%sthe refusal does not name the serving process (pid %d): %s", why, first.pid(), tail(second.log, 5))
	}
	if first.exited() {
		t.Fatalf("the first serving process stopped")
	}
	if _, err := u.call("ktw-agents:tasks/Groups", map[string]any{}); err != nil {
		t.Errorf("the first serving process no longer answers: %v", err)
	}
}

// S4 — A run a CLI started (`ktw-agents run`), quit with a Ctrl+C (a console
// break on Windows), ends cancelled; the serving process never takes it up
// (watched 40 s, past its 30 s restore sweep).
//
// S4a runs the CLI beside the serving process, as the documentation has it ("any
// other process (a CLI or TUI run …) stops them … the serving process never
// restarts them"). S4b runs it while no process serves, then starts the serving
// process: its first restore sweep, then every later one, must leave the quit
// run ended.
func TestS4a_CLIRunBesideServingProcessQuitIsNotRestarted(t *testing.T) {
	u := newUniverse(t)
	m, quitAt, ok := cliRunAndQuit(t, u)
	if !ok {
		return
	}
	watchNotTakenUp(t, u, m, quitAt)
}

func TestS4b_CLIRunQuitIsNotTakenUpByTheNextServingProcess(t *testing.T) {
	u := newUniverseWith(t, false)
	m, quitAt, ok := cliRunAndQuit(t, u)
	if !ok {
		return
	}
	u.serving = u.startServing()
	t.Logf("the serving process started after the quit (pid %d)", u.serving.pid())
	watchNotTakenUp(t, u, m, quitAt)
}

// cliRunAndQuit starts `ktw-agents run l2-cli-sim`, waits for its run to be
// kept, quits it with a Ctrl+C and checks its run ended cancelled. ok false:
// the scenario could not go on (skipped or failed).
func cliRunAndQuit(t *testing.T, u *universe) (manifest, time.Time, bool) {
	t.Helper()
	cli := u.start("cli-run", "ktw-agents", "run", "l2-cli-sim", "--raw")
	var m manifest
	u.waitFor(90*time.Second, "the CLI's run is kept in the runs folder", func() (bool, string) {
		if cli.exited() {
			out := tail(cli.log, 30)
			if strings.Contains(out, "workspace lock held by another process") {
				t.Fatalf("PRODUCT: `ktw-agents run` cannot start while the serving process serves the universe — "+
					"it exits %d at boot, the CWM workspace (the attached project) being locked by the serving process:\n%s",
					cli.cmd.ProcessState.ExitCode(), out)
			}
			t.Fatalf("the CLI exited early: %s", out)
		}
		for _, x := range u.manifests() {
			if x.Request.TaskName == "l2-cli-sim" {
				m = x
				return true, ""
			}
		}
		return false, "no manifest yet"
	})
	time.Sleep(2 * time.Second)
	t.Logf("the CLI (pid %d) runs %s (owner %s)", cli.pid(), m.RunID, m.Owner)

	if err := interrupt(cli.pid()); err != nil {
		t.Skipf("cannot send %s to the CLI: %v — scenario not run", signalName, err)
		return m, time.Time{}, false
	}
	t.Logf("sent %s to the CLI", signalName)
	code, err := cli.wait(30 * time.Second)
	if err != nil {
		t.Fatalf("the CLI did not quit on %s: %v", signalName, err)
	}
	t.Logf("the CLI exited %d; output tail:\n%s", code, tail(cli.log, 8))
	if s := u.ended(m.RunID); s != "cancelled" {
		t.Errorf("after the quit, the run's record says %q, want cancelled", s)
	}
	if ts := u.lifecycle(m.RunID); !hasTransition(ts, "cancelled", time.Time{}) {
		t.Errorf("the quit run's journal has no `cancelled` transition: %v", transitions(ts))
	}
	return m, time.Now(), true
}

// ended is how a run's record says it ended: runs/ended/<id>.json, else its
// manifest.
func (u *universe) ended(id string) string {
	if rec, ok := u.endedRecord(id); ok {
		s, _ := rec["status"].(string)
		return s
	}
	for _, x := range u.manifests() {
		if x.RunID == id {
			if x.Ended != "" {
				return x.Ended
			}
			return "manifest kept, not ended (stopped=" + x.Stopped + ")"
		}
	}
	return "no record"
}

// watchNotTakenUp watches, 40 s from the quit, that the serving process never
// takes the quit run up.
func watchNotTakenUp(t *testing.T, u *universe, m manifest, quitAt time.Time) {
	t.Helper()
	for time.Since(quitAt) < 40*time.Second {
		r, ok, err := u.run(m.RunID)
		if err == nil && ok && slices.Contains([]string{"queued", "running", "waiting", "pending"}, r.Status) {
			t.Fatalf("the serving process took the quit run up: %s is %s (lifecycle %v)", m.RunID, r.Status, transitions(u.lifecycle(m.RunID)))
		}
		time.Sleep(2 * time.Second)
	}
	ts := u.lifecycle(m.RunID)
	for _, bad := range []string{"restarted", "resumed", "interrupted"} {
		if hasTransition(ts, bad, quitAt) {
			t.Errorf("the quit run was %s by the serving process (lifecycle %v)", bad, transitions(ts))
		}
	}
	r, listed, _ := u.run(m.RunID)
	t.Logf("40 s after the quit: record %q; the serving process lists it: %v %s %q; lifecycle %v", u.ended(m.RunID), listed, r.Status, r.Error, transitions(ts))
	if s := u.ended(m.RunID); s != "cancelled" {
		t.Errorf("40 s after the quit, the run's record says %q, want cancelled", s)
	}
}

// S5 — A clean stop of the serving process (Ctrl+C / a console break) pauses
// its working run — kept, told `paused` — and the next start takes it up: the
// same run restarts and completes.
func TestS5_CleanStopPausesAndNextStartTakesUp(t *testing.T) {
	u := newUniverse(t)

	sim := u.runTask("l2-sim")
	u.waitStatus(sim, 30*time.Second, "running")
	time.Sleep(2 * time.Second)

	first := u.serving
	if err := interrupt(first.pid()); err != nil {
		t.Skipf("cannot send %s to the serving process: %v — scenario not run", signalName, err)
	}
	stoppedAt := time.Now()
	code, err := first.wait(30 * time.Second)
	if err != nil {
		t.Fatalf("the serving process did not stop on %s: %v", signalName, err)
	}
	t.Logf("the serving process exited %d on %s; output tail:\n%s", code, signalName, tail(first.log, 6))
	ts := u.lifecycle(sim)
	if !hasTransition(ts, "paused", stoppedAt.Add(-time.Second)) {
		t.Errorf("the working run was not paused by the clean stop (lifecycle %v)", transitions(ts))
	}
	kept := false
	for _, m := range u.manifests() {
		if m.RunID == sim {
			kept = m.Ended == "" && m.Stopped == ""
			t.Logf("its manifest is kept: ended %q stopped %q", m.Ended, m.Stopped)
		}
	}
	if !kept {
		t.Errorf("the paused run's manifest was not kept as a run to take up")
	}

	u.serving = u.startServing()
	r := u.waitStatus(sim, 60*time.Second, "completed", "failed", "cancelled", "killed")
	ts = u.lifecycle(sim)
	t.Logf("after the next start, run %s ended %s %q; lifecycle %v", sim, r.Status, r.Error, transitions(ts))
	if r.Status != "completed" || r.Error != "" {
		t.Errorf("the paused run ended %s (%q), want completed", r.Status, r.Error)
	}
	if !hasTransition(ts, "restarted", stoppedAt) && !hasTransition(ts, "resumed", stoppedAt) {
		t.Errorf("the next start did not take the paused run up (lifecycle %v)", transitions(ts))
	}
}

// S6 — A bus reconnect: the hub is killed and started again (on another port)
// while the serving process lives. Its shared hub connection follows the hub
// and the waits that recheck themselves (an agent's await) are woken once —
// the reconnect recheck (RecheckWaits). Then the serving process must answer
// through the new hub again, and the answer to the agent's question
// completes it.
func TestS6_BusReconnectRechecksAgentWaits(t *testing.T) {
	u := newUniverse(t)

	ask := u.runTask("l2-ask")
	u.waitStatus(ask, 90*time.Second, "waiting")
	t.Logf("agent %s waits; lifecycle %v", ask, transitions(u.lifecycle(ask)))

	oldHub := u.hub
	oldHub.kill()
	t.Logf("killed -9 the hub (pid %d); the serving process (pid %d) lives on", oldHub.pid(), u.serving.pid())
	time.Sleep(2 * time.Second)
	restartedAt := time.Now()
	u.startHub()
	if u.serving.exited() {
		t.Fatalf("the serving process died with the hub: %s", tail(u.serving.log, 20))
	}

	// The reconnect recheck: the waiting agent is woken once, rechecks its
	// question (still pending), and waits again.
	// RecheckWaits publishes on the wait's own topic: the run is woken by its
	// wait's label (ask-user), not by the executor's tick.
	u.waitFor(60*time.Second, "the agent's wait is rechecked after the reconnect (woken by ask-user)", func() (bool, string) {
		ts := u.lifecycle(ask)
		for _, tr := range ts {
			if tr.Transition == "woken" && tr.WokenBy == "ask-user" && !tr.At.Before(restartedAt) {
				return true, ""
			}
		}
		return false, fmt.Sprintf("lifecycle %v", transitions(ts))
	})
	t.Logf("RecheckWaits ran: lifecycle %v", transitions(u.lifecycle(ask)))

	// The serving process answers through the new hub.
	deadline := time.Now().Add(45 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, lastErr = u.call("ktw-agents:tasks/Groups", map[string]any{}); lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		t.Fatalf("PRODUCT: the serving process does not answer through the restarted hub after 45 s: %v — "+
			"its slot subscriptions dialled the old hub's URL (see the report)", lastErr)
	}
	r := u.waitStatus(ask, 30*time.Second, "waiting")
	t.Logf("the serving process answers through the new hub; agent %s is %s", ask, r.Status)

	reqID := u.pendingRequest()
	u.mustCall("ktw-agents:inbox/Answer", map[string]any{"id": reqID, "note": "after the reconnect (L2)"}, nil)
	r = u.waitStatus(ask, 60*time.Second, "completed", "failed", "cancelled", "killed")
	if r.Status != "completed" {
		t.Fatalf("the agent ended %s (%q), want completed", r.Status, r.Error)
	}
	t.Logf("the agent completed: %s", u.result(ask))
}
