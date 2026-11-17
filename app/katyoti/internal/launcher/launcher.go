// SPDX-License-Identifier: Apache-2.0

// Package launcher gives the katyoti front door its LAUNCHER face: `katyoti gui`
// starts the ktw-gui desktop app, and a bare `katyoti` (no subcommand) launches
// per the global startup config's launch_mode. This completes "one engine,
// three faces" — the same binary is CLI, bootstrapper, and launcher.
package launcher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/app/katyoti/internal/instance"
	"github.com/katyoti/katyoti-fwk/app/katyoti/internal/startup"
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane/shortcut"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktcommands "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/commands"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

const guiName = "ktw-gui"

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// ResolveGUI finds the ktw-gui binary. Order: the control-plane location (a
// `katyoti install-infra` / `self-update` puts the canonical binary there) → the
// versioned store (a recorded marketplace install) → beside the running exe (dev
// bin/, a bundle, a frozen distribution) → PATH. Returns an actionable error when
// none is found.
func ResolveGUI() (string, error) {
	if p, err := controlplane.TargetPath(guiName); err == nil && fileExists(p) {
		return p, nil
	}
	if p := fromStore(); p != "" {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), guiName+exeSuffix())
		if fileExists(cand) {
			return cand, nil
		}
	}
	if p, err := exec.LookPath(guiName); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s not found — bootstrap it (`katyoti install-infra`) or place it beside this binary", guiName)
}

// fromStore resolves ktw-gui from the recorded available-items inventory (a
// marketplace install). Best-effort: any read error just falls through.
func fromStore() string {
	path, err := catalog.DefaultSidecarsPath()
	if err != nil {
		return ""
	}
	sy, err := catalog.LoadSidecarsYaml(path)
	if err != nil || sy == nil {
		return ""
	}
	for _, e := range sy.Sidecars {
		if e.Name == guiName && e.BinaryPath != "" && fileExists(e.BinaryPath) {
			return e.BinaryPath
		}
	}
	return ""
}

// LaunchGUI resolves + starts ktw-gui in the foreground with inherited stdio, so
// its startup errors surface; it blocks until the GUI exits. extra args pass
// through unchanged.
//
// Before spawning, the launcher owns the pre-hub control-plane bootstrap: it
// ensures the protected admin universe exists on disk + in the registry so the
// GUI (a pure KTSP client — it writes no management state itself) can activate
// admin to reach the universe-registry + marketplace services. Best-effort: a
// bootstrap error is warned, not fatal — the GUI still opens for other work and
// the ktw-univers core self-heals admin on its next boot.
func LaunchGUI(out, errw io.Writer, args ...string) error {
	// Say so when the control plane we are about to run is not the one this
	// binary was built with. ResolveGUI and resolveHubBinary both prefer the
	// INSTALLED copy over ./bin, which is right for a shipped install and
	// silent for a developer: a current launcher happily starts a hub from days
	// ago, and a hub older than the contribution model it loads reports an
	// EMPTY CATALOG rather than an error.
	if self := appcore.ResolveVersion(""); true {
		if w := controlplane.SkewWarning(self, controlplane.VersionSkew(self)); w != "" {
			fmt.Fprint(errw, w)
		}
	}
	if err := admin.EnsureDefault(); err != nil {
		fmt.Fprintf(errw, "warning: admin universe bootstrap failed: %v\n", err)
	}
	gui, err := ResolveGUI()
	if err != nil {
		return err
	}
	c := exec.Command(gui, args...)
	c.Stdout = out
	c.Stderr = errw
	c.Stdin = os.Stdin
	return c.Run()
}

