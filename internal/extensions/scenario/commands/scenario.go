// SPDX-License-Identifier: Apache-2.0

// Package commands is the `scenario` command: install, run and replay
// scenario test cases (internal/extensions/scenario).
package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/internal/machine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/sandbox"
)

type flags struct {
	cases, root, bin string
}

// NewScenarioCommand is `scenario <verb>`.
func NewScenarioCommand() *cobra.Command {
	f := &flags{}
	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Scenario test cases: install a case into its own universe, run its stages, checkpoint and replay",
		Long: `A scenario test case is a folder (test_files/scenarios/<case>/): test-case.yaml (the
universe and its sidecars, the project created from the case's folder, the stages
with their steps and checks), user.md (the person a simulated user plays, answering
the Inbox), and the project's initial content.

The case runs in its own runtime folder (<root>/<case>: bin, universe, project,
data, checkpoints, runs) and its own universe (scn-<case>), started headless and
driven through its hub as the GUI drives it. Every stage that passes is
checkpointed: when a stage fails, fix the cause and replay just that stage:

  ktw-compose scenario install todo-bun --root D:\TestProjects
  ktw-compose scenario run todo-bun            # continue after the last stage passed
  ktw-compose scenario run todo-bun --from s2  # restore the checkpoint before s2, replay from s2
  ktw-compose scenario run todo-bun --from s2 --refresh-bin   # the same, with the fix just built
  ktw-compose scenario status todo-bun

Root: --root, else $KTW_TEST_ROOT, else ~/katyoti-tests.`,
	}
	root := sandbox.DefaultRoot()
	cmd.PersistentFlags().StringVar(&f.cases, "cases", filepath.Join("test_files", "scenarios"), "Folder holding the case folders")
	cmd.PersistentFlags().StringVar(&f.root, "root", root, "Folder the cases run in (one runtime folder per case)")
	cmd.PersistentFlags().StringVar(&f.bin, "bin", "bin", "Folder the sidecars' binaries are copied from (install; refresh-bin defaults to the install's)")

	engine := func(name string) (*core.Engine, error) {
		c, err := api.Load(filepath.Join(f.cases, name))
		if err != nil {
			return nil, err
		}
		cases, _ := filepath.Abs(f.cases)
		c.Dir = filepath.Join(cases, name)
		root, _ := filepath.Abs(f.root)
		bin, _ := filepath.Abs(f.bin)
		e := core.NewEngine(c, root, bin, cmd.OutOrStdout())
		e.InstalledBinary = controlplane.TargetPath
		e.CasesDir = cases
		// An installed case refreshes from where it was installed from, unless
		// --bin says otherwise: never a default that names another folder.
		if !cmd.PersistentFlags().Changed("bin") {
			if src := core.BinSource(e.RT); src != "" {
				e.BinSrc = src
			}
		}
		return e, nil
	}
	one := func(use, short string, run func(cmd *cobra.Command, e *core.Engine, args []string) error) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				// Past the arguments, a failure is the verb's own (a stage
				// that failed): its report says it, the usage would bury it.
				cmd.SilenceUsage = true
				e, err := engine(args[0])
				if err != nil {
					return err
				}
				// The case is a closed world: worked on only inside its
				// sandbox, and the installation the command was run from is
				// watched for any write while the verb runs.
				if err := e.RequireSandbox(); err != nil {
					return err
				}
				guard := core.WatchLeaks(sandbox.Host(), e.OwnFolders()...)
				// A clean-machine run touches no universe of the case's.
				if cmd.Name() != "install" && cmd.Name() != "machine" {
					if err := e.Adopt(cmd.Context()); err != nil {
						return err
					}
				}
				err = run(cmd, e, args[1:])
				// A Windows Sandbox cannot write to this machine: what
				// changes here meanwhile is someone else's, not a leak.
				if cmd.Name() == "machine" {
					if kind, _ := cmd.Flags().GetString("kind"); !machine.WatchesHost(e.Case, kind) {
						return err
					}
				}
				leak := guard.Check()
				for _, n := range guard.Notes {
					fmt.Fprintln(cmd.ErrOrStderr(), "note: not the case's: "+n)
				}
				if leak != nil {
					return errors.Join(err, leak)
				}
				return err
			},
		}
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the cases and where each stands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, err := os.ReadDir(f.cases)
			if err != nil {
				return err
			}
			for _, d := range entries {
				if !d.IsDir() {
					continue
				}
				e, err := engine(d.Name())
				if err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%-20s (invalid: %v)\n", d.Name(), err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s — %s\n", e.Case.Name, e.Case.Title, core.Summary(e))
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "new <case>",
		Short: "Create a case folder from the template, to fill in",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := filepath.Join(f.cases, args[0])
			if err := core.Scaffold(dir, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "case created at %s — fill in test-case.yaml, user.md and project/\n", dir)
			return nil
		},
	})

	install := one("install <case>", "Install the case from scratch: binaries, universe, project from the case's folder, checkpoint `installed`",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return e.Install(cmd.Context(), force)
		})
	install.Flags().Bool("force", false, "Remove a previous install of the case first")
	cmd.AddCommand(install)

	cmd.AddCommand(one("start <case>", "Start the case's universe headless (its hub, then ktw-agents)",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error { return e.Start(cmd.Context()) }))
	cmd.AddCommand(one("stop <case>", "Stop the case's universe (every process running from its bin folder)",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error { return e.Stop(cmd.Context()) }))

	cmd.AddCommand(one("status <case>", "Where the case stands: stages passed, last failure, checkpoints, running",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error {
			return core.PrintStatus(cmd.Context(), e, cmd.OutOrStdout())
		}))

	run := one("run <case>", "Run the stages: after the last one passed, or --from a stage (its checkpoint restored first)",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error {
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			noCp, _ := cmd.Flags().GetBool("no-checkpoint")
			refresh, _ := cmd.Flags().GetBool("refresh-bin")
			failed, err := e.Run(cmd.Context(), core.RunOptions{From: from, To: to, NoCheckpoint: noCp, RefreshBin: refresh})
			if err != nil {
				return err
			}
			if failed != "" {
				return fmt.Errorf("stage %s failed", failed)
			}
			return nil
		})
	run.Flags().String("from", "", "Restore the checkpoint before this stage, then run from it")
	run.Flags().String("to", "", "Stop after this stage")
	run.Flags().Bool("no-checkpoint", false, "Do not checkpoint the stages that pass")
	run.Flags().Bool("refresh-bin", false, "Copy the binaries from --bin again first (after building a fix)")
	cmd.AddCommand(run)
	cmd.AddCommand(one("refresh-bin <case>", "Stop the case's universe and copy its binaries from --bin again (checkpoints kept)",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error { return e.RefreshBinaries(cmd.Context()) }))

	cmd.AddCommand(one("refresh-project <case>", "Stop the case's universe and take its project.live project's data again (project.cwm.yaml and modules/), before trying a migration on today's objects",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error { return e.RefreshProject(cmd.Context()) }))

	cmd.AddCommand(one("checkpoint <case> <name>", "Take a checkpoint of the case now",
		func(cmd *cobra.Command, e *core.Engine, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("name the checkpoint")
			}
			return e.Checkpoint(cmd.Context(), args[0])
		}))
	cmd.AddCommand(one("restore <case> <checkpoint>", "Put the case back to a checkpoint (installed, or a stage that passed)",
		func(cmd *cobra.Command, e *core.Engine, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("name the checkpoint (have %s)", strings.Join(e.Checkpoints(), ", "))
			}
			return e.Restore(cmd.Context(), args[0])
		}))

	snapshot := one("snapshot <case>", "Publish the case as a marketplace solution with its seed (its members pinned by content, its project and tasks), its config-sets, vaults and documents, from a checkpoint or as it stands",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error {
			var o core.SnapshotOptions
			o.Checkpoint, _ = cmd.Flags().GetString("checkpoint")
			o.Version, _ = cmd.Flags().GetString("version")
			o.Marketplace, _ = cmd.Flags().GetString("marketplace")
			o.Name, _ = cmd.Flags().GetString("name")
			o.SeedName, _ = cmd.Flags().GetString("seed-name")
			o.ConfigSets, _ = cmd.Flags().GetStringSlice("config-set")
			o.Vaults, _ = cmd.Flags().GetStringSlice("vault")
			others, _ := cmd.Flags().GetStringSlice("other-config-set")
			for _, oc := range others {
				name, ref, ok := strings.Cut(oc, "=")
				if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(ref) == "" {
					return fmt.Errorf("--other-config-set must be <universe>=<name>@<version>, got %q", oc)
				}
				if o.OtherConfigSets == nil {
					o.OtherConfigSets = map[string][]string{}
				}
				name = strings.TrimSpace(name)
				o.OtherConfigSets[name] = append(o.OtherConfigSets[name], strings.TrimSpace(ref))
			}
			if d, _ := cmd.Flags().GetString("docs"); d != "" {
				o.Docs, _ = filepath.Abs(d)
			}
			if wn, _ := cmd.Flags().GetString("whats-new"); wn != "" {
				o.WhatsNew, _ = filepath.Abs(wn)
			}
			if o.Version == "" || o.Marketplace == "" {
				return fmt.Errorf("--version and --marketplace are required")
			}
			o.Marketplace, _ = filepath.Abs(o.Marketplace)
			if st, err := os.Stat(o.Marketplace); err == nil && st.IsDir() {
				o.Marketplace = filepath.Join(o.Marketplace, "katyoti-marketplace.yaml")
			}
			if folder, _ := cmd.Flags().GetString("key-folder"); folder != "" {
				prefix, _ := cmd.Flags().GetString("key-prefix")
				o.SecretKey = filepath.Join(folder, prefix+".key")
			}
			r, err := e.Snapshot(o)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "solution %s@%s published into %s\n", r.Name, r.Version, o.Marketplace)
			if len(r.Universes) > 1 {
				fmt.Fprintf(out, "  universes %s (main: %s)\n", strings.Join(r.Universes, ", "), r.Universes[0])
			}
			for _, m := range r.Members {
				fmt.Fprintf(out, "  member %s@%s\n", m.Name, m.Version)
			}
			if r.Seed != "" {
				fmt.Fprintf(out, "  seed %s (%d files)\n", r.Seed, r.SeedFiles)
			}
			for _, sd := range r.OtherSeeds {
				fmt.Fprintf(out, "  seed %s\n", sd)
			}
			if r.Infra != "" {
				fmt.Fprintf(out, "  infra %s\n", r.Infra)
			}
			if r.Documents > 0 {
				fmt.Fprintf(out, "  %d document(s)\n", r.Documents)
			}
			if r.Warning != "" {
				fmt.Fprintln(out, "  warning:", r.Warning)
			}
			return nil
		})
	snapshot.Flags().String("checkpoint", "", "Publish this checkpoint's folders (default: the case as it stands)")
	snapshot.Flags().String("version", "", "Solution version (immutable)")
	snapshot.Flags().String("marketplace", "", "The marketplace (folder or katyoti-marketplace.yaml) to publish into (it must publish the case's binaries)")
	snapshot.Flags().String("name", "", "Solution name (default: the case's name)")
	snapshot.Flags().String("seed-name", "", "Seed name (default: <solution>-seed)")
	snapshot.Flags().StringSlice("config-set", nil, "Config-sets the solution applies, in order, as <name>@<version> (repeatable or comma-separated)")
	snapshot.Flags().StringSlice("vault", nil, "Vaults whose keys the solution imports, as <name>@<version> (repeatable or comma-separated)")
	snapshot.Flags().StringSlice("other-config-set", nil, "A config-set one of the case's other universes applies, as <universe>=<name>@<version> (repeatable or comma-separated)")
	snapshot.Flags().String("docs", "", "Folder of the solution's documents: <lang>/*.md")
	snapshot.Flags().String("whats-new", "", "A version's change log made into the solution's « Quoi de neuf » (French)")
	snapshot.Flags().String("key-folder", "", "Signing key folder (re-signs a signed catalog)")
	snapshot.Flags().String("key-prefix", "", "Signing key prefix (the key is <key-folder>/<key-prefix>.key)")
	cmd.AddCommand(snapshot)

	callCmd := func(action bool) func(cmd *cobra.Command, e *core.Engine, args []string) error {
		return func(cmd *cobra.Command, e *core.Engine, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("name the URI")
			}
			var input any = map[string]any{}
			if len(args) > 1 {
				if err := json.Unmarshal([]byte(args[1]), &input); err != nil {
					return fmt.Errorf("input is not JSON: %w", err)
				}
			}
			raw, err := core.CallOnce(cmd.Context(), e, args[0], input, action)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return nil
		}
	}
	cmd.AddCommand(one("call <case> <provider:slot/Method> [json]", "Call any sidecar method through the case's hub (as the GUI does)", callCmd(false)))
	cmd.AddCommand(one("action <case> <action-uri> [json]", "Run an action URI through ktw-agents' ActionService (as the GUI's buttons do)", callCmd(true)))

	machineCmd := one("machine <case>", "Run the case on a clean machine (Windows Sandbox, or this one closed in a folder of its own): the channel's installer double-clicked, then each machine stage, timed",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error {
			kind, _ := cmd.Flags().GetString("kind")
			channel, _ := cmd.Flags().GetString("channel")
			timeout, _ := cmd.Flags().GetInt("timeout-min")
			only, _ := cmd.Flags().GetStringSlice("only")
			self, err := os.Executable()
			if err != nil {
				return err
			}
			res, err := machine.Prove(cmd.Context(), machine.Options{
				Case: e.Case, Kind: kind, Channel: channel, Dir: filepath.Join(e.RT.Dir, "clean-machine"),
				Runner: self, Timeout: time.Duration(timeout) * time.Minute, Env: os.Environ(), Out: cmd.OutOrStdout(), Only: only,
				RunCase: runCaseStages,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout())
			machine.Report(cmd.OutOrStdout(), res)
			if !res.Passed {
				return fmt.Errorf("the case did not pass on the clean machine")
			}
			return nil
		})
	machineCmd.Flags().String("kind", "", "windows-sandbox or local (default: the case's machine.kind, else windows-sandbox)")
	machineCmd.Flags().String("channel", "", "The infra channel folder (<marketplace>/infra) the installer is taken from (default: the case's machine.channel)")
	machineCmd.Flags().Int("timeout-min", 0, "Bound the whole run (default: the case's machine.timeout_min, else 120)")
	machineCmd.Flags().StringSlice("only", nil, "Run only these machine stages, by id (a hand-over: the installer and the GUI, then the machine is yours to test by hand); the Sandbox's preparation still runs")
	cmd.AddCommand(machineCmd)

	stagesCmd := &cobra.Command{
		Use:   "machine-stages <plan.json>",
		Short: "On the clean machine itself: run a plan's stages (the installer, then each command), writing timed results (what a Windows Sandbox's bootstrap runs)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			results, _ := cmd.Flags().GetString("results")
			plan, err := machine.LoadPlan(args[0])
			if err != nil {
				return err
			}
			world := machine.MachineWorld(os.Environ())
			world.RunCase = runCaseStages
			res, err := machine.Run(cmd.Context(), plan, world, results, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			machine.Report(cmd.OutOrStdout(), res)
			if !res.Passed {
				return fmt.Errorf("the case did not pass on this machine")
			}
			return nil
		},
	}
	stagesCmd.Flags().String("results", ".", "The folder the results are written into (results.json)")
	cmd.AddCommand(stagesCmd)

	guiCmd := one("gui <case>", "Write the case's GUI launcher (open-gui.cmd / open-gui.sh: the case's universe started as the scenario runs it, ktw-gui opened on it - on an element with --open <address>) and say how to drive the GUI",
		func(cmd *cobra.Command, e *core.Engine, _ []string) error {
			open, _ := cmd.Flags().GetString("open")
			path, err := e.WriteLauncherOpen(e.CasesDir, open)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "GUI launcher: %s — double-click it (or run it) to open the case in ktw-gui.\n\n", path)
			fmt.Fprint(cmd.OutOrStdout(), core.GUIHelp(e))
			return nil
		})
	guiCmd.Flags().String("open", "", "start the GUI on an element's address (object/<id>/<tab>, screen/<screen>, home/<entry>, rail/<entry>; the case's universe when it names none)")
	cmd.AddCommand(guiCmd)
	return cmd
}

// runCaseStages runs a clean machine's case stage through the scenario
// engine, on the installed universe the runner found.
func runCaseStages(ctx context.Context, c api.MachineCase, t machine.CaseTarget, out io.Writer) error {
	return core.RunStagesOn(ctx, c.Name, c.Resolved, c.User,
		core.InstalledUniverse{Name: t.Universe, Home: t.Home, Data: t.Data, Project: t.Project, Work: t.Work}, out)
}
