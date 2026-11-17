// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// NewConfigSetCommand builds `config-set list | show <name>`.
//
// Named `config-set` (not `config`) because `katyoti config` is already taken by
// katyoti's OWN startup config (launch_mode / default_universe) — two different
// concepts that would otherwise collide on one verb.
func NewConfigSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config-set",
		Short: "Browse config-sets (templates + the keys they need) offered by your marketplaces",
	}
	cmd.AddCommand(newConfigSetListCommand(), newConfigSetShowCommand(), NewConfigSetApplyCommand())
	return cmd
}

func newConfigSetListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List config-sets across enabled marketplaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			sets, err := s.ListConfigSets(cmd.Context())
			if err != nil {
				return err
			}
			if len(sets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no config-sets offered by the enabled marketplaces")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tLATEST\tMARKETPLACE\tDESCRIPTION")
			for _, c := range sets {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Name, c.Latest, c.Marketplace, c.Description)
			}
			return w.Flush()
		},
	}
}

func newConfigSetShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a config-set's versions and the keys it needs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			out, err := s.ShowConfigSet(cmd.Context(), mktapi.ShowConfigSetInput{Name: args[0]})
			if err != nil {
				return err
			}
			o := cmd.OutOrStdout()
			fmt.Fprintf(o, "%s", out.Name)
			if out.DisplayName != "" && out.DisplayName != out.Name {
				fmt.Fprintf(o, " — %s", out.DisplayName)
			}
			fmt.Fprintln(o)
			if out.Description != "" {
				fmt.Fprintf(o, "  %s\n", out.Description)
			}
			if out.Marketplace != "" {
				fmt.Fprintf(o, "  marketplace: %s\n", out.Marketplace)
			}
			if len(out.Versions) > 0 {
				fmt.Fprintf(o, "  versions:    %v  (newest first)\n", out.Versions)
			}

			if !out.Installed {
				// The keys live INSIDE the bundle, so they are unreadable until an
				// install unpacks it. Say so rather than print an empty key list,
				// which would read as "this set needs nothing".
				fmt.Fprintf(o, "\n  not installed — its keys ship inside the bundle.\n")
				fmt.Fprintf(o, "  run `katyoti install %s` to see what it needs.\n", out.Name)
				return nil
			}
			fmt.Fprintf(o, "  installed:   %s\n", out.Version)
			if len(out.Keys) == 0 {
				fmt.Fprintln(o, "\n  declares no keys.")
				return nil
			}
			fmt.Fprintln(o, "\nRequired keys:")
			w := tabwriter.NewWriter(o, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "  NAME\tSCOPE\tSECRET\tDESCRIPTION")
			for _, k := range out.Keys {
				secret := ""
				if k.Secret {
					secret = "yes"
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", k.Name, k.Scope, secret, k.Description)
			}
			return w.Flush()
		},
	}
}
