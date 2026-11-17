// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// A UNIVERSE STARTED AND STOPPED WITHOUT THE GUI: `katyoti
// universe start` runs the universe's hub detached — the way the GUI's
// orchestrator starts one, minus the GUI — and asks ktw-agents through it, so
// the hub spawns the member serving the workspace; `status` says what runs;
// `stop` ends the hub and the members it spawned. A GUI opened afterwards
// attaches to that hub (it connects find-or-spawn, and never owns a hub it
// found); the command line is its client.

// runtimeOptions are start's knobs: the hub binary (dev: a fresh build) and
// how long to wait for ktw-agents to answer through the hub.
type runtimeOptions struct {
	HubBinaryPath string
	MemberWait    time.Duration
}

// universeStatus is what status reports.
type universeStatus struct {
	Home     string              `json:"home"`
	DataDir  string              `json:"data_dir"`
	Hub      *hubclient.LockFile `json:"hub,omitempty"`     // the hub the lockfile names; nil: none alive
	Stale    bool                `json:"stale,omitempty"`   // a lockfile names a dead process
	Serving  map[string]string   `json:"serving,omitempty"` // provider → "pid N"
	Reaching bool                `json:"reaching"`          // the hub answered a connection
	// Hubs is every hub serving the home: the one the lockfile names and each
	// live process that started a serving member — a hub the lockfile no
	// longer names is listed, never hidden behind "stopped".
	Hubs []hubSeen `json:"hubs,omitempty"`
}

// hubSeen is one hub found serving a universe.
type hubSeen struct {
	PID        int      `json:"pid"`
	Program    string   `json:"program,omitempty"`
	Registered bool     `json:"registered"`       // the universe's lockfile names it
	Serves     []string `json:"serves,omitempty"` // the providers whose serving member it started
}

// unregistered answers the hubs serving the home that its lockfile does not name.
func (st universeStatus) unregistered() []hubSeen {
	var out []hubSeen
	for _, h := range st.Hubs {
		if !h.Registered {
			out = append(out, h)
		}
	}
	return out
}

// startUniverse starts the universe at home's hub, detached, unless one runs,
// then asks ktw-agents through it so the workspace is served. It answers
// whether this call started the hub.
func startUniverse(ctx context.Context, home string, opts runtimeOptions, out io.Writer) (bool, error) {
	env := application.EnvOf(home)
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{
		DataDir:       env.DataDir(),
		Mode:          hubclient.ModeFindOrSpawn,
		Detach:        true,
		SpawnEnv:      hubclient.SpawnEnvForWorkspace(home),
		HubBinaryPath: opts.HubBinaryPath,
	})
	if err != nil {
		return false, err
	}
	defer hc.Close() // the connection only: a detached or found hub keeps running
	spawned := hc.Spawned()
	lf := hc.LockFile()
	if spawned {
		fmt.Fprintf(out, "universe started: hub pid %d at %s (data %s)\n", lf.PID, lf.NATSURL, env.DataDir())
	} else {
		fmt.Fprintf(out, "universe already running: hub pid %d at %s\n", lf.PID, lf.NATSURL)
	}
	// The first call through the hub makes it spawn the serving ktw-agents;
	// a universe without one answers nothing and says so, the hub runs still.
	wait := opts.MemberWait
	if wait <= 0 {
		wait = 90 * time.Second
	}
	tenantID, universeID := cenv.ResolveTenantUniverse(env.UniverseDir())
	sender := messagebus.NewRetrySender(messagebus.NewSender(hc.MessageBus(), tenantID, universeID), "ktw-hub")
	deadline := time.Now().Add(wait)
	var last error
	for {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, last = sender.Call(cctx, "ktw-agents:/tasks/groups", map[string]any{})
		cancel()
		if last == nil {
			fmt.Fprintln(out, "ktw-agents serves the workspace")
			return spawned, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintf(out, "the hub runs, but ktw-agents did not answer through it: %v\n", last)
	return spawned, nil
}

// stopUniverse ends the universe at home's hub and its members.
func stopUniverse(ctx context.Context, home string, out io.Writer) error {
	pid, err := hubclient.Stop(ctx, application.EnvOf(home).DataDir())
	if err != nil {
		return err
	}
	if pid == 0 {
		fmt.Fprintln(out, "universe not running")
		return nil
	}
	fmt.Fprintf(out, "universe stopped: hub pid %d and its members ended\n", pid)
	return nil
}

// restartUniverse is a universe restarted in one action: its
// hub and every member ended, then started again - each program reads its
// keys, config and pinned version anew. A universe that does not run is
// started.
func restartUniverse(ctx context.Context, home string, opts runtimeOptions, out io.Writer) error {
	if err := stopUniverse(ctx, home, out); err != nil {
		return fmt.Errorf("restart: the universe did not stop: %w", err)
	}
	_, err := startUniverse(ctx, home, opts, out)
	return err
}

// statusOf reads what runs for the universe at home: the hub its lockfile
// names, the members holding a serving lock, and the process that started
// each member — so a hub serving the home that no lockfile names is listed.
func statusOf(ctx context.Context, home string) universeStatus {
	env := application.EnvOf(home)
	st := universeStatus{Home: home, DataDir: env.DataDir()}
	if lf, err := hubclient.ReadLockFile(env.DataDir()); err == nil && lf != nil {
		if hubclient.ProcessAlive(lf.PID) {
			st.Hub = lf
			st.Hubs = append(st.Hubs, hubSeen{PID: lf.PID, Registered: true})
		} else {
			st.Stale = true
		}
	}
	st.Serving = ktslocal.ServingProviders(env.DataDir())
	st.Hubs = hubsOfMembers(st.Hubs, st.Serving)
	if st.Hub != nil {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if hc, err := hubclient.ConnectToHub(cctx, hubclient.ConnectOptions{DataDir: env.DataDir(), Mode: hubclient.ModeReuse}); err == nil {
			st.Reaching = true
			_ = hc.Close()
		}
	}
	return st
}

// hubsOfMembers adds to hubs the live process that started each serving
// member (its lock's note "pid N"), the provider to the hub's Serves.
func hubsOfMembers(hubs []hubSeen, serving map[string]string) []hubSeen {
	providers := make([]string, 0, len(serving))
	for p := range serving {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	for _, p := range providers {
		pid := notePID(serving[p])
		if pid <= 0 {
			continue
		}
		ppid, program := parentProcess(pid)
		if ppid <= 0 || !hubclient.ProcessAlive(ppid) {
			continue
		}
		i := slices.IndexFunc(hubs, func(h hubSeen) bool { return h.PID == ppid })
		if i < 0 {
			hubs = append(hubs, hubSeen{PID: ppid})
			i = len(hubs) - 1
		}
		if hubs[i].Program == "" {
			hubs[i].Program = program
		}
		hubs[i].Serves = append(hubs[i].Serves, p)
	}
	return hubs
}

// notePID reads the pid of a serving lock's note ("pid N"); 0 when none.
func notePID(note string) int {
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(note), "pid")))
	if err != nil {
		return 0
	}
	return n
}

