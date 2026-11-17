// SPDX-License-Identifier: Apache-2.0

// Package cmd wires the ktw-agents binary: the task runner, its I/O
// model and console, the CWM host and the services a universe serves.
package cmd

import (
	"context"
	"embed"
	"fmt"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/server"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/spf13/cobra"
)

var (
	version    = "dev"
	embeddedFS embed.FS
	bootApp    *application.Application // Application created at boot for CLI registration
)

var rootCmd = &cobra.Command{
	Use:   "ktw-agents",
	Short: "ktw-agents — LLM-powered skill execution engine",
	Long: `ktw-agents packages domain-specific skills into guided interactive
wizards powered by LLMs. Define tasks and skills as Markdown files,
and ktw-agents handles the conversation loop, tool dispatch, and output generation.`,
	Version:           version,
	PersistentPreRunE: earlyInit,
	RunE:              runRoot,
}

var (
	flagHome         string
	flagUniverse     string
	flagWorkspaceDir string
)

func init() {
	// Universe selection flags. Both are pre-scanned by cenv.Preflight at
	// process boot (BEFORE Cobra runs) so that KTW_WORKSPACE is set before any
	// config / .env loading. Cobra still declares them here for --help and
	// validation. Mutual exclusion is enforced by cenv.Preflight.
	rootCmd.PersistentFlags().StringVar(&flagHome, "home", "",
		"Universe home folder (mutually exclusive with --universe)")
	rootCmd.PersistentFlags().StringVar(&flagUniverse, "universe", "",
		"Universe name; resolved via ~/.config/katyoti/universes.yaml (mutually exclusive with --home)")
	// CWM workspace folder override. Read lazily by the cwmlocal factory
	// (no pre-scan needed — service construction happens after Cobra flag
	// parsing). Falls back to KTW_WORKSPACE_DIR env var, then to walk-up
	// discovery of .katyoti from CWD.
	rootCmd.PersistentFlags().StringVar(&flagWorkspaceDir, "workspace-dir", "",
		"Absolute path to the workspace folder (.katyoti). Overrides KTW_WORKSPACE_DIR + auto-discovery.")
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show ktw-agents version",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf("ktw-agents %s\n", version)
	},
}

// earlyInit bootstraps global config and checks first-start condition.
// Runs before ANY command (including subcommands via PersistentPreRunE).
func earlyInit(cmd *cobra.Command, args []string) error {
	// Global() handles .env bootstrap (CWD → home) and HomeDir resolution.
	_ = application.Config()
	env := application.Env()
	server.Init(env.DataDir())

	// Skip universe + Settings checks for commands that don't need them
	// (diagnostics + setup paths; describe: a marketplace publish indexes the
	// binary on a machine that may have no universe at all).
	name := cmd.Name()
	if name == "config" || name == "version" || name == "help" || name == "completion" || name == "check" || name == "dirs" || name == "describe" {
		return nil
	}

	// Univers.yaml must exist at <KTW_WORKSPACE>/.katyoti/config/Univers.yaml — that's the
	// authoritative universe identity. Missing manifest = not a universe.
	if err := env.UniverseError(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	settingsPath := filepath.Join(env.ConfigDir(), "Settings.yaml")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		fmt.Println("ktw-agents is not configured yet.")
		fmt.Println()
		fmt.Println("Run the following command to set up:")
		fmt.Println("  ktw-agents config")
		fmt.Println()
		fmt.Printf("Expected config at: %s\n", env.HomeDir())
		os.Exit(1)
	}

	return nil
}

// runRoot is the default action when ktw-agents is run with no subcommand.
// This ensures PersistentPreRunE fires (Cobra skips pre-run hooks for commands with no Run/RunE).
func runRoot(cmd *cobra.Command, args []string) error {
	return cmd.Usage()
}

