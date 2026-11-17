// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// GUILauncher opens the desktop app — on universe when one is named (the main
// universe of the solution setup just installed). Supplied by the katyoti app,
// which owns binary resolution, process spawning and the startup config; this
// package only decides WHEN.
type GUILauncher func(cmd *cobra.Command, universe string) error

// DefaultUniverse makes a universe the one katyoti opens on, unless the person
// already chose one. Supplied by the katyoti app, which owns the startup config.
type DefaultUniverse func(universe string)

// Shortcuts gives the installation setup runs in its own shortcuts. Supplied by
// the katyoti app, which owns the installation and the desktop.
type Shortcuts func(cmd *cobra.Command) error

// solutionInstaller is what setup asks of the marketplace engine for the
// channel's solution: installing it, nothing else — no program is installed by
// setup (the Ready to start screen does it).
type solutionInstaller interface {
	InstallSolution(ctx context.Context, in mktapi.InstallSolutionInput) (mktapi.InstallSolutionOutput, error)
}

// setupInfra and setupEngine resolve the services setup drives; a test
// replaces them.
var (
	setupInfra  = resolveInfra
	setupEngine = func(cmd *cobra.Command) (solutionInstaller, error) { return resolve(cmd) }
)

// solutionProject is the folder of the project of the solution setup installs,
// in the installation's projects folder (<Documents>/Katyoti until the person
// chooses one): <projects>/<installation> for a program of an installation —
// each installation its own, never another's — else <projects>/<solution>.
func solutionProject(solution string) string {
	name := solution
	inst := cenv.CurrentInstallation()
	if inst != nil {
		name = filepath.Base(inst.Dir())
	}
	projects, _ := inst.ProjectsFolder()
	return filepath.Join(projects, name)
}

// NewSetupCommand builds `setup` — first run, end to end.
//
// This is THE path for a new user, and it exists because the alternative is a
// list of four commands in the right order that nobody should have to learn:
//
//	install-infra  →  remember the channel  →  subscribe its marketplace  →  gui
//
// The installer script calls it, so `irm …/install.ps1 | iex` ends with the GUI
// open rather than a PATH hint and a homework assignment.
//
// Idempotent, so it doubles as a repair command on a machine whose setup is
// half-done — which is exactly the state a failed first run leaves behind.
func NewSetupCommand(launch GUILauncher, setDefault DefaultUniverse, shortcuts Shortcuts) *cobra.Command {
	var noGUI, noSolution, noShortcut bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "First run: install the control plane, subscribe its marketplace, install its solution, open the GUI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// First run installs the control plane, so it resolves the LOCAL-only
			// InfraService: this is the one process where nothing being installed
			// is already running.
			s, err := setupInfra(cmd)
			if err != nil {
				return err
			}
			out, err := s.SetupMachine(cmd.Context())
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(out.Installed) > 0 {
				fmt.Fprintf(w, "control plane %s installed: %s\n", out.Version, strings.Join(out.Installed, ", "))
			} else {
				fmt.Fprintln(w, "control plane already current")
			}
			if len(out.Skipped) > 0 {
				// Not a failure: Wails builds ktw-gui only on its host OS, so a
				// channel legitimately lacks it for some platforms.
				fmt.Fprintf(w, "not served for this platform: %s\n", strings.Join(out.Skipped, ", "))
			}
			switch {
			case out.GUIRuntime == "installed":
				fmt.Fprintln(w, "the GUI's runtime (Microsoft Edge WebView2) installed")
			case out.GUIRuntimeError != "":
				fmt.Fprintf(cmd.ErrOrStderr(), "the GUI's runtime is missing (%s): the GUI will ask to install it\n", out.GUIRuntimeError)
			}
			fmt.Fprintf(w, "infra channel: %s\n", out.Channel)
			if out.Pinned {
				// Worth saying out loud: from here on, every fetch from this
				// channel is verified against the key adopted just now.
				fmt.Fprintln(w, "  pinned to the publisher's key — later updates are signature-verified")
			}
			switch {
			case out.Marketplace != "":
				fmt.Fprintf(w, "marketplace %q subscribed\n", out.Marketplace)
			case out.MarketplaceError != "":
				// Say what still works and what to do, rather than only what broke.
				fmt.Fprintf(cmd.ErrOrStderr(),
					"no marketplace subscribed (%s)\n  the control plane works; add one with `katyoti marketplace add <name> <url>`\n",
					out.MarketplaceError)
			}
			// The solution the channel names: installed under its own name, its
			// main universe the one to open on, and the default from now on.
			universe := ""
			if out.Solution != "" && !noSolution {
				if inst, err := setupEngine(cmd); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "solution %s not installed (%v)\n", out.Solution, err)
				} else {
					universe = installChannelSolution(cmd, inst, out.Solution)
				}
			}
			if universe != "" && setDefault != nil {
				setDefault(universe)
			}
			// Its shortcuts, named after it: never a failed setup either.
			if !noShortcut && shortcuts != nil {
				if err := shortcuts(cmd); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "no shortcut (%v)\n", err)
				}
			}
			if noGUI || launch == nil {
				fmt.Fprintln(w, "run `katyoti gui` to open the desktop app")
				return nil
			}
			fmt.Fprintln(w, "opening the GUI…")
			return launch(cmd, universe)
		},
	}
	cmd.Flags().BoolVar(&noGUI, "no-gui", false, "stop after setup instead of opening the desktop app (CI / headless)")
	cmd.Flags().BoolVar(&noSolution, "no-solution", false, "do not install the solution the channel names")
	// --no-demo is the flag's former name, kept for the scripts that pass it.
	cmd.Flags().BoolVar(&noSolution, "no-demo", false, "former name of --no-solution")
	_ = cmd.Flags().MarkHidden("no-demo")
	cmd.Flags().BoolVar(&noShortcut, "no-shortcut", false, "do not make the installation's Start-menu and Desktop shortcuts (Windows)")
	return cmd
}

// installChannelSolution installs the channel's solution under its own name —
// its main universe named after it, each other one beside it as
// <solution>-<name> — its project in solutionProject(), and answers the
// universe to open: the main one, made the default. "" when it could not be
// installed (a key it needs, a marketplace unreachable): NEVER a failed setup,
// the control plane works and the solution installs later from Marketplace ›
// Solutions. Again on a set-up machine it repairs: a project folder that holds
// files is kept as it is. It installs no program: what the members need is
// installed from the Ready to start screen.
func installChannelSolution(cmd *cobra.Command, s solutionInstaller, solution string) string {
	w := cmd.OutOrStdout()
	ref := parseRef(solution)
	res, err := s.InstallSolution(cmd.Context(), mktapi.InstallSolutionInput{
		Ref: ref, UniverseHome: ref.Name, ProjectDir: solutionProject(ref.Name), SolutionName: true,
	})
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "solution %s not installed (%v)\n  install it later from Marketplace › Solutions\n", solution, err)
		return ""
	}
	main := mainUniverse(res, ref.Name)
	fmt.Fprintf(w, "solution %s installed: universe %s, project %s\n", solution, main, res.Project)
	return main
}

// mainUniverse is the universe a solution install opens on: its main
// universe's registered name, else fallback (an install that reports none).
func mainUniverse(res mktapi.InstallSolutionOutput, fallback string) string {
	if res.Main != "" {
		return res.Main
	}
	return fallback
}
