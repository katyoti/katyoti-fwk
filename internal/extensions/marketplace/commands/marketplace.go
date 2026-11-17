// SPDX-License-Identifier: Apache-2.0

// Package commands provides the katyoti top-level CLI: the `marketplace`
// subscription group plus `search` and `info`. All resolve the in-process
// SetupService from the cobra context.
package commands

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
)

func resolve(cmd *cobra.Command) (mktapi.SetupService, error) {
	return application.UseService[mktapi.SetupService](cmd.Context())
}

// resolveInfra resolves the control-plane MUTATION surface.
//
// Separate from resolve() because InfraService is deliberately not published
// over KTSP: these verbs replace katyoti.exe, ktw-hub.exe and ktw-gui.exe, so
// running them inside a hub would mean a process replacing its own binary and
// then reporting a success that is true about the disk and false about itself.
// The launcher resolves it IN-PROCESS, where nothing being updated is running.
func resolveInfra(cmd *cobra.Command) (mktapi.InfraService, error) {
	return application.UseService[mktapi.InfraService](cmd.Context())
}

// NewMarketplaceCommand builds the `marketplace` subscription group.
func NewMarketplaceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Manage marketplace subscriptions (add/list/remove/enable/disable/refresh/show)",
		Long: `Subscribe to marketplaces (catalog URLs) that serve installable items.
The effective list is the beside-exe seed overlaid by your user overrides;
CRUD writes only the user overlay (the seed is read-only).`,
	}
	cmd.AddCommand(mpListCmd(), mpAddCmd(), mpRemoveCmd(), mpEnableCmd(), mpDisableCmd(), mpChannelCmd(), mpRefreshCmd(), mpShowCmd())
	return cmd
}

func mpListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List subscribed marketplaces (effective = seed ∪ user)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			mkts, err := s.ListMarketplaces(cmd.Context())
			if err != nil {
				return err
			}
			if len(mkts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no marketplaces subscribed (add one with `marketplace add <name> <url>`)")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tENABLED\tSOURCE\tTRUST\tURL")
			for _, m := range mkts {
				trust := m.Trust
				if trust == "" {
					trust = mktapi.TrustNone
				}
				fmt.Fprintf(w, "%s\t%v\t%s\t%s\t%s\n", m.Name, m.Enabled, m.Source, trust, m.URL)
			}
			return w.Flush()
		},
	}
}

func mpAddCmd() *cobra.Command {
	var pubKeyFile, trust string
	cmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add (or update) a marketplace URL",
		Long: `Add (or update) a marketplace subscription.

For a team/public marketplace, pin its publisher's public key so katyoti
verifies the catalog signature before every install (fail-closed):

  marketplace add team <url> --pubkey team.key.pub --trust pinned

--pubkey implies --trust pinned unless you pass --trust explicitly. A local
marketplace needs neither (installs are still sha256-gated).`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			pubKey := ""
			if pubKeyFile != "" {
				raw, err := os.ReadFile(pubKeyFile)
				if err != nil {
					return fmt.Errorf("read --pubkey %q: %w", pubKeyFile, err)
				}
				pk, err := signing.ParsePublicKey(raw)
				if err != nil {
					return fmt.Errorf("--pubkey %q: %w", pubKeyFile, err)
				}
				pubKey = signing.PublicKeyLine(pk) // store the canonical base64 line
				if trust == "" {
					trust = mktapi.TrustPinned // a pinned key implies pinned trust
				}
			}
			if _, err := s.AddMarketplace(cmd.Context(), mktapi.AddMarketplaceInput{Name: args[0], URL: args[1], PubKey: pubKey, Trust: trust}); err != nil {
				return err
			}
			note := ""
			if trust == mktapi.TrustPinned {
				note = " (trust: pinned — catalog signature will be verified)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added marketplace %q%s\n", args[0], note)
			return nil
		},
	}
	cmd.Flags().StringVar(&pubKeyFile, "pubkey", "", "path to the publisher's minisign public key to pin (trust root)")
	cmd.Flags().StringVar(&trust, "trust", "", "trust level: none | pinned (default: pinned when --pubkey is set, else none)")
	return cmd
}

func mpRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a marketplace (a seed entry is recorded as disabled)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			if _, err := s.RemoveMarketplace(cmd.Context(), mktapi.RemoveMarketplaceInput{Name: args[0]}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed marketplace %q\n", args[0])
			return nil
		},
	}
}

func mpEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a marketplace",
		Args:  cobra.ExactArgs(1),
		RunE:  setEnabledRun(true),
	}
}

func mpDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a marketplace",
		Args:  cobra.ExactArgs(1),
		RunE:  setEnabledRun(false),
	}
}

func mpChannelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "channel <name> <stable|canary>",
		Short: "Follow a marketplace's stable channel (end users) or its canary channel (alpha users: the release candidates too)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			if _, err := s.SetMarketplaceChannel(cmd.Context(), mktapi.SetMarketplaceChannelInput{Name: args[0], Channel: args[1]}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "marketplace %q: following its %s channel\n", args[0], args[1])
			return nil
		},
	}
}

func setEnabledRun(on bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		s, err := resolve(cmd)
		if err != nil {
			return err
		}
		if _, err := s.SetMarketplaceEnabled(cmd.Context(), mktapi.SetMarketplaceEnabledInput{Name: args[0], Enabled: on}); err != nil {
			return err
		}
		state := "disabled"
		if on {
			state = "enabled"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s marketplace %q\n", state, args[0])
		return nil
	}
}

func mpRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh [name]",
		Short: "Fetch + cache marketplace catalogs (all enabled if no name)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			cat, err := s.RefreshCatalog(cmd.Context(), mktapi.RefreshCatalogInput{Name: name})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "refreshed: %d item(s)\n", len(cat.Items))
			return nil
		},
	}
}

func mpShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a marketplace's catalog items",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			cat, err := s.RefreshCatalog(cmd.Context(), mktapi.RefreshCatalogInput{Name: args[0]})
			if err != nil {
				return err
			}
			return renderItems(cmd, cat.Items)
		},
	}
}

func renderItems(cmd *cobra.Command, items []mktapi.Item) error {
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no items")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tKIND\tLATEST\tDESCRIPTION")
	for _, it := range items {
		latest := ""
		if len(it.Versions) > 0 {
			latest = it.Versions[0].Version
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", it.Name, it.Kind, latest, it.Description)
	}
	return w.Flush()
}