// Command builds `gui` — launch ktw-gui explicitly (regardless of launch_mode).
// `--universe <name>` opens straight into that universe; without it, the startup
// config's default_universe is used, and with neither the GUI shows its picker.
// `--open <address>` opens the GUI on an element - the universe
// the address names, unless --universe says one - which is what a
// katyoti://open/<address> link runs.
func Command() *cobra.Command {
	var universe, infraVersion, open string
	var doUpdate, repair bool
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Launch the ktw-gui desktop app",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			failed := hideOwnConsole(cmd.ErrOrStderr())
			// --update is an EXPLICIT request, so it applies whatever
			// update_check says: that setting gates the passive notice, not a
			// command the user just typed.
			switch {
			case infraVersion != "" || repair:
				applyInfraBeforeLaunch(cmd, infraVersion, repair)
			case doUpdate:
				applyUpdatesBeforeLaunch(cmd)
			default:
				cfg, _ := startup.Load()
				noteAvailableUpdates(cmd, cfg.UpdateCheck != "off")
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "starting ktw-gui…")
			if universe == "" {
				universe = universeOfAddress(open)
			}
			target := resolveLaunchUniverse(universe)
			launchArgs := append(universeArgs(cmd.ErrOrStderr(), target), args...)
			if open != "" {
				launchArgs = append(launchArgs, "--open", open)
			}
			err := LaunchGUI(cmd.OutOrStdout(), cmd.ErrOrStderr(), launchArgs...)
			failed(err)
			return err
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "",
		"Universe to open on launch (default: the startup default_universe)")
	cmd.Flags().StringVar(&open, "open", "",
		"An element to open: its address <universe>/<place>/<path>, or a katyoti://open/ link")
	cmd.Flags().BoolVar(&doUpdate, "update", false,
		"Bring the control plane + sidecars up to date BEFORE launching (opt-in; launch is the safest moment to swap them)")
	cmd.Flags().StringVar(&infraVersion, "infra-version", "",
		"Move the control plane (only) to this channel version BEFORE launching: update, or roll back")
	cmd.Flags().BoolVar(&repair, "repair", false,
		"Reinstall the control plane (only) from the channel BEFORE launching: every binary placed again")
	return cmd
}

// makeShortcuts gives the installation setup runs in its shortcuts, named after
// it, opening its own katyoti (a bare katyoti: the GUI) with ktw-gui's icon.
// Outside an installation (a development build) it makes none.
func makeShortcuts(cmd *cobra.Command) error {
	inst := cenv.CurrentInstallation()
	if inst == nil {
		return nil
	}
	target, err := controlplane.TargetPath("katyoti")
	if err != nil {
		return err
	}
	icon, err := controlplane.TargetPath(guiName)
	if err != nil {
		return err
	}
	made, err := shortcut.Make(inst.Name, target, icon)
	for _, m := range made {
		fmt.Fprintf(cmd.OutOrStdout(), "shortcut %s\n", m)
	}
	// Links to an element open this installation's GUI on it.
	if where, serr := shortcut.RegisterScheme(shortcut.LinkScheme, target); serr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: the %s:// links are not registered: %v\n", shortcut.LinkScheme, serr)
	} else if where != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "links %s:// %s\n", shortcut.LinkScheme, where)
	}
	return err
}

// universeOfAddress is the universe an element's address names: its first
// segment, once a katyoti://open/ prefix is off; "" for none.
func universeOfAddress(addr string) string {
	addr = strings.Trim(strings.TrimPrefix(strings.TrimSpace(addr), shortcut.LinkScheme+"://open/"), "/")
	universe, _, _ := strings.Cut(addr, "/")
	return universe
}

// resolveLaunchUniverse picks the universe to open on launch: the explicit
// --universe flag wins, else the startup config's default_universe. Empty result
// = let the GUI show its welcome picker (gui-universe-selector). "Selection" here
// is which universe the GUI displays first; the multi-container activate/deactivate
// model is a later launcher milestone (hub-multi-container-authority).
func resolveLaunchUniverse(flag string) string {
	if s := strings.TrimSpace(flag); s != "" {
		return s
	}
	cfg, _ := startup.Load()
	return strings.TrimSpace(cfg.DefaultUniverse)
}

// universeArgs builds the pass-through args that tell ktw-gui which universe to
// open. It validates name against the registry: an unregistered name is warned
// about and dropped (the GUI opens its picker rather than failing). Empty name →
// no args.
func universeArgs(errw io.Writer, name string) []string {
	if name == "" {
		return nil
	}
	if !universeRegistered(name) {
		fmt.Fprintf(errw, "warning: universe %q is not registered — opening the universe picker instead\n", name)
		return nil
	}
	return []string{"--universe", name}
}