// Execute runs the CLI with the given embedded filesystem.
// Each binary passes its own embed.FS so the cmd package stays FS-agnostic.
//
// cenv.Preflight has already run at package init time (--home / --universe
// / KTW_WORKSPACE / KTW_UNIVERSE / fallback resolution). KTW_WORKSPACE is set in the
// process env, so application.Env() returns the resolved universe paths.
func Execute(efs embed.FS) error {
	embeddedFS = efs

	// Route framework logs to a file (Env().LogDir()/ktw-agents.log) so they
	// don't pollute stdout/stderr — which carry the actual command output
	// (llm JSON, terminal NDJSON, task results). KTW_LOG_STDERR=1 opts out.
	appcore.SetupFileLogging("ktw-agents")

	// The soft memory limit and the diagnostics endpoint every long-lived
	// program runs under (appcore.ApplyRuntimeGuards). KTW_PPROF=<port> stays
	// this program's own switch for net/http/pprof on 127.0.0.1:<port>, so a
	// hung core's goroutine stacks can be read
	// (curl 127.0.0.1:<port>/debug/pprof/goroutine?debug=2); inherited from
	// the parent env when ktw-gui spawns the universe core.
	appcore.ApplyRuntimeGuards("ktw-agents")
	if port := os.Getenv("KTW_PPROF"); port != "" {
		appcore.ServePprof("ktw-agents", port)
	}

	// Boot-time: build EM and run all 4 phases so extensions can register
	// CLI commands (Bootstrap phase) and contribute strategies/config (Contribute phase).
	// All services (TaskService, etc.) are available after Run() via UseService.
	bootApp = buildApp(runtimeContext{
		Config:     loader.NewFSLoader(application.Env().ConfigDir()),
		HomeDir:    application.Env().HomeDir(),
		EmbeddedFS: embeddedFS,
		AddCommand: func(cmd *cobra.Command) { rootCmd.AddCommand(cmd) },
	})

	// Register the root cobra command as a named CobraCommand service so that
	// generic extensions (e.g. skillslocal which uses appcore.CommandExt) can
	// hang subcommands off it via application.AppUseNamedService[CobraCommand].
	// Sidecars get this for free via appcore.NewAppExt; ktw-agents uses a
	// custom buildApp path, so we wire it explicitly here.
	application.PutService[appcore.CobraCommand](bootApp, "root", appcore.CobraCommand{Cmd: rootCmd})

	if err := bootApp.Run(); err != nil {
		return fmt.Errorf("boot: %w", err)
	}

	rootCmd.AddCommand(appcore.NewDirsCommand())

	// `describe` — the marketplace's self-description verb.
	//
	// Sidecars get this from `appcore.NewAppExt`; ktw-agents builds its app by
	// hand, so it was simply never added — and the consequence was invisible in
	// the wrong place. The catalog indexer skips any binary that cannot answer
	// `describe`, and `index.notSidecars` recorded ktw-agents as "the
	// orchestrator" as if that were a decision. It was not: it is a sidecar
	// wired into universes like any other, and a universe that lists it as a
	// member could not install it, because it was in no catalog and no channel.
	rootCmd.AddCommand(appcore.NewDescribeCommand(appcore.AppConfig{
		AppName:     "ktw-agents",
		Version:     version,
		DisplayName: "ktw-agents",
		Short:       rootCmd.Short,
	}))

	// Wire KTSP CLI surface (`sc ktsp describe`, `sc ktsp stdio`,
	// `sc extensions`, auto-generated CLI for KTSP slots). This exposes
	// the long-lived KTSP-server mode used by ktw-gui's Master to spawn
	// one core per active universe (`ktw-agents sc ktsp stdio`).
	ktslocal.AddKTSPCommands(bootApp, rootCmd, ktslocal.KTSPCommandConfig{
		ExtensionsCmd: func(c *ktsp.Endpoint) *cobra.Command {
			return appcore.NewExtensionsCommand(appcore.AppConfig{AppName: "ktw-agents", Version: version}, c)
		},
		KTSPDescribeCmd: func(c *ktsp.Endpoint) *cobra.Command {
			return appcore.NewKTSPDescribeCommand(appcore.AppConfig{AppName: "ktw-agents", Version: version}, c)
		},
	})

	// SidecarExt's Bootstrap-phase promote step (relocate `dirs`/`ktsp`/
	// `extensions` from root → sc) ran inside bootApp.Run() BEFORE we
	// just added these commands. Run the same promotion now so
	// ktw-agents matches the canonical layout the rest of the binaries
	// inherit automatically.
	promoteToSC(bootApp, rootCmd, "dirs", "ktsp", "extensions")

	// Store Application in context — all Use* functions resolve from this.
	// A first Ctrl+C (or SIGTERM) first closes the task service — the serving
	// process pauses its runs, any other process stops the ones it started —
	// THEN cancels the command's context, so its runs end as paused or stopped,
	// never as failed by the cancellation. A second signal ends the process at
	// once.
	// The person using this command is the cause of every write it makes
	// (person.Cause: the profile's handle, when one is set) — a command an
	// agent's session runs is the agent's (person.CommandCause), never the
	// serving process, which only answers its callers.
	causeOf := person.Cause
	if wantsClient(os.Args[1:]) {
		causeOf = person.CommandCause
	}
	ctx, cancel := context.WithCancel(causeOf(application.WithApplication(context.Background(), bootApp)))
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		signal.Stop(sigs) // the next one takes the default: the process ends
		if svc, err := application.UseService[taskapi.TaskService](ctx); err == nil {
			if c, ok := svc.(interface{ Close() error }); ok {
				_ = c.Close()
			}
		}
		cancel()
	}()
	return appcore.ExecuteCLI(ctx, rootCmd)
}

// promoteToSC moves the named top-level cobra commands under the `sc`
// group, mirroring what `SidecarExt.cli` does at Bootstrap for the
// other sidecars. ktw-agents' custom Execute() flow registers some
// commands AFTER `bootApp.Run()` finishes (because they depend on the
// KTSP container built during Run), so the Bootstrap-phase promote
// can't catch them — this is the post-bootstrap mirror.
func promoteToSC(app *application.Application, root *cobra.Command, names ...string) {
	scGrp, err := application.AppUseNamedService[appcore.CobraCommand](app, appcore.GroupNameSidecar)
	if err != nil {
		return // no sc group registered — non-sidecar binary; leave commands at root
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	children := append([]*cobra.Command(nil), root.Commands()...)
	for _, child := range children {
		if want[child.Use] {
			root.RemoveCommand(child)
			scGrp.Cmd.AddCommand(child)
		}
	}
}

// RootCommand is the binary's command tree as its code and its extensions build
// it - the verbs generated from the KTSP services are added by Execute, not
// here. Exposed so tests can check the commands a task or a skill names.
func RootCommand() *cobra.Command { return rootCmd }

// BuildApp constructs the application exactly as Execute does, without running
// the CLI. Exposed so tests can inspect what this binary wires — keeping any
// such test bound to what actually ships.
//
// The returned application has NOT been run.
func BuildApp(efs embed.FS) *application.Application {
	embeddedFS = efs
	app := buildApp(runtimeContext{
		Config:     loader.NewFSLoader(application.Env().ConfigDir()),
		HomeDir:    application.Env().HomeDir(),
		EmbeddedFS: embeddedFS,
		AddCommand: func(*cobra.Command) {},
	})
	// Mirrors Execute: extensions resolve CobraCommand("root") during AppInit,
	// and ktw-agents wires it explicitly because it does not go through
	// appcore.NewAppExt. Omitting it makes AppInit fail.
	application.PutService[appcore.CobraCommand](app, "root", appcore.CobraCommand{Cmd: rootCmd})
	return app
}
