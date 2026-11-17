// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

func parseRef(s string) mktapi.ItemRef {
	if name, ver, ok := strings.Cut(s, "@"); ok {
		return mktapi.ItemRef{Name: name, Version: ver}
	}
	return mktapi.ItemRef{Name: s}
}

// NewInstallCommand builds `install <item>[@version]` → the versioned store.
func NewInstallCommand() *cobra.Command {
	var marketplace string
	cmd := &cobra.Command{
		Use:   "install <item>[@version]",
		Short: "Install an item into the versioned store (→ available)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			ref := parseRef(args[0])
			ref.Marketplace = marketplace
			inst, err := s.Install(cmd.Context(), ref)
			if err != nil {
				return err
			}
			// A content item unpacks to a FOLDER and has no binary; printing
			// BinaryPath unconditionally left a bare arrow pointing at nothing.
			where := inst.BinaryPath
			if where == "" {
				where = inst.FolderPath
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed %s@%s → %s\n", inst.Name, inst.Version, where)
			return nil
		},
	}
	cmd.Flags().StringVar(&marketplace, "marketplace", "", "install from this marketplace (required when the name is served by more than one)")
	return cmd
}

// NewUninstallCommand builds `uninstall <item>[@version]`. Bare removes every
// version plus the available record; `@version` retires just that one store
// version and leaves the rest installed.
func NewUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <item>[@version]",
		Short: "Remove an installed item from the store (all versions, or one with @version)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			ref := parseRef(args[0])
			out, err := s.Uninstall(cmd.Context(), mktapi.UninstallInput{Name: ref.Name, Version: ref.Version})
			if err != nil {
				return err
			}
			if out.Version != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "uninstalled %s@%s (other versions kept)\n", out.Name, out.Version)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "uninstalled %s (all versions)\n", ref.Name)
			return nil
		},
	}
}

// NewOutdatedCommand builds `outdated` -- what is behind, in BOTH halves.
//
// Infra and sidecars are diffed against different things (a channel vs. a
// catalog), so they are reported as separate sections rather than merged into
// one list that would silently imply they update the same way.
//
// An unreachable infra channel is reported inline, not fatally: the sidecar
// half is still worth showing.
func NewOutdatedCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "outdated",
		Short: "Show infra and sidecars that are behind",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			anything := false

			infraUps, infraErr := s.InfraOutdated(cmd.Context())
			switch {
			case infraErr != nil:
				fmt.Fprintf(cmd.ErrOrStderr(), "infra: %v\n", infraErr)
			case len(infraUps) > 0:
				anything = true
				fmt.Fprintln(out, "INFRA (moves as a set -- `update --infra`)")
				w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
				fmt.Fprintln(w, "  NAME\tINSTALLED\tLATEST")
				for _, u := range infraUps {
					from := u.Installed
					if from == "" {
						from = "(absent)"
					}
					fmt.Fprintf(w, "  %s\t%s\t%s\n", u.Name, from, u.Latest)
				}
				if err := w.Flush(); err != nil {
					return err
				}
			}

			ups, err := s.Outdated(cmd.Context())
			if err != nil {
				return err
			}
			if len(ups) > 0 {
				if anything {
					fmt.Fprintln(out)
				}
				anything = true
				fmt.Fprintln(out, "SIDECARS (`update --sidecars`, or one by name)")
				w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
				fmt.Fprintln(w, "  NAME\tINSTALLED\tLATEST")
				for _, u := range ups {
					fmt.Fprintf(w, "  %s\t%s\t%s\n", u.Name, u.Installed, u.Latest)
				}
				if err := w.Flush(); err != nil {
					return err
				}
			}

			if !anything {
				fmt.Fprintln(out, "everything is up to date")
			}
			return nil
		},
	}
}

// printUpdates reports what moved: the store, the universes repinned to the
// new version, and the universes installed from a solution
// that keep the version they pin on purpose.
func printUpdates(w io.Writer, done []mktapi.Update) {
	for _, u := range done {
		fmt.Fprintf(w, "updated %s %s -> %s\n", u.Name, u.Installed, u.Latest)
		if len(u.Universes) > 0 {
			fmt.Fprintf(w, "  now run by: %s\n", strings.Join(u.Universes, ", "))
		}
		if len(u.Behind) > 0 {
			// A universe reproduced from a release pins its members on purpose:
			// it moves with its solution's update, or when its owner says so.
			fmt.Fprintf(w, "  still on the old version (installed from a solution): %s\n", strings.Join(u.Behind, ", "))
			fmt.Fprintf(w, "  adopt with: katyoti universe wire %s --universe <name>\n", u.Name)
		}
	}
}

