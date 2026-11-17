// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// NewSearchCommand builds `search <query>` — search items across enabled marketplaces.
func NewSearchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "search [query]",
		Short: "Search installable items across enabled marketplaces",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			q := ""
			if len(args) == 1 {
				q = args[0]
			}
			results, err := s.SearchItems(cmd.Context(), mktapi.SearchItemsInput{Query: q})
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no matching items")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tKIND\tLATEST\tMARKETPLACE\tDESCRIPTION")
			for _, r := range results {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Name, r.Kind, r.Latest, r.Marketplace, r.Description)
			}
			return w.Flush()
		},
	}
}

// NewInstalledCommand builds `installed` — the machine-wide available-items registry.
func NewInstalledCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "installed",
		Short: "List items available on this machine (the available-items registry)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			items, err := s.ListInstalled(cmd.Context())
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing installed")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tKIND\tVERSION\tSOURCE\tLOCATION")
			for _, it := range items {
				kind := it.Kind
				if kind == "" {
					kind = "sidecar"
				}
				loc := it.BinaryPath
				if it.FolderPath != "" {
					loc = it.FolderPath
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", it.Name, kind, it.Version, it.Source, loc)
			}
			return w.Flush()
		},
	}
}

// NewInfoCommand builds `info <item>` — show one item's detail.
func NewInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info <item>",
		Short: "Show details for one catalog item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			it, err := s.GetItem(cmd.Context(), mktapi.GetItemInput{Name: args[0]})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s (%s)\n", it.Name, it.Kind)
			if it.DisplayName != "" {
				fmt.Fprintf(out, "  %s\n", it.DisplayName)
			}
			if it.Description != "" {
				fmt.Fprintf(out, "  %s\n", it.Description)
			}
			if len(it.Tags) > 0 {
				fmt.Fprintf(out, "  tags: %v\n", it.Tags)
			}
			fmt.Fprintln(out, "  versions:")
			for _, v := range it.Versions {
				fmt.Fprintf(out, "    - %s (%d asset(s))\n", v.Version, len(v.Assets))
			}
			return nil
		},
	}
}
