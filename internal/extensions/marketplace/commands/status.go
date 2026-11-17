// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// NewStatusCommand builds `status` — the whole machine on one screen.
//
// Answering "where am I?" used to take five commands (`installed`, `outdated`,
// `versions`, `marketplace list`, `universe list`) and the knowledge that those
// five exist. That is a poor trade for the question a user asks most often, and
// it is the question every troubleshooting conversation opens with.
//
// It reads only. Nothing here installs, updates, or writes — so it is always
// safe to run first, which is exactly what makes it useful when something is
// wrong.
//
// Every section degrades independently: an unreachable channel must not hide the
// marketplaces, and a broken catalog must not hide the control plane. A status
// command that fails as a unit is useless precisely when it matters most.
//
// Universes are deliberately NOT here. They are the orchestrator's concern,
// reached over KTSP, so including them would make `status` need a running hub --
// and a status command that requires the hub cannot tell you the hub is down.
// `katyoti universe list` answers that question separately.
func NewStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Where this machine stands: control plane, marketplaces, what is behind, what is broken",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			ctx := cmd.Context()

			// ── Control plane ────────────────────────────────────────────
			fmt.Fprintln(w, "CONTROL PLANE")
			if v, err := s.InfraVersions(ctx); err != nil {
				fmt.Fprintf(w, "  unavailable: %v\n", err)
			} else {
				switch {
				case v.DevBuild:
					// make dev placed it from a checkout: the dev channel, which
					// make dev moves - never behind the infra channel.
					fmt.Fprintf(w, "  dev build %s, installed by make dev from a checkout (the dev channel)\n", v.Installed)
					if v.Channel != "" {
						fmt.Fprintf(w, "  infra channel: %s (%s)   `katyoti install-infra` replaces the dev build with it\n", v.Channel, v.Current)
					}
				case v.Installed == "":
					fmt.Fprintln(w, "  not installed — run `katyoti install-infra`")
				case v.Installed == v.Current:
					fmt.Fprintf(w, "  %s (current)\n", v.Installed)
				default:
					fmt.Fprintf(w, "  %s — %s available   `katyoti update --infra`\n", v.Installed, v.Current)
				}
				if !v.DevBuild {
					fmt.Fprintf(w, "  channel: %s\n", v.Channel)
				}
			}

			// ── Marketplaces ─────────────────────────────────────────────
			fmt.Fprintln(w, "\nMARKETPLACES")
			if ms, err := s.ListMarketplaces(ctx); err != nil {
				fmt.Fprintf(w, "  unavailable: %v\n", err)
			} else if len(ms) == 0 {
				fmt.Fprintln(w, "  none — `katyoti marketplace add <name> <url>`")
			} else {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				for _, m := range ms {
					state := "enabled"
					if !m.Enabled {
						state = "disabled"
					}
					trust := m.Trust
					if trust == "" {
						trust = "unpinned"
					}
					fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", m.Name, state, trust, m.URL)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
			}

			// ── Installed + behind ───────────────────────────────────────
			fmt.Fprintln(w, "\nINSTALLED")
			if items, err := s.ListInstalled(ctx); err != nil {
				fmt.Fprintf(w, "  unavailable: %v\n", err)
			} else {
				writeInstalledCounts(w, items)
			}
			if ups, err := s.Outdated(ctx); err == nil && len(ups) > 0 {
				names := make([]string, 0, len(ups))
				for _, u := range ups {
					names = append(names, u.Name)
				}
				fmt.Fprintf(w, "  %d behind: %s   `katyoti update --sidecars`\n",
					len(ups), strings.Join(names, ", "))
			}

			// ── What is broken ───────────────────────────────────────────
			fmt.Fprintln(w, "\nINSTALLATION")
			printDiagnosis(cmd, w)
			return nil
		},
	}
}

// writeInstalledCounts says what the installation holds, the control plane
// left to its own section: the dev builds of its dev channel (make dev; one
// each, whatever store copy is kept beside), then what a marketplace manages,
// then the binaries discovered. Discovered ones are counted apart because they
// are NOT managed: they cannot be updated or uninstalled, and one total would
// overstate what this machine controls.
func writeInstalledCounts(w io.Writer, items []mktapi.Installed) {
	dev, stored, managed, discovered := 0, 0, 0, 0
	for _, it := range items {
		switch {
		case it.ControlPlane:
		case it.DevBuild != nil:
			dev++
			if it.Source != "executable-dir" {
				stored++
			}
		case it.Source == "executable-dir":
			discovered++
		default:
			managed++
		}
	}
	if dev > 0 {
		fmt.Fprintf(w, "  %d dev builds of a checkout (make dev, the dev channel) - make dev updates them", dev)
		if stored > 0 {
			fmt.Fprintf(w, "; %d also kept in the store from a marketplace", stored)
		}
		fmt.Fprintln(w)
	}
	switch {
	case managed > 0 || dev == 0:
		fmt.Fprintf(w, "  %d managed", managed)
		if discovered > 0 {
			fmt.Fprintf(w, ", %d discovered (dev binaries, not managed)", discovered)
		}
		fmt.Fprintln(w)
	case discovered > 0:
		fmt.Fprintf(w, "  %d discovered (outside the dev channel, not managed)\n", discovered)
	}
}

// printDiagnosis names what is broken in the installation:
// a program gone or replaced against its checksum, a registry row to a
// vanished path, a row a test left. It reads the installation's own files,
// in-process - a broken installation is when the hub may not answer.
func printDiagnosis(cmd *cobra.Command, w io.Writer) {
	infra, err := resolveInfra(cmd)
	if err != nil {
		fmt.Fprintf(w, "  unavailable: %v\n", err)
		return
	}
	d, err := infra.DiagnoseInstallation(cmd.Context())
	if err != nil {
		fmt.Fprintf(w, "  unavailable: %v\n", err)
		return
	}
	writeDiagnosis(w, d)
}

// writeDiagnosis prints a diagnosis: sound, or each problem with its file.
func writeDiagnosis(w io.Writer, d mktapi.InstallationDiagnosis) {
	if len(d.Problems) == 0 {
		fmt.Fprintln(w, "  sound: programs match their checksums, every registry row names a path that is there")
		return
	}
	fmt.Fprintf(w, "  %d problem(s)   `katyoti doctor` repairs them after a backup\n", len(d.Problems))
	for _, p := range d.Problems {
		fmt.Fprintf(w, "  - %s: %s\n      in %s\n", p.Kind, p.Detail, p.File)
		if p.Path != "" && p.Path != p.File {
			fmt.Fprintf(w, "      path: %s\n", p.Path)
		}
	}
}