func (st universeStatus) print(out io.Writer) {
	others := st.unregistered()
	switch {
	case st.Hub != nil:
		fmt.Fprintf(out, "running: hub pid %d at %s since %s (reachable: %v)\n", st.Hub.PID, st.Hub.NATSURL, st.Hub.StartedAt, st.Reaching)
	case len(others) > 0:
		fmt.Fprintf(out, "served by %d hub(s) no lockfile names (`universe stop` does not reach them: end them by pid)\n", len(others))
	case len(st.Serving) > 0:
		fmt.Fprintln(out, "no hub, but members still serve it (the process that started them is gone)")
	case st.Stale:
		fmt.Fprintln(out, "stopped (a stale lockfile names a dead hub; `universe stop` removes it)")
	default:
		fmt.Fprintln(out, "stopped")
	}
	for _, h := range others {
		program := h.Program
		if program == "" {
			program = "unknown program"
		}
		fmt.Fprintf(out, "  unregistered hub: pid %d (%s) started %s\n", h.PID, program, strings.Join(h.Serves, ", "))
	}
	if st.Hub != nil && len(st.Serving) == 0 {
		fmt.Fprintln(out, "  serving: nothing yet (a member is spawned on its first call)")
	}
	names := make([]string, 0, len(st.Serving))
	for p := range st.Serving {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		fmt.Fprintf(out, "  serving: %s (%s)\n", p, st.Serving[p])
	}
	if st.Stale && st.Hub == nil && (len(others) > 0 || len(st.Serving) > 0) {
		fmt.Fprintln(out, "  a stale lockfile names a dead hub")
	}
	fmt.Fprintf(out, "  home %s\n  data %s\n", st.Home, st.DataDir)
}

// universeHomeArg resolves the universe named (a registered name or a folder;
// none: the current workspace).
func universeHomeArg(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 0 {
		return application.Env().WorkspaceDir(), nil
	}
	return resolveUniverseHome(cmd, args[0])
}

func universeStartCmd() *cobra.Command {
	var hubBinary string
	cmd := &cobra.Command{
		Use:   "start [name|folder]",
		Short: "Start a universe without the GUI: its hub, detached, and the ktw-agents serving its workspace; a GUI opened later attaches to it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := universeHomeArg(cmd, args)
			if err != nil {
				return err
			}
			_, err = startUniverse(cmd.Context(), home, runtimeOptions{HubBinaryPath: hubBinary}, cmd.OutOrStdout())
			return err
		},
	}
	cmd.Flags().StringVar(&hubBinary, "hub-binary", "", "The ktw-hub binary to run (default: the installed control plane's, else ./bin, else PATH)")
	return cmd
}

func universeStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop [name|folder]",
		Short: "Stop a universe's hub and the members it spawned (started by `universe start`, `ktw-agents --hub keep`, or the GUI)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := universeHomeArg(cmd, args)
			if err != nil {
				return err
			}
			return stopUniverse(cmd.Context(), home, cmd.OutOrStdout())
		},
	}
}

func universeRestartCmd() *cobra.Command {
	var hubBinary string
	cmd := &cobra.Command{
		Use:   "restart [name|folder]",
		Short: "Restart a universe in one action: its hub and members stopped, then started again, each reading its keys, config and programs anew (a universe that does not run is started)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := universeHomeArg(cmd, args)
			if err != nil {
				return err
			}
			return restartUniverse(cmd.Context(), home, runtimeOptions{HubBinaryPath: hubBinary}, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&hubBinary, "hub-binary", "", "The ktw-hub binary to run (default: the installed control plane's, else ./bin, else PATH)")
	return cmd
}

func universeStatusCmd() *cobra.Command {
	var outFormat string
	cmd := &cobra.Command{
		Use:   "status [name|folder]",
		Short: "What runs for a universe: its hub (pid, address, since when), every other hub serving it that no lockfile names, and the members serving it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := universeHomeArg(cmd, args)
			if err != nil {
				return err
			}
			st := statusOf(cmd.Context(), home)
			switch outFormat {
			case "json":
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(st)
			case "", "text":
				st.print(cmd.OutOrStdout())
				return nil
			default:
				return fmt.Errorf("--out %q: text or json", outFormat)
			}
		},
	}
	cmd.Flags().StringVar(&outFormat, "out", "text", "Output: text | json")
	return cmd
}