// NewUpdateCommand builds `update [<item>...]` with SELECTORS.
//
//	update                  every infra binary + every outdated sidecar (= --all)
//	update --infra          the control-plane set only
//	update --sidecars       catalog-installed sidecars only
//	update <name>           that one item -- infra binary or sidecar
//	update <name> --to vX   pin/downgrade that one item
//
// One verb with selectors, rather than `update` for sidecars and `self-update`
// for infra: users think "update", and having to know which of two commands
// owns a given binary is exactly the knowledge a front door should absorb.
//
// INFRA MOVES AS A SET. The control-plane binaries speak one KTSP protocol
// version to each other, so naming a single infra binary updates the whole set
// and says why -- a partial infra update is a skew bug you build by hand.
func NewUpdateCommand() *cobra.Command {
	var to string
	var wantInfra, wantSidecars, wantAll bool
	cmd := &cobra.Command{
		Use:   "update [item...]",
		Short: "Update infra and/or sidecars (--infra, --sidecars, --all, or a name)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			if (wantInfra || wantSidecars || wantAll) && len(args) > 0 {
				return fmt.Errorf("--infra/--sidecars/--all select what to update; do not also name items")
			}

			// `--infra --to <version>` is the ROLLBACK: the whole set moves, so
			// there is no skew to build. That is why it is allowed where pinning
			// a single infra binary is refused.
			if wantInfra && to != "" {
				infra, err := resolveInfra(cmd)
				if err != nil {
					return err
				}
				done, err := infra.SetInfraVersion(cmd.Context(), to)
				if err != nil {
					return err
				}
				if len(done) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "infra: already at %s\n", to)
					return nil
				}
				for _, u := range done {
					fmt.Fprintf(cmd.OutOrStdout(), "infra: %s %s -> %s\n", u.Name, u.Installed, u.Latest)
				}
				return nil
			}

			// Naming an INFRA binary means the whole control-plane set.
			if len(args) == 1 && controlplane.IsInfra(args[0]) {
				if to != "" {
					return fmt.Errorf(
						"%s is a control-plane binary: it moves with the infra set, which is versioned as a whole, so it cannot be pinned on its own",
						args[0])
				}
				fmt.Fprintf(cmd.OutOrStdout(),
					"%s is part of the control plane, which moves as a set (shared KTSP protocol version)\n", args[0])
				return updateInfra(cmd)
			}

			// A config-set named here is almost certainly a mistake worth catching:
			// it would install a new version into the store and leave every universe
			// that applied the old one untouched, while reporting success.
			if len(args) == 1 {
				if items, err := s.ListInstalled(cmd.Context()); err == nil {
					for _, it := range items {
						if it.Name == args[0] && it.Kind == string(mktapi.KindConfig) {
							return fmt.Errorf(
								"%s is a config-set: it is APPLIED to a universe, not updated in place.\n"+
									"Re-apply it to pick up a new version:  katyoti config-set apply %s --universe <name>",
								args[0], args[0])
						}
					}
				}
			}

			// Explicit single-item version pin/downgrade.
			if to != "" {
				if len(args) != 1 {
					return fmt.Errorf("--to <version> requires exactly one <item>")
				}
				inst, err := s.SetVersion(cmd.Context(), mktapi.SetVersionInput{Name: args[0], Version: to})
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "set %s -> %s\n", inst.Name, inst.Version)
				return nil
			}

			// A single named sidecar with no --to: move it to latest (+ repoint).
			//
			// Routed through Update, NOT straight to SetVersion. SetVersion
			// repoints only the universe it is GIVEN, and this passed none — so
			// "the current one", which for a central-universe CLI run is no
			// universe at all. `katyoti update <name>` therefore moved the store
			// and left every universe pinning the old version, while reporting
			// success and while use-cases.md promised it repointed.
			if len(args) == 1 {
				done, err := s.Update(cmd.Context(), mktapi.UpdateInput{Names: []string{args[0]}})
				if err != nil {
					return err
				}
				if len(done) == 0 {
					// Nothing outdated. Say so rather than implying a move.
					inst, ierr := s.SetVersion(cmd.Context(), mktapi.SetVersionInput{Name: args[0]})
					if ierr != nil {
						return ierr
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s is already at %s (latest)\n", inst.Name, inst.Version)
					return nil
				}
				printUpdates(cmd.OutOrStdout(), done)
				return nil
			}

			// Bare `update` means everything -- the reading a user expects.
			if !wantInfra && !wantSidecars {
				wantAll = true
			}
			touched := false
			if wantAll || wantInfra {
				if commit, dev := devControlPlane(cmd); dev && !wantInfra {
					// A control plane make dev placed follows the dev channel:
					// updating everything leaves it, only --infra replaces it
					//
					WriteDevControlPlaneLeft(cmd.OutOrStdout(), "infra: ", commit)
				} else if err := updateInfra(cmd); err != nil {
					// With --all, a channel that is unreachable must not block the
					// sidecar half: the user asked for everything they can get.
					if !wantAll {
						return err
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "infra: %v\n", err)
				}
				touched = true
			}
			if wantAll || wantSidecars {
				if err := updateSidecars(cmd, s, args); err != nil {
					return err
				}
				touched = true
			}
			if !touched {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing selected")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "pin the named item to this version (with --infra: roll the whole control-plane set)")
	cmd.Flags().BoolVar(&wantInfra, "infra", false, "update the control-plane set ("+strings.Join(controlplane.InfraBinaries, " + ")+")")
	cmd.Flags().BoolVar(&wantSidecars, "sidecars", false, "update installed sidecars only")
	cmd.Flags().BoolVar(&wantAll, "all", false, "update infra AND sidecars (the default when nothing is selected)")
	return cmd
}

// updateInfra brings the control-plane set to the channel version.
func updateInfra(cmd *cobra.Command) error {
	infra, err := resolveInfra(cmd)
	if err != nil {
		return err
	}
	_, dev := devControlPlane(cmd)
	done, err := infra.SelfUpdate(cmd.Context())
	if err != nil {
		return err
	}
	if len(done) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "infra: up to date")
		return nil
	}
	WriteInfraUpdates(cmd.OutOrStdout(), "infra: ", done, dev)
	return nil
}

