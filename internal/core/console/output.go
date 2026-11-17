// SPDX-License-Identifier: Apache-2.0

package console

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// OutputMode controls how CLI commands render their output.
type OutputMode string

const (
	OutputRich OutputMode = "rich" // styled with lipgloss (default)
	OutputRaw  OutputMode = "raw"  // plain text, no ANSI codes
	OutputJSON OutputMode = "json" // JSON
)

// OutputFlags holds the output mode and optional output file path.
type OutputFlags struct {
	Mode OutputMode
	File string // --of flag: when set, write output to this file (UTF-8) instead of stdout
}

// AddOutputFlag adds --out flag (json|raw|rich) and --of flag (output file) to a cobra command.
// Returns a pointer to the OutputFlags value.
//
// The default mode is resolved at run-time by walking the cobra parent
// chain for a SetGroupDefault annotation (per the command-surface decision
// §2.3). Resolution is deferred to PreRunE — at AddOutputFlag time the
// command may not yet be attached to its parent (typical pattern:
// child := buildCmd(); parent.AddCommand(child)), so cmd.Parent() can
// be nil. When no ancestor declares a default, falls back to OutputRich.
//
// The `llm` group calls SetGroupDefault(cmd, OutputJSON) so its entire
// subtree defaults to JSON without users needing `--out json`.
func AddOutputFlag(cmd *cobra.Command) *OutputMode {
	flags := &OutputFlags{Mode: OutputRich} // placeholder — real default resolved at PreRunE
	cmd.Flags().Var(&flags.Mode, "out", "Output mode: rich, raw, json (default depends on group)")
	cmd.Flags().StringVar(&flags.File, "of", "", "Output file path (writes UTF-8, bypasses shell encoding)")
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.PreRunE = chainPreRun(cmd.PreRunE, func(cmd *cobra.Command, args []string) error {
		// Resolve the group default LATE — by now the cobra tree is
		// fully wired and cmd.Parent() returns the real parent chain.
		// Only apply if the user did not pass --out explicitly.
		if !cmd.Flags().Changed("out") {
			flags.Mode = resolveDefaultOutputMode(cmd)
		}
		cmd.SetContext(withOutputFlags(cmd.Context(), flags))
		return nil
	})
	return &flags.Mode
}

// String implements pflag.Value.
func (m *OutputMode) String() string { return string(*m) }

// Set implements pflag.Value.
func (m *OutputMode) Set(v string) error {
	switch strings.ToLower(v) {
	case "rich", "":
		*m = OutputRich
	case "raw":
		*m = OutputRaw
	case "json":
		*m = OutputJSON
	default:
		return fmt.Errorf("invalid output mode %q (use: rich, raw, json)", v)
	}
	return nil
}

// Type implements pflag.Value.
func (m *OutputMode) Type() string { return "string" }

// IsJSON returns true when the mode is JSON.
func (m OutputMode) IsJSON() bool { return m == OutputJSON }

// IsRaw returns true when the mode is raw (plain text).
func (m OutputMode) IsRaw() bool { return m == OutputRaw }

// PrintJSON writes v as indented JSON to stdout.
func PrintJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// Printer adapts output helpers based on the current mode.
// In rich mode, lipgloss styling is used. In raw mode, plain text.
type Printer struct {
	Mode OutputMode
}

// NewPrinter creates a Printer for the given output mode.
func NewPrinter(mode OutputMode) *Printer {
	return &Printer{Mode: mode}
}

// Header prints a section title.
func (p *Printer) Header(title string) {
	fmt.Println()
	if p.Mode == OutputRaw {
		fmt.Println("  " + title)
	} else {
		fmt.Println(TitleStyle.Render("  " + title))
	}
	fmt.Println()
}

// Dim prints dimmed informational text.
func (p *Printer) Dim(msg string) {
	if p.Mode == OutputRaw {
		fmt.Println("  " + msg)
	} else {
		fmt.Println(DimStyle.Render("  " + msg))
	}
}

// Table creates a table that respects the output mode.
func (p *Printer) Table(headers ...string) *Table {
	t := NewTable(headers...)
	if p.Mode == OutputRaw {
		t.raw = true
	}
	return t
}

// chainPreRun chains a new PreRunE with an existing one.
func chainPreRun(existing func(*cobra.Command, []string) error, fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if existing != nil {
			if err := existing(cmd, args); err != nil {
				return err
			}
		}
		return fn(cmd, args)
	}
}
