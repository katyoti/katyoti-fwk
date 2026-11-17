// SPDX-License-Identifier: Apache-2.0

// catalog.go provides the `catalog` Cobra command group exposed by the
// bus host (ktw-hub). The catalog reads cached descriptors for every
// sidecar listed in the active universe's Univers.yaml, without
// spawning any sidecar.
//
//	ktw-hub catalog list             # one-line summary per cached binary
//	ktw-hub catalog get <name>       # full Manifest JSON for a binary
//	ktw-hub catalog members          # all binary names per Univers.yaml
//	ktw-hub catalog reload           # re-read cache files from disk
//	ktw-hub catalog kinds [<kind>]   # every kind with its description and count, or one kind's entries
package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/spf13/cobra"
)

// NewCatalogCommand builds the `catalog` command group. The Catalog
// service must be published in the application — typically by the bus
// host's bootstrap (see app/ktw-hub/cmd/bootstrap.go).
func NewCatalogCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "catalog",
		Short: "Inspect the in-host sidecar catalog (cached manifests)",
		Long: `The catalog aggregates every sidecar listed in the sidecars: block of
this universe's Univers.yaml and serves their cached manifests (slot_types +
slots + resources) to consumers without spawning the sidecars themselves.

This command group is read-only. Re-probing a sidecar (running its binary's
'sc resources manifest' into the tier-1 cache under <CacheDir>/sidecars/) is
ktw-hub's 'catalog refresh <name>'.`,
	}

	root.AddCommand(newCatalogListCommand())
	root.AddCommand(newCatalogGetCommand())
	root.AddCommand(newCatalogMembersCommand())
	root.AddCommand(newCatalogReloadCommand())
	root.AddCommand(newCatalogKindsCommand(publishedCatalog))

	return root
}

func newCatalogListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List loaded sidecar manifests (JSON array)",
		Long:  `Emits one JSON object per loaded sidecar with provider, version, hash, and resource count.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := application.UseService[mbapi.Catalog](cmd.Context())
			if err != nil {
				return fmt.Errorf("catalog: service not published: %w", err)
			}
			ms, err := cat.List(cmd.Context())
			if err != nil {
				return err
			}
			type row struct {
				Provider      string `json:"provider"`
				Version       string `json:"version"`
				Hash          string `json:"hash,omitempty"`
				ResourceCount int    `json:"resource_count"`
				SlotCount     int    `json:"slot_count"`
			}
			rows := make([]row, 0, len(ms))
			for _, m := range ms {
				rows = append(rows, row{
					Provider:      m.Provider,
					Version:       m.Version,
					Hash:          m.Hash,
					ResourceCount: len(m.Contributions),
					SlotCount:     len(m.Slots),
				})
			}
			out, err := json.MarshalIndent(rows, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}

func newCatalogGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Print one sidecar's full cached Manifest as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := application.UseService[mbapi.Catalog](cmd.Context())
			if err != nil {
				return fmt.Errorf("catalog: service not published: %w", err)
			}
			m, err := cat.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}

func newCatalogMembersCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "members",
		Short: "List every sidecar name in this universe (per the Univers.yaml sidecars: block)",
		Long:  `Includes sidecars that have no cached manifest yet. Use 'catalog list' for the cached-only view.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := application.UseService[mbapi.Catalog](cmd.Context())
			if err != nil {
				return fmt.Errorf("catalog: service not published: %w", err)
			}
			names, err := cat.Names(cmd.Context())
			if err != nil {
				return err
			}
			out, err := json.MarshalIndent(names, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}

// CatalogReloader is the optional admin surface on the published Catalog
// implementation. The CLI's `reload` subcommand uses this to force a
// re-read from disk; production refresh (re-running a sidecar's
// resources list) lives elsewhere.
type CatalogReloader interface {
	Reload() error
}

func newCatalogReloadCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Re-read the tier-1 manifest cache from disk",
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := application.UseService[mbapi.Catalog](cmd.Context())
			if err != nil {
				return fmt.Errorf("catalog: service not published: %w", err)
			}
			r, ok := cat.(CatalogReloader)
			if !ok {
				return fmt.Errorf("catalog: published Catalog does not support reload")
			}
			if err := r.Reload(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	}
}

// publishedCatalog resolves the aggregate the hub publishes.
func publishedCatalog(ctx context.Context) (mbapi.CatalogReader, error) {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: service not published: %w", err)
	}
	return cat, nil
}

// newCatalogKindsCommand builds `catalog kinds [<kind>]`: the list of lists.
// `resolve` gives the aggregate read; the command group passes the published
// catalog.
func newCatalogKindsCommand(resolve func(context.Context) (mbapi.CatalogReader, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "kinds [<kind>]",
		Short: "List every declared kind with its description and entry count, or one kind's entries (JSON)",
		Long: `Without an argument: one JSON object per kind declared in this universe -
name, description and count, the count being the entries listing that kind
returns (a kind with no entry is listed at 0). With a kind: that kind's
entries - uri, name and description - read from the index alone.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := resolve(cmd.Context())
			if err != nil {
				return err
			}
			q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}
			var rows any
			if len(args) == 1 {
				rows, err = mbapi.ListKindEntries(cmd.Context(), q, args[0])
			} else {
				rows, err = mbapi.ListKinds(cmd.Context(), q)
			}
			if err != nil {
				return err
			}
			out, err := json.MarshalIndent(rows, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}
