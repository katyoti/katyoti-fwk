// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/spf13/cobra"
)

// newCacheCommand builds the `cache` subcommand group under the hub's
// `catalog` cobra root. Surface for inspecting + maintaining the
// per-sidecar manifest cache at `<CacheDir>/sidecars/` (post 2026-06-05
// redesign — see `cache-redesign-per-sidecar`). Read-only by default;
// `invalidate` and `gc` are explicit mutating verbs.
//
// All operations target the canonical cache root resolved from
// `application.Env().CacheDir()`. No flag knob to override — the
// universe-agnostic cache lives in exactly one place.
func newCacheCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "cache",
		Short: "Inspect + maintain the per-sidecar manifest cache",
		Long: `The hub maintains a per-sidecar manifest cache at
<CacheDir>/sidecars/<sidecar>@<key>/{metadata.yaml, manifest.json}, one
record per (member name, declared binary) — <key> is derived from the
binary's path. This subcommand surface lets operators inspect the cache
state without spawning sidecars, force-invalidate a single sidecar's
records to trigger a re-fetch on next refresh, and sweep stale state
(crashed .tmp dirs, orphaned entries for sidecars removed from Univers.yaml).

The cache is shared across the universes of this host: the same binary
produces the same manifest in any universe, so universes naming the same
binary share one record, while universes naming one member at two binaries
hold two. To re-probe this universe's view, run ` + "`catalog refresh <sidecar>`" + ` instead.`,
	}
	root.AddCommand(newCacheListCommand())
	root.AddCommand(newCacheShowCommand())
	root.AddCommand(newCacheInvalidateCommand())
	root.AddCommand(newCacheGCCommand())
	return root
}

// cacheEntry is the GUI-facing summary of one cached sidecar — what
// `cache list` emits per row. Slim by design (matches what's worth
// scanning in a terminal). For the full picture per sidecar, use
// `cache show <name>`.
type cacheEntry struct {
	Sidecar       string `json:"sidecar"`
	ExePath       string `json:"exe_path,omitempty"`
	ExeSize       int64  `json:"exe_size,omitempty"`
	ExeModTime    string `json:"exe_mtime,omitempty"`
	ManifestHash  string `json:"manifest_hash,omitempty"`
	IndexedAt     string `json:"indexed_at,omitempty"`
	Stale         bool   `json:"stale,omitempty"`
	StaleReason   string `json:"stale_reason,omitempty"`
	Contributions int    `json:"contributions,omitempty"`
}

func newCacheListCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:          "list",
		Short:        "List every cached record (one row per <CacheDir>/sidecars/<name>@<key>/)",
		Long:         "Walks the cache root and emits one summary row per record — a member name and the binary the record was taken from. The 'stale' column flags records this universe would re-probe: the binary is not the one this universe declares for the name, or its recorded exe stat no longer matches the live binary. Another universe's record of a same-named member at another binary shows as stale here and is never served here.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := openCache()
			records, err := cache.Records()
			if err != nil {
				return err
			}
			sort.SliceStable(records, func(i, j int) bool { return records[i].Name < records[j].Name })
			rows := make([]cacheEntry, 0, len(records))
			for _, r := range records {
				rows = append(rows, summarizeEntry(cache, r))
			}
			if asJSON {
				out, _ := json.MarshalIndent(rows, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(cache is empty)")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-22s  %10s  %-25s  %-5s  %s\n", "SIDECAR", "EXE_SIZE", "INDEXED_AT", "STALE", "BINARY")
			for _, r := range rows {
				flag := "no"
				if r.Stale {
					flag = "YES"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-22s  %10d  %-25s  %-5s  %s\n",
					r.Sidecar, r.ExeSize, r.IndexedAt, flag, r.ExePath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of the table")
	return cmd
}

func newCacheShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "show <sidecar>",
		Short:        "Show full metadata + manifest stats for one cached sidecar",
		Long:         "Prints the metadata.yaml fields, the manifest hash, slot count, and per-type Contribution counts for the record of the named sidecar taken from the binary THIS universe declares for it (Univers.yaml, else a ktw-* binary next to ktw-hub). Use to verify what the hub last fetched + when.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := openCache()
			want := sourceDigestsFor(args[0])
			if want.Binary == "" {
				return fmt.Errorf("no binary declared for %s in this universe — a record is addressed by name and declared binary", args[0])
			}
			manifest, meta, err := cache.Load(args[0], want.Binary)
			if err != nil {
				return fmt.Errorf("load %s: %w", args[0], err)
			}
			if manifest == nil {
				return fmt.Errorf("no cache entry for %s taken from %s — run `catalog refresh %s` to populate it",
					args[0], want.Binary, args[0])
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Sidecar:       %s\n", args[0])
			fmt.Fprintf(out, "Record dir:    %s\n", cache.RecordPath(args[0], want.Binary))
			if meta != nil {
				fmt.Fprintf(out, "Exe path:      %s\n", meta.ExePath)
				fmt.Fprintf(out, "Exe size:      %d\n", meta.ExeSize)
				fmt.Fprintf(out, "Exe mtime:     %s\n", meta.ExeModTime.Format("2006-01-02T15:04:05Z07:00"))
				fmt.Fprintf(out, "Manifest hash: %s\n", meta.ManifestHash)
				fmt.Fprintf(out, "Indexed at:    %s\n", meta.IndexedAt.Format("2006-01-02T15:04:05Z07:00"))
				stale, reason := cache.IsStale(meta, want)
				if stale {
					fmt.Fprintf(out, "Staleness:     STALE — %s\n", reason)
				} else {
					fmt.Fprintln(out, "Staleness:     fresh")
				}
			}
			fmt.Fprintf(out, "\nManifest:\n")
			fmt.Fprintf(out, "  Provider:      %s\n", manifest.Provider)
			fmt.Fprintf(out, "  Version:       %s\n", manifest.Version)
			fmt.Fprintf(out, "  Hash:          %s\n", manifest.Hash)
			fmt.Fprintf(out, "  Slot types:    %d\n", len(manifest.SlotTypes))
			fmt.Fprintf(out, "  Slots:         %d\n", len(manifest.Slots))
			fmt.Fprintf(out, "  Contributions: %d\n", len(manifest.Contributions))
			// Per-type Contribution counts — clusters by type so
			// operators can see at-a-glance the shape of what's cached.
			byType := make(map[string]int)
			for _, c := range manifest.Contributions {
				byType[c.Type]++
			}
			if len(byType) > 0 {
				types := make([]string, 0, len(byType))
				for t := range byType {
					types = append(types, t)
				}
				sort.Strings(types)
				fmt.Fprintln(out, "  By type:")
				for _, t := range types {
					fmt.Fprintf(out, "    %-20s  %d\n", t, byType[t])
				}
			}
			return nil
		},
	}
}

func newCacheInvalidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "invalidate <sidecar>",
		Short:        "Remove one sidecar's cache records (next refresh re-fetches)",
		Long:         "Deletes every <CacheDir>/sidecars/<sidecar>@<key>/ — the records of that name taken from any binary, metadata.yaml and manifest.json alike. The next `catalog refresh <sidecar>` re-spawns the binary and rebuilds the cache from scratch. Use when the cache is suspected corrupt or you want to force a re-probe regardless of stat freshness.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := openCache()
			if err := cache.Invalidate(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "invalidated %s — run `catalog refresh %s` to re-populate\n",
				args[0], args[0])
			return nil
		},
	}
}

func newCacheGCCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:          "gc",
		Short:        "Sweep stale .tmp dirs + orphaned entries (sidecars removed from Univers.yaml)",
		Long:         "Removes two classes of stale state from the cache root: '<sidecar>.tmp' leftovers from crashed prior Save calls (always removed), and sidecar folders for names no longer in Univers.yaml's 'sidecars:' block (orphan sweep). Use --dry-run to report what WOULD be removed without touching disk.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := openCache()
			// Build the allowlist from the current universe sidecars.
			reg, err := universeMembers()
			if err != nil {
				return fmt.Errorf("load Univers.yaml: %w", err)
			}
			// The catalog's membership, the hub's own record included.
			keep := memberNames(catalogMembers(reg.Sidecars))

			if dryRun {
				// Dry-run path: emulate the sweep by listing what would
				// be removed, without touching disk. Operators use this
				// before a real GC to verify the allowlist matches
				// expectations.
				names, err := cache.ListNames()
				if err != nil {
					return err
				}
				keepSet := make(map[string]bool, len(keep))
				for _, n := range keep {
					keepSet[n] = true
				}
				out := cmd.OutOrStdout()
				removed := 0
				for _, name := range names {
					if !keepSet[name] {
						fmt.Fprintf(out, "  would remove (orphan): %s\n", name)
						removed++
					}
				}
				// Tmp dirs aren't surfaced by ListNames (it filters
				// `.tmp` suffix); detect them via a direct readdir.
				tmps, _ := scanTmpDirs(cache.Root)
				for _, t := range tmps {
					fmt.Fprintf(out, "  would remove (.tmp):   %s\n", t)
					removed++
				}
				if removed == 0 {
					fmt.Fprintln(out, "(nothing to remove)")
				}
				return nil
			}

			result, err := cache.GC(keep)
			out := cmd.OutOrStdout()
			for _, t := range result.TmpDirsRemoved {
				fmt.Fprintf(out, "removed (.tmp):   %s\n", t)
			}
			for _, o := range result.OrphansRemoved {
				fmt.Fprintf(out, "removed (orphan): %s\n", o)
			}
			if len(result.TmpDirsRemoved)+len(result.OrphansRemoved) == 0 {
				fmt.Fprintln(out, "(nothing to remove)")
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would be removed without touching disk")
	return cmd
}

// openCache returns a *manifestlocal.Cache rooted at the hub's canonical
// cache path. Centralized so every subcommand resolves the same way.
func openCache() *manifestlocal.Cache {
	root := filepath.Join(application.Env().CacheDir(), "sidecars")
	return universeCache(root)
}

// summarizeEntry projects one loaded record into the cacheEntry row shape.
// Failures collapse into a row with empty fields — the record still surfaces
// in `list` so the operator can see it exists even if it's unreadable.
// Staleness is judged for THIS universe: a record of a binary this universe
// does not declare for the name is stale here, with the reason saying so.
func summarizeEntry(cache *manifestlocal.Cache, r manifestlocal.Record) cacheEntry {
	entry := cacheEntry{Sidecar: r.Name}
	if r.Err != nil {
		entry.Stale = true
		entry.StaleReason = r.Err.Error()
		return entry
	}
	if r.Meta != nil {
		entry.ExePath = r.Meta.ExePath
		entry.ExeSize = r.Meta.ExeSize
		entry.ExeModTime = r.Meta.ExeModTime.Format("2006-01-02T15:04:05Z")
		entry.ManifestHash = r.Meta.ManifestHash
		entry.IndexedAt = r.Meta.IndexedAt.Format("2006-01-02T15:04:05Z")
		want := sourceDigestsFor(r.Name)
		if want.Binary == "" {
			entry.Stale = true
			entry.StaleReason = "not a member of this universe (no declared binary)"
		} else {
			entry.Stale, entry.StaleReason = cache.IsStale(r.Meta, want)
		}
	}
	if r.Manifest != nil {
		entry.Contributions = len(r.Manifest.Contributions)
	}
	return entry
}

// scanTmpDirs lists `<root>/<name>.tmp` directories (the abandoned-Save
// leftovers Cache.GC removes). Used only by the dry-run path — the
// real GC path uses Cache.GC directly.
func scanTmpDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) > 4 && name[len(name)-4:] == ".tmp" {
			out = append(out, name)
		}
	}
	return out, nil
}
