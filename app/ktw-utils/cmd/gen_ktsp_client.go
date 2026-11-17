// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/tsgen"
	"github.com/spf13/cobra"
)

func init() {
	var (
		outDir    string
		provider  string
		slotTypes []string
	)
	cmd := &cobra.Command{
		Use:   "gen-ktsp-client <descriptor.json>",
		Short: "Generate a typed TypeScript KTSP client from a sidecar descriptor",
		Long: `Generate a typed TypeScript client from a KTSP descriptor.

The GUI reaches services through ONE generic dispatcher; the typing lives in a
generated TS client. Because the client is generated from the same descriptor the
wire is built from, it cannot drift — publishing a service method makes it typed
and callable with no Go change in ktw-gui.

The descriptor is a FILE, not a live process, so generation is deterministic and
reviewable:

  ktw-univers sc ktsp describe --out json > univers.json
  ktw-utils gen-ktsp-client univers.json --out app/ktw-gui/frontend/src/ktsp \
      --slot-type marketplace --slot-type orchestrator

Writes one <slot-type>.ts per slot type. Check the output in.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read descriptor: %w", err)
			}
			mods, err := tsgen.Generate(data, tsgen.Options{Provider: provider, SlotTypes: slotTypes})
			if err != nil {
				return err
			}
			if outDir == "" {
				// No --out: print to stdout so the tool is usable in a pipe and its
				// output inspectable without touching the tree.
				names := make([]string, 0, len(mods))
				for n := range mods {
					names = append(names, n)
				}
				sort.Strings(names)
				for _, n := range names {
					fmt.Fprint(cmd.OutOrStdout(), mods[n])
				}
				return nil
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			written := make([]string, 0, len(mods))
			for slot, src := range mods {
				path := filepath.Join(outDir, slot+".ts")
				if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
					return fmt.Errorf("write %s: %w", path, err)
				}
				written = append(written, path)
			}
			sort.Strings(written)
			fmt.Fprintf(cmd.OutOrStdout(), "generated %d client(s):\n  %s\n",
				len(written), strings.Join(written, "\n  "))
			return nil
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "Directory to write <slot-type>.ts into (default: stdout)")
	cmd.Flags().StringVar(&provider, "provider", "", "Override the wire provider prefix (default: the descriptor's own name)")
	cmd.Flags().StringArrayVar(&slotTypes, "slot-type", nil, "Generate only these slot types (repeatable; default: all)")
	rootCmd.AddCommand(cmd)
}
