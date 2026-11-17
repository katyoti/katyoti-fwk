// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// NewConfigureCommand builds `configure <item>` — drives the installed
// sidecar's own config surface (materialize config; report what else is needed).
func NewConfigureCommand() *cobra.Command {
	var universe string
	cmd := &cobra.Command{
		Use:   "configure <item>",
		Short: "Configure an installed sidecar via its own config surface",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			rep, err := s.ConfigureItem(cmd.Context(), mktapi.ConfigureItemInput{Name: args[0], UniverseHome: universe})
			if err != nil {
				return err
			}
			renderReport(cmd, args[0], rep)
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "Universe folder to configure in (default: the current workspace)")
	return cmd
}

// NewDoctorCommand builds `doctor`: it repairs what `status` names broken in
// the installation, every file it changes backed up first, and says what it
// did; then the config check across installed items.
func NewDoctorCommand() *cobra.Command {
	var universe string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Repair the installation (after a backup), then check the installed sidecars' config in a universe (default: the default one)",
		Long: `doctor repairs what katyoti status names broken in this installation: a
registry row to a vanished path or left by a test is removed, a control-plane
program replaced or gone is put back from its verified store copy. Every file
it changes is copied first into <root>/backups/doctor-<time> (manifest.yaml
says where each copy came from). What it cannot repair is left, with what to
do. --dry-run says what it would do and changes nothing. Then it checks the
installed sidecars' config in a universe.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "INSTALLATION")
			if infra, err := resolveInfra(cmd); err != nil {
				fmt.Fprintf(out, "  unavailable: %v\n", err)
			} else if rep, err := infra.RepairInstallation(cmd.Context(), mktapi.RepairInstallationInput{DryRun: dryRun}); err != nil {
				writeRepair(out, rep)
				return err
			} else {
				writeRepair(out, rep)
			}
			if dryRun {
				return nil
			}
			fmt.Fprintln(out, "\nCONFIG")
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			if universe == "" {
				universe = defaultUniverse()
			}
			checks, err := s.Doctor(cmd.Context(), mktapi.DoctorInput{UniverseHome: universe})
			if err != nil {
				return err
			}
			if len(checks) == 0 {
				fmt.Fprintln(out, "no installed sidecars to check")
				return nil
			}
			for _, c := range checks {
				renderReport(cmd, c.Name, c.Report)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "Universe to check in: a registered name or its folder (default: the default universe)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Say what would be repaired in the installation and change nothing (no config check)")
	return cmd
}

// writeRepair says what the doctor did to the installation: each problem,
// repaired or left with what to do, and where the backup is.
func writeRepair(w io.Writer, rep mktapi.InstallationRepair) {
	if len(rep.Actions) == 0 {
		fmt.Fprintln(w, "  sound: nothing to repair")
		return
	}
	done := 0
	for _, a := range rep.Actions {
		mark := "left"
		if a.Done {
			done++
			mark = "repaired"
			if rep.DryRun {
				mark = "would repair"
			}
		}
		fmt.Fprintf(w, "  - %s %s: %s\n      %s\n", mark, a.Problem.Kind, a.Problem.Detail, a.Did)
	}
	switch {
	case rep.DryRun:
		fmt.Fprintf(w, "  dry run: %d of %d would be repaired, nothing changed\n", done, len(rep.Actions))
	default:
		fmt.Fprintf(w, "  %d of %d repaired\n", done, len(rep.Actions))
	}
	if rep.Backup != "" {
		fmt.Fprintf(w, "  backup of every file changed: %s\n", rep.Backup)
	}
}

// defaultUniverse is the universe katyoti opens on: the installation's
// default_universe (katyoti.yml settings), "" outside an installation (the
// checks then read the process's own workspace).
func defaultUniverse() string {
	if inst := cenv.CurrentInstallation(); inst != nil {
		if u, ok := inst.Settings["default_universe"].(string); ok {
			return u
		}
	}
	return ""
}

func renderReport(cmd *cobra.Command, name string, rep sidecarapi.ConfigCheckReport) {
	out := cmd.OutOrStdout()
	state := "healthy"
	if !rep.Healthy {
		state = "needs attention"
	}
	fmt.Fprintf(out, "%s — %s (%s)\n", name, state, rep.Summary)
	var pending []sidecarapi.CheckItem
	for _, it := range rep.Items {
		if it.Status != "ok" {
			pending = append(pending, it)
		}
	}
	if len(pending) == 0 {
		return
	}
	w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "  CATEGORY\tNAME\tSTATUS\tNEXT STEP")
	for _, it := range pending {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", it.Category, it.Name, it.Status, it.Hint)
	}
	_ = w.Flush()
}
