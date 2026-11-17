// SPDX-License-Identifier: Apache-2.0

// Package commands provides the `schema` Cobra subcommand for ktw-agents
// (and any future binary that bundles the schema extension).
package commands

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/spf13/cobra"
)

// NewSchemaCommand builds the `schema` command tree:
//
//	schema parse <file> [--format yaml|json] [--quiet] [--strict]
//
// The verb resolves SchemaService from the application context (so the same
// command works in ktw-agents and in a future ktw-schema sidecar binary).
func NewSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Parse and validate KTW data-DSL schemas",
		Long: `Tools for the KTW data-DSL framework's parser+normalizer.

Reads YAML or JSON DSL schemas (with shorthand: string@email?, Item[], ->User,
~Category, inline unions), expands shorthand to long form, resolves imports
recursively with cycle detection, validates the AST, and emits a canonical,
language-neutral AST.

The canonical AST is the cross-language contract every downstream generator
(JSON Schema, Go structs, TS types, Pydantic) consumes.`,
	}
	cmd.AddCommand(newParseCommand())
	return cmd
}

func newParseCommand() *cobra.Command {
	var (
		format string
		quiet  bool
		strict bool
	)
	cmd := &cobra.Command{
		Use:   "parse <file>",
		Short: "Parse a YAML or JSON DSL schema and emit its canonical AST",
		Long: `Parses a schema file from disk and prints the canonical AST as JSON.

Format is inferred from the file extension (.yaml, .yml, .json) unless --format
is set explicitly. Use --quiet to suppress AST output (validation-only).
Use --strict to fail on warnings.

Examples:
  ktw-agents schema parse schema.yaml
  ktw-agents schema parse --format yaml schema.txt
  ktw-agents schema parse --quiet schema.yaml`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			svc, err := application.UseService[api.SchemaService](ctx)
			if err != nil {
				return fmt.Errorf("schema service: %w", err)
			}
			out, err := svc.ParseFile(ctx, schemaapi.ParseFileInput{
				Path:   args[0],
				Format: schemaapi.SchemaFormat(format),
				Strict: strict,
				Quiet:  quiet,
			})
			if err != nil {
				return err
			}
			for _, d := range out.Diagnostics {
				fmt.Fprintf(os.Stderr, "[%s] %s: %s (path: %s)\n",
					d.Severity, d.Code, d.Message, d.Path)
			}
			if quiet {
				return nil
			}
			raw, err := out.Schema.MarshalCanonicalJSONIndent("  ")
			if err != nil {
				return fmt.Errorf("marshal canonical JSON: %w", err)
			}
			cmd.OutOrStdout().Write(raw)
			cmd.OutOrStdout().Write([]byte("\n"))
			return nil
		},
	}
	cmd.Flags().StringVarP(&format, "format", "t", "", "Schema format: yaml or json (default: from extension)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress AST output (validation only)")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail on unknown fields and warnings")
	return cmd
}