// devControlPlane answers the commit of the installation's control plane when
// make dev placed it - a dev build of a checkout.
func devControlPlane(cmd *cobra.Command) (string, bool) {
	s, err := resolve(cmd)
	if err != nil {
		return "", false
	}
	return DevControlPlane(cmd.Context(), s)
}

// InfraVersionsReader is what DevControlPlane reads: the installation's
// control-plane versions (mktapi.SetupService answers it).
type InfraVersionsReader interface {
	InfraVersions(ctx context.Context) (mktapi.InfraVersionsOutput, error)
}

// DevControlPlane answers the commit of the installation's control plane when
// make dev placed it - a dev build of a checkout, which an update of
// everything leaves (bare update, --all, katyoti gui --update).
func DevControlPlane(ctx context.Context, s InfraVersionsReader) (string, bool) {
	v, err := s.InfraVersions(ctx)
	if err != nil || !v.DevBuild {
		return "", false
	}
	return v.Installed, true
}

// WriteDevControlPlaneLeft says an update of everything left the dev build
// make dev placed, and what replaces it.
func WriteDevControlPlaneLeft(w io.Writer, prefix, commit string) {
	fmt.Fprintf(w, "%sdev build %s (make dev) left as it is - `katyoti update --infra` replaces it with the infra channel's set\n", prefix, commit)
}

// WriteInfraUpdates says what an infra update moved, one line per program; a
// dev build it replaced is said as such.
func WriteInfraUpdates(w io.Writer, prefix string, done []mktapi.Update, replacedDev bool) {
	for _, u := range done {
		from := u.Installed
		switch {
		case replacedDev:
			from = "dev build " + from + " (make dev) replaced by"
		case from == "":
			from = "(absent) ->"
		default:
			from += " ->"
		}
		fmt.Fprintf(w, "%s%s %s %s\n", prefix, u.Name, from, u.Latest)
	}
}

// updateSidecars moves catalog-installed sidecars to latest.
func updateSidecars(cmd *cobra.Command, s mktapi.SetupService, names []string) error {
	done, err := s.Update(cmd.Context(), mktapi.UpdateInput{Names: names})
	if err != nil {
		return err
	}
	if len(done) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "sidecars: up to date")
		return nil
	}
	for _, u := range done {
		fmt.Fprintf(cmd.OutOrStdout(), "sidecars: %s %s -> %s\n", u.Name, u.Installed, u.Latest)
	}
	return nil
}

// NewSelfUpdateCommand builds `self-update` — bring the control-plane binaries
// (controlplane.InfraBinaries) to their latest channel version, swapping each
// running binary in place (rename-aside).
func NewSelfUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "self-update",
		Short: "Alias for `update --infra` (kept: the installer script and docs use it)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			infra, err := resolveInfra(cmd)
			if err != nil {
				return err
			}
			_, dev := devControlPlane(cmd)
			done, err := infra.SelfUpdate(cmd.Context())
			if err != nil {
				return err
			}
			if len(done) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "control plane is up to date")
				return nil
			}
			WriteInfraUpdates(cmd.OutOrStdout(), "updated ", done, dev)
			return nil
		},
	}
}

