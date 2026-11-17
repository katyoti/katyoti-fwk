// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// THE UNIVERSE'S PROCESSES — started headless, the way the GUI starts them: a
// ktw-hub serving the universe (its embedded NATS), which spawns each member
// on its first call. Every process of a case runs from the case's own bin
// folder, so stopping the case stops exactly its processes, by their path.

const hubBinary = "ktw-hub"

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Running reports whether the case's hub answers.
func (e *Engine) Running(ctx context.Context) bool { return e.reach(ctx) == nil }

// reach is nil once the case's hub answers, else why it does not (no
// lockfile, a stale one, a refused connection): what Start says when the hub
// it started never answers.
func (e *Engine) reach(ctx context.Context) error {
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: e.RT.Data(), Mode: hubclient.ModeReuse})
	if err != nil {
		return err
	}
	_ = hc.Close()
	return nil
}

// inheritedHubVars are what a process of another universe hands its
// children: a command's hub owner, a member's bus key, its bus, tenant and
// universe. A case command run from such a process (a walk a verification
// runs from the development universe's ktw-methods) passed them to the case's
// hub, which then ran as a command's hub - ending once no client was
// connected, its lockfile hidden - not the universe's own.
var inheritedHubVars = []string{hubclient.OwnerEnv, "KTW_BUS_KEY", "KTW_NATS_URL", "KTW_TENANT_ID", "KTW_UNIVERSE_ID", "KTW_UNIVERSE", "KTW_DATA"}

// caseProcessEnv is the environment a process of a case's universe starts
// with: this process's own, scoped to the universe and its data, without
// what another universe's process handed down. A display switch a walk
// exports (KTW_*DISPLAY*) stays.
func caseProcessEnv(universe, data string) []string {
	in := hubclient.SpawnEnvForWorkspace(universe)
	out := make([]string, 0, len(in)+1)
	for _, kv := range in {
		inherited := false
		for _, name := range inheritedHubVars {
			if strings.HasPrefix(strings.ToUpper(kv), name+"=") {
				inherited = true
				break
			}
		}
		if !inherited {
			out = append(out, kv)
		}
	}
	return append(out, "KTW_DATA="+data)
}

// Start starts the case's universe: its hub, detached from this process (it
// outlives the command), then a first call that spawns ktw-agents — the
// serving process, which runs the planner.
func (e *Engine) Start(ctx context.Context) error {
	if err := e.CheckFresh(); err != nil {
		return err
	}
	if e.Running(ctx) {
		if pid, path := e.servingHub(); !e.isOwnHub(path) {
			// Served by another binary (the GUI's control plane, say): not the
			// conditions the case runs in. Replaced by the case's own hub.
			e.logf("universe %s is served by a hub that is not the case's own (pid %d, %s) — replacing it", e.Case.UniverseName(), pid, path)
			if err := e.Stop(ctx); err != nil {
				return err
			}
			return e.Start(ctx)
		}
		// A hub that answers is not yet a universe that works: after a sleep or
		// a crash its members may be gone and its own slots lost. Then the
		// universe is started again, as a person would.
		if err := e.membersAnswer(ctx, 5); err != nil {
			e.logf("universe %s: its hub runs but ktw-agents does not answer (%v) — starting it again", e.Case.UniverseName(), err)
			if err := e.Stop(ctx); err != nil {
				return err
			}
			return e.Start(ctx)
		}
		e.logf("universe %s already running", e.Case.UniverseName())
		return nil
	}
	if err := os.MkdirAll(e.RT.Runs(), 0o755); err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(e.RT.Runs(), "hub.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(e.RT.Bin(), exe(hubBinary)), "serve")
	// The universe's processes start in the case's project, as when a person
	// runs the platform from their project: what resolves against the working
	// folder (an agent's file tools) stays in the case, never in whatever
	// folder the scenario command was run from.
	cmd.Dir = e.RT.Project()
	cmd.Env = caseProcessEnv(e.RT.Universe(), e.RT.Data())
	cmd.Stdout, cmd.Stderr = logf, logf
	publish.Detach(cmd) // it outlives this command; a hidden console, so nothing it starts opens a window
	if err := cmd.Start(); err != nil {
		_ = logf.Close()
		return fmt.Errorf("start the hub: %w", err)
	}
	_ = cmd.Process.Release()
	_ = logf.Close()

	deadline := time.Now().Add(60 * time.Second)
	for {
		last := e.reach(ctx)
		if last == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the hub did not answer within 60s: %v (see %s)", last, filepath.Join(e.RT.Runs(), "hub.log"))
		}
		time.Sleep(time.Second)
	}
	// The first call spawns ktw-agents (the retrying sender asks the hub's
	// connection manager to): the planner and the CWM host start with it.
	if err := e.membersAnswer(ctx, 30); err != nil {
		return fmt.Errorf("ktw-agents did not answer: %w", err)
	}
	e.logf("universe %s started (hub + ktw-agents)", e.Case.UniverseName())
	return nil
}

// membersAnswer is nil once ktw-agents answers through the case's hub, tried
// at most tries times two seconds apart; the first call spawns it.
func (e *Engine) membersAnswer(ctx context.Context, tries int) error {
	b, err := e.connect(ctx)
	if err != nil {
		return err
	}
	defer b.Close()
	var last error
	for i := 0; i < tries; i++ {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, last = b.Call(cctx, "ktw-agents:tasks/Groups", map[string]any{})
		cancel()
		if last == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return last
}

// Stop stops every process running from the case's bin folder, and a hub
// from elsewhere that serves the case's universe.
func (e *Engine) Stop(ctx context.Context) error {
	bin := e.RT.Bin()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		script := fmt.Sprintf("Get-Process | Where-Object { $_.Path -and $_.Path.StartsWith('%s', 'OrdinalIgnoreCase') } | Stop-Process -Force -ErrorAction SilentlyContinue", strings.ReplaceAll(bin, "'", "''"))
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	} else {
		cmd = exec.CommandContext(ctx, "pkill", "-f", bin)
	}
	_ = cmd.Run() // nothing to stop is not an error
	// A hub serving the case from outside its bin folder is stopped too, with
	// what it started: the copy a GUI spawns from the sandbox's control plane,
	// or a hub from elsewhere. A pid whose executable cannot be told is never
	// killed on a guess (a stale lockfile's pid may be anyone's now).
	if pid, path := e.servingHub(); pid > 0 && path != "" && !within(e.RT.Bin(), path) {
		e.logf("stopping the hub serving %s from outside the case's bin folder (pid %d, %s)", e.Case.UniverseName(), pid, path)
		killTree(ctx, pid)
	}
	deadline := time.Now().Add(20 * time.Second)
	for e.Running(ctx) {
		if time.Now().After(deadline) {
			return fmt.Errorf("the universe still answers 20s after stopping its processes")
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Give Windows time to release the files the processes held.
	time.Sleep(1500 * time.Millisecond)
	e.logf("universe %s stopped", e.Case.UniverseName())
	return nil
}