// universeRegistered reports whether name exists in the cross-universe registry.
// Best-effort: any read error is treated as "not found".
func universeRegistered(name string) bool {
	regPath, err := catalog.DefaultRegistryPath()
	if err != nil {
		return false
	}
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		return false
	}
	_, err = reg.Find(name)
	return err == nil
}

// Extension adds the `gui` command AND makes a bare `katyoti` (no subcommand)
// launch per KatyotiConfig.launch_mode: gui → launch, cli → help, none → hint.
// Setting root.Args = NoArgs keeps typos ("katyoti instal") a clean "unknown
// command" error instead of a stray launch.
func Extension() application.Extension {
	addGUICmd := appcore.CommandExt(Command())
	addInstanceCmd := appcore.CommandExt(instance.Command())
	// `setup` is owned by the marketplace extension (it is a marketplace
	// operation), but its LAST step opens the GUI -- and spawning the GUI is
	// this package's job, not the marketplace's. Injecting the launcher keeps
	// that boundary intact instead of teaching the marketplace how to find and
	// start a desktop binary.
	addSetupCmd := appcore.CommandExt(mktcommands.NewSetupCommand(func(cmd *cobra.Command, universe string) error {
		// The main universe of the solution setup installed opens first.
		target := resolveLaunchUniverse(universe)
		return LaunchGUI(cmd.OutOrStdout(), cmd.ErrOrStderr(), universeArgs(cmd.ErrOrStderr(), target)...)
	}, func(universe string) {
		// ...and is the default from now on, unless the person already chose one.
		if cfg, err := startup.Load(); err == nil && strings.TrimSpace(cfg.DefaultUniverse) == "" {
			_, _ = startup.Set("default_universe", universe)
		}
	}, makeShortcuts))
	// Cobra's Windows "mousetrap" refuses to run a program started from
	// Explorer ("This is a command line tool…") -- which is exactly what the
	// Start-menu / Desktop shortcut does. A bare `katyoti` IS that path.
	cobra.MousetrapHelpText = ""
	return func(init *application.ApplicationInit) {
		addGUICmd(init)
		addInstanceCmd(init)
		addSetupCmd(init)
		init.AddBootstrap("katyoti.launcher", ".root-run", func(app *application.Application) error {
			// Reap `<binary>.old-*` files a previous self-update/install-infra could
			// not delete because the outgoing binary was still running. Best-effort:
			// anything still locked is skipped and swept on a later run.
			_ = controlplane.Sweep()
			// The installation this katyoti belongs to is listed by
			// `katyoti instance list` from its first run (best-effort).
			_ = instance.RegisterCurrent()

			cr, err := application.AppUseNamedService[appcore.CobraCommand](app, "root")
			if err != nil {
				return fmt.Errorf("katyoti launcher: %w", err)
			}
			root := cr.Cmd
			root.Args = cobra.NoArgs
			// A refusal is said once (main prints it), without the usage
			// text: the person asked for something precise; --help is there.
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.RunE = func(cmd *cobra.Command, _ []string) error {
				cfg, _ := startup.Load()
				// Opened from its shortcut, the console katyoti holds is hidden
				// while it opens the GUI; the other modes write to it.
				failed := func(error) {}
				if cfg.LaunchMode != startup.LaunchCLI && cfg.LaunchMode != startup.LaunchNone {
					failed = hideOwnConsole(cmd.ErrOrStderr())
				}
				// Before ANY launch mode, not just the GUI one: whether the machine
				// is behind has nothing to do with what a bare `katyoti` opens.
				noteAvailableUpdates(cmd, cfg.UpdateCheck != "off")
				switch cfg.LaunchMode {
				case startup.LaunchCLI:
					return cmd.Help()
				case startup.LaunchNone:
					fmt.Fprintln(cmd.OutOrStdout(), "katyoti: launch_mode=none — try `katyoti --help` or `katyoti gui`.")
					return nil
				default: // LaunchGUI (and any unrecognized value → the safe default)
					fmt.Fprintln(cmd.ErrOrStderr(), "starting ktw-gui…")
					target := resolveLaunchUniverse("")
					err := LaunchGUI(cmd.OutOrStdout(), cmd.ErrOrStderr(),
						universeArgs(cmd.ErrOrStderr(), target)...)
					failed(err)
					return err
				}
			}
			return nil
		})
	}
}