// NewInstallInfraCommand builds `install-infra` — bootstrap the control-plane
// set from the INFRA CHANNEL into the store + control-plane location.
//
// Not from a catalog: what to install is compiled in, so only where and which
// version are open. See the infra package.
func NewInstallInfraCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install-infra",
		Short: "Install the control-plane set (" + strings.Join(controlplane.InfraBinaries, " + ") + ") from the infra channel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			infra, err := resolveInfra(cmd)
			if err != nil {
				return err
			}
			out, err := infra.InstallInfra(cmd.Context())
			if err != nil {
				return err
			}
			for _, ins := range out.Installed {
				fmt.Fprintf(cmd.OutOrStdout(), "installed %s@%s → control-plane\n", ins.Name, ins.Version)
			}
			if len(out.Skipped) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "skipped (no subscribed catalog serves them): %s\n", strings.Join(out.Skipped, ", "))
			}
			if len(out.Installed) == 0 && len(out.Skipped) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no control-plane binaries to install")
			}
			return nil
		},
	}
}

// NewVersionsCommand builds `versions <item>` -- what you can move to.
//
// Infra and sidecars answer from different places, and the command hides that:
// a sidecar's versions come from its catalog, an infra binary's from the
// channel pointer. Routing by name means nobody has to know which is which to
// find out what `update --to` will accept.
func NewVersionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions <item>",
		Short: "List an item's available versions (newest first)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			if controlplane.IsInfra(args[0]) {
				return printInfraVersions(cmd, s)
			}
			vs, err := s.ItemVersions(cmd.Context(), mktapi.ItemVersionsInput{Name: args[0]})
			if err != nil {
				return err
			}
			for i, v := range vs.Versions {
				tag := ""
				if i == 0 {
					tag = "  (latest)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", v, tag)
			}
			return nil
		},
	}
	return cmd
}

// printInfraVersions shows the control-plane set's reachable versions.
//
// There are at most two, and that is the design rather than a limitation:
// nothing pins infra, so the channel retains only what a command can still
// resolve -- the current version and one rollback step. Labelling which is
// installed turns the list into an answer ("where am I, where can I go")
// instead of two bare strings.
func printInfraVersions(cmd *cobra.Command, s mktapi.SetupService) error {
	out, err := s.InfraVersions(cmd.Context())
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "control-plane set (%s) -- moves together\n", strings.Join(controlplane.InfraBinaries, " + "))
	fmt.Fprintf(w, "channel: %s\n", out.Channel)
	fmt.Fprintf(w, "  %s%s%s\n", out.Current, marker(out.Current, out.Installed), "  (current)")
	if out.Previous != "" {
		fmt.Fprintf(w, "  %s%s%s\n", out.Previous, marker(out.Previous, out.Installed), "  (rollback: update --infra --to "+out.Previous+")")
	}
	if out.Installed != "" && out.Installed != out.Current && out.Installed != out.Previous {
		// Running something the channel no longer carries is worth saying out
		// loud: there is no way back to it once it has been pruned.
		fmt.Fprintf(w, "  %s  <- installed, NO LONGER IN THE CHANNEL\n", out.Installed)
	}
	return nil
}

func marker(version, installed string) string {
	if version != "" && version == installed {
		return "  <- installed"
	}
	return ""
}

// NewRepinProgramsCommand builds `repin-programs` - point every universe of
// this installation at its own programs (make install runs it after copying).
func NewRepinProgramsCommand() *cobra.Command {
	var dry bool
	cmd := &cobra.Command{
		Use:   "repin-programs",
		Short: "Point every universe of this installation at its own programs (control-plane/, bin/dev/), never a checkout's bin/",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			infra, err := resolveInfra(cmd)
			if err != nil {
				return err
			}
			out, err := infra.RepinPrograms(cmd.Context(), mktapi.RepinProgramsInput{DryRun: dry})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			verb := "re-pinned"
			if dry {
				verb = "would re-pin"
			}
			for _, r := range out.Moved {
				fmt.Fprintf(w, "%s %s/%s: %s -> %s\n", verb, r.Universe, r.Member, r.From, r.To)
			}
			for _, r := range out.Missing {
				fmt.Fprintf(w, "kept %s/%s: %s (the installation has no such program)\n", r.Universe, r.Member, r.From)
			}
			if len(out.Moved) == 0 && len(out.Missing) == 0 {
				fmt.Fprintf(w, "every universe of %s runs its own programs\n", out.Installation)
			}
			if !dry {
				fmt.Fprintf(w, "registry: %d program(s) discovered in %s\n", len(out.Discovered), out.Installation)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dry, "dry-run", false, "Say what would move and change nothing")
	return cmd
}
