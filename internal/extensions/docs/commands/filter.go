// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	docscore "github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/diagram"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/pandoc"
	"github.com/spf13/cobra"
)

// NewFilterCommand builds the Pandoc JSON filter command.
func NewFilterCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "filter",
		Short: "Pandoc JSON filter — renders diagram code blocks via Kroki",
		Long: `Reads Pandoc JSON AST from stdin, replaces diagram code blocks
(mermaid, plantuml, graphviz, etc.) with rendered images via Kroki API,
and writes the modified AST to stdout.

Usage with Pandoc:
  pandoc input.md --filter "ktw-docs filter" -o output.pdf`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := application.UseConfig[docscore.DocServiceConfig](cmd.Context())
			if err != nil {
				return err
			}

			// Pandoc passes the output format as the first argument.
			outputFormat := ""
			if len(args) > 0 {
				outputFormat = args[0]
			}

			filter := &pandoc.PandocFilter{
				Kroki:     diagram.NewKrokiRenderer(settings.KrokiURL),
				OutputDir: ".kroki-images",
				Format:    outputFormat,
				ImgFormat: "svg",
			}

			if err := filter.RunFilter(os.Stdin, os.Stdout); err != nil {
				return fmt.Errorf("filter: %w", err)
			}
			return nil
		},
	}
}
