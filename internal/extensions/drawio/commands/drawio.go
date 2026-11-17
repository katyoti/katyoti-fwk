// SPDX-License-Identifier: Apache-2.0

// Package commands holds the drawio engine's CLI: `drawio render`,
// `drawio validate`, `drawio lint`, `drawio read`, `drawio score`,
// `drawio check` and `drawio export`.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiorender "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/render"
	"github.com/spf13/cobra"
)

// NewDrawioCommand builds the `drawio` command group.
func NewDrawioCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drawio",
		Short: "Render, check and export draw.io diagrams",
		Long: `The drawio engine. A diagram is described by a LogicalDiagram JSON file
(typed nodes, edges and zones plus the viewpoint URI). Produce one from a
connector sidecar's walk (for example 'ktw-example to-diagram'), or write it by hand. An existing
.drawio is read, scored and laid out again with read, score and layout.`,
	}
	cmd.AddCommand(newRenderCommand(), newValidateCommand(), newLintCommand(), newReadCommand(), newScoreCommand(), newLayoutCheckCommand(), newLayoutCommand(), newExportCommand(), newViewpointsCommand(), newNotationsCommand())
	return cmd
}

func newRenderCommand() *cobra.Command {
	var (
		outputPath string
		noLayout   bool
		previous   string
	)
	cmd := &cobra.Command{
		Use:   "render <logical-diagram.json>",
		Short: "Render a LogicalDiagram to an editable .drawio file",
		Long: `Validates and lints the diagram, resolves its viewpoint from the
catalog, lays it out with ELK and writes the drawio XML. Validation errors
and lint warnings go to stderr; an error that blocks rendering fails the
command.

With --previous, the diagram is drawn over the last version: nothing it
shows moves, its locks stay, and new nodes are placed in their zone or
beside a neighbour (no layout runs; 'drawio layout' tidies the drawing). Build the model from 'drawio read --model' of
that file to keep a person's edits.

Examples:
  ktw-docs drawio render logical.json -o landscape.drawio
  ktw-docs drawio render logical.json --no-layout
  ktw-docs drawio render logical-v2.json --previous landscape.drawio -o landscape.drawio`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ld, err := readLogical(args[0])
			if err != nil {
				return err
			}
			var result drawiorender.EmitResult
			if previous != "" {
				prev, rerr := os.ReadFile(previous)
				if rerr != nil {
					return fmt.Errorf("read %q: %w", previous, rerr)
				}
				result, err = drawiorender.Refine(cmd.Context(), ld, prev)
			} else {
				result, err = drawiorender.Emit(cmd.Context(), ld, noLayout)
			}
			if err != nil {
				return fmt.Errorf("render: %w", err)
			}
			for _, e := range result.Errors {
				fmt.Fprintln(cmd.ErrOrStderr(), "error:", e)
			}
			for _, w := range result.Warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
			}
			if result.XML == "" {
				return fmt.Errorf("nothing rendered: %d error(s)", len(result.Errors))
			}
			if outputPath == "" {
				outputPath = strings.TrimSuffix(args[0], filepath.Ext(args[0])) + ".drawio"
			}
			if err := os.WriteFile(outputPath, []byte(result.XML), 0o644); err != nil {
				return fmt.Errorf("write %q: %w", outputPath, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d bytes, %d error(s), %d warning(s))\n",
				outputPath, len(result.XML), len(result.Errors), len(result.Warnings))
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: <input>.drawio)")
	cmd.Flags().BoolVar(&noLayout, "no-layout", false, "Skip ELK auto-layout")
	cmd.Flags().StringVar(&previous, "previous", "", "The last version's .drawio: draw over it, keeping places and locks")
	return cmd
}

func newValidateCommand() *cobra.Command {
	return newCheckCommand("validate", "Check a LogicalDiagram against its stencil set",
		`Reports what blocks or corrupts a render: an unknown stencil set, an
unknown node or edge kind, a duplicate id, an edge or zone member that
points at no node. Exits 1 when there is an error.`,
		func(ctx context.Context, ld *drawioapi.LogicalDiagram) (*drawiorender.ValidationResult, error) {
			return drawiorender.Validate(ctx, ld)
		})
}

func newLintCommand() *cobra.Command {
	return newCheckCommand("lint", "Report the soft issues of a LogicalDiagram",
		`Reports what renders but reads badly: an orphan node, an empty zone, a
label over 80 characters, an unknown layout preset. Never fails on a
warning; exits 1 only when the diagram cannot be read.`,
		func(ctx context.Context, ld *drawioapi.LogicalDiagram) (*drawiorender.ValidationResult, error) {
			return drawiorender.Lint(ctx, ld)
		})
}

// newCheckCommand builds validate and lint, which differ only by the check.
func newCheckCommand(use, short, long string, check func(context.Context, *drawioapi.LogicalDiagram) (*drawiorender.ValidationResult, error)) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:          use + " <logical-diagram.json>",
		Short:        short,
		Long:         long,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ld, err := readLogical(args[0])
			if err != nil {
				return err
			}
			r, err := check(cmd.Context(), ld)
			if err != nil {
				return fmt.Errorf("%s: %w", use, err)
			}
			if err := printIssues(cmd.OutOrStdout(), r, asJSON); err != nil {
				return err
			}
			if len(r.Errors) > 0 {
				return fmt.Errorf("%d error(s)", len(r.Errors))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the findings as JSON")
	return cmd
}

func newReadCommand() *cobra.Command {
	var modelOnly bool
	cmd := &cobra.Command{
		Use:   "read <diagram.drawio>",
		Short: "Read a .drawio file back into its LogicalDiagram",
		Long: `Prints, as JSON, the model a .drawio file draws: the LogicalDiagram,
each node's and zone's box, the edges' bend points, the ids a person
pinned, and a report of what could not be carried over. A diagram this
engine wrote reads back into the model it came from, with the changes a
person made in draw.io (Edit Data values, boxes added by hand); a foreign
diagram reads into nodes, edges and zones of kind "unknown".

Examples:
  ktw-docs drawio read landscape.drawio
  ktw-docs drawio read landscape.drawio --model > logical.json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read %q: %w", args[0], err)
			}
			res, err := drawiorender.Read(raw)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if modelOnly {
				return enc.Encode(res.Diagram)
			}
			return enc.Encode(res)
		},
	}
	cmd.Flags().BoolVar(&modelOnly, "model", false, "Print only the LogicalDiagram (ready for drawio render)")
	return cmd
}

func newLayoutCommand() *cobra.Command {
	var (
		out       string
		preset    string
		direction string
	)
	cmd := &cobra.Command{
		Use:   "layout <diagram.drawio>",
		Short: "Lay out an existing .drawio again",
		Long: `Lays out a .drawio file — generated, edited in draw.io or drawn by hand —
with ELK and writes it back with only its geometry changed: every label,
style and property stays as it was. Groups (a box with boxes inside, by
parent or drawn around them) are laid out as nested containers and fit
their content; cells a person pinned (ktw-pinned="1" in Edit Data, or
draw.io's Lock) keep their place; the drawing's current order is kept, so
laying out an unchanged drawing again moves little. Prints the quality
report of the result on stderr. Needs bun.

The layout options come from --preset / --direction, then the file's own
layout hints, then its viewpoint's profile.

Examples:
  ktw-docs drawio layout sketch.drawio -o sketch.drawio
  ktw-docs drawio layout landscape.drawio --preset archi-layered -o out.drawio`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read %q: %w", args[0], err)
			}
			var hints *drawioapi.LayoutHints
			if preset != "" || direction != "" {
				hints = &drawioapi.LayoutHints{Preset: preset, Direction: strings.ToUpper(direction)}
			}
			res, err := drawiorender.Relayout(cmd.Context(), raw, hints)
			if err != nil {
				return err
			}
			if out == "" {
				fmt.Fprint(cmd.OutOrStdout(), res.XML)
			} else if err := os.WriteFile(out, []byte(res.XML), 0o644); err != nil {
				return fmt.Errorf("write %q: %w", out, err)
			}
			stderr := cmd.ErrOrStderr()
			fmt.Fprintf(stderr, "laid out %d cell(s)\n", res.Moved)
			for _, line := range res.Report {
				fmt.Fprintln(stderr, "  "+line)
			}
			if q := res.Quality; q != nil {
				fmt.Fprintf(stderr, "quality: %d overlaps, %d crossings, %d edges through nodes, %d bends, %.0fx%.0f\n",
					q.Overlaps, q.Crossings, q.EdgesThroughNodes, q.Bends, q.Width, q.Height)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "Write the laid-out .drawio here (default stdout)")
	cmd.Flags().StringVar(&preset, "preset", "", "Layout preset of the host (archi-layered, layered-right, layered-down, force, ...)")
	cmd.Flags().StringVar(&direction, "direction", "", "Reading direction: RIGHT, DOWN, LEFT or UP")
	return cmd
}

func newViewpointsCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "viewpoints",
		Short: "List the viewpoints — the types of drawing a diagram can name",
		Long: `Lists every viewpoint the catalog holds (this sidecar's and, under the hub,
every other sidecar's): its URI, name, notation and layout profile. A
diagram names one in its "viewpoint" field.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			vps, err := drawioapi.ListViewpoints(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), vps)
			}
			w := cmd.OutOrStdout()
			for _, v := range vps {
				layout := ""
				if l := v.Body.Layout; l != nil {
					layout = strings.TrimSpace(l.Preset + " " + l.Algorithm + " " + l.Direction)
				}
				fmt.Fprintf(w, "%s\n  %s — %s\n  notation %s, layout %s\n", v.URI, v.Body.DisplayName, v.Body.Description, v.Body.Notation, layout)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the viewpoints as JSON")
	return cmd
}

func newNotationsCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "notations [<notation URI>]",
		Short: "List the notations, or the kinds of one",
		Long: `Without an argument, lists every notation the catalog holds. With a
notation URI, prints its element kinds (a node's kind) and relationship
kinds (an edge's kind), and the relationships it permits when it declares
rules.

Examples:
  ktw-docs drawio notations
  ktw-docs drawio notations ktw-docs:/notation/archimate3`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) == 1 {
				n, err := drawioapi.ResolveNotation(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if asJSON {
					return writeJSON(w, n)
				}
				fmt.Fprintf(w, "%s — %s\n\nelement kinds:\n", n.DisplayName, n.Description)
				for _, k := range sortedKeys(n.Elements) {
					e := n.Elements[k]
					fmt.Fprintf(w, "  %-28s %s", k, e.Label)
					if e.Category != "" {
						fmt.Fprintf(w, " [%s]", e.Category)
					}
					fmt.Fprintln(w)
				}
				fmt.Fprintln(w, "\nrelationship kinds:")
				for _, k := range sortedKeys(n.Relationships) {
					fmt.Fprintf(w, "  %-28s %s\n", k, n.Relationships[k].Label)
				}
				if n.Rules != nil {
					fmt.Fprintln(w, "\nthe notation declares which relationships join which kinds: validate reports the others")
				}
				return nil
			}
			ns, err := drawioapi.ListNotations(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(w, ns)
			}
			for _, n := range ns {
				fmt.Fprintf(w, "%s\n  %s — %d element kinds, %d relationship kinds\n", n.URI, n.Body.DisplayName, len(n.Body.Elements), len(n.Body.Relationships))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print as JSON")
	return cmd
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func newScoreCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "score <diagram.drawio>",
		Short: "Measure how a .drawio diagram reads",
		Long: `Prints, as JSON, the quality report of a .drawio file: node overlaps
(must be 0), edge crossings, edges through nodes, bends, nodes outside
their zone, extent and density. Works on any diagram, generated or drawn
by hand. A file of several pages prints one report per page, each named.`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read %q: %w", args[0], err)
			}
			pages, err := drawiorender.ScorePages(raw)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if len(pages) == 1 {
				return enc.Encode(pages[0].Quality)
			}
			return enc.Encode(pages)
		},
	}
}

// checkFileJSON is the --json shape of one file `drawio check` read.
type checkFileJSON struct {
	File     string              `json:"file"`
	Error    string              `json:"error,omitempty"`
	Findings []drawioapi.Finding `json:"findings"`
	Counts   map[string]int      `json:"counts"`
	Gated    int                 `json:"gated"`
}

func newLayoutCheckCommand() *cobra.Command {
	var asJSON, strict bool
	cmd := &cobra.Command{
		Use:   "check <diagram.drawio>...",
		Short: "Check a .drawio drawing for overlapping text and covered boxes",
		Long: `Reads each drawing as draw.io paints it (each label laid out from its
style and the Helvetica glyph widths) and lists:

  text-overlap   two labels painted over each other (fails the check)
  duplicate      the same text painted twice in the same place
  text-overflow  a label painted out of its own shape
  box-cover      filled shapes covering each other, or a label hidden
                 under a later shape
  not-measured   a label cell on an edge (not placed by the check)

Two boxes meet only when they intersect by more than 4 px both ways. Not
counted: a label inside its own shape, a text cell over a shape, a shape
inside another, hidden cells. Exits 1 on a text-overlap, on any finding but
not-measured with --strict, or when a file cannot be read.

Examples:
  ktw-docs drawio check roadmap.drawio
  ktw-docs drawio check docs/*.drawio --json`,
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var results []checkFileJSON
			failing := 0
			for _, path := range args {
				res := checkFileJSON{File: path, Findings: []drawioapi.Finding{}, Counts: map[string]int{}}
				raw, err := os.ReadFile(path)
				if err == nil {
					res.Findings, err = drawiorender.CheckLayout(raw)
				}
				if err != nil {
					res.Error = err.Error()
					failing++
					results = append(results, res)
					continue
				}
				if res.Findings == nil {
					res.Findings = []drawioapi.Finding{}
				}
				fails := false
				for _, f := range res.Findings {
					res.Counts[f.Kind]++
					if f.Gated {
						res.Gated++
					}
					if f.Gated || (strict && f.Kind != drawioapi.FindingNotMeasured) {
						fails = true
					}
				}
				if fails {
					failing++
				}
				results = append(results, res)
			}
			w := cmd.OutOrStdout()
			if asJSON {
				if err := writeJSON(w, results); err != nil {
					return err
				}
			} else {
				for _, res := range results {
					printLayoutCheck(w, res)
				}
			}
			if failing > 0 {
				return fmt.Errorf("%d of %d drawing(s) fail the check", failing, len(args))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the findings and counts of each file as JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail on any finding, not only on overlapping text")
	return cmd
}

func printLayoutCheck(w io.Writer, res checkFileJSON) {
	if res.Error != "" {
		fmt.Fprintf(w, "%s: error: %s\n", res.File, res.Error)
		return
	}
	if len(res.Findings) == 0 {
		fmt.Fprintf(w, "%s: no findings\n", res.File)
		return
	}
	kinds := sortedKeys(res.Counts)
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%d %s", res.Counts[k], k))
	}
	fmt.Fprintf(w, "%s: %s (%d gated)\n", res.File, strings.Join(parts, ", "), res.Gated)
	for _, f := range res.Findings {
		mark := " "
		if f.Gated {
			mark = "!"
		}
		quoted := make([]string, len(f.Labels))
		for i, l := range f.Labels {
			quoted[i] = fmt.Sprintf("%q", l)
		}
		page := ""
		if f.Page != "" {
			page = "[" + f.Page + "] "
		}
		fmt.Fprintf(w, "  %s %-13s %s%s %s", mark, f.Kind, page, strings.Join(f.CellIDs, " / "), strings.Join(quoted, " / "))
		if f.Kind != drawioapi.FindingNotMeasured {
			fmt.Fprintf(w, " at (%.0f, %.0f) by %.1f px", f.Box.X, f.Box.Y, f.Depth)
		}
		fmt.Fprintln(w)
	}
}

func newExportCommand() *cobra.Command {
	var (
		outputPath string
		format     string
	)
	cmd := &cobra.Command{
		Use:   "export <diagram.drawio>",
		Short: "Export a .drawio file to PNG, SVG or PDF",
		Long: `Exports through the draw.io desktop CLI, which must be installed
(or on PATH as draw.io / drawio).

Examples:
  ktw-docs drawio export landscape.drawio --format svg
  ktw-docs drawio export landscape.drawio -o landscape.png`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("format") && outputPath != "" {
				// -o x.png means PNG.
				if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(outputPath)), "."); ext != "" {
					format = ext
				}
			}
			switch format {
			case "png", "svg", "pdf":
			default:
				return fmt.Errorf("unsupported format %q (png, svg or pdf)", format)
			}
			data, err := drawiorender.Export(args[0], format)
			if err != nil {
				return err
			}
			if outputPath == "" {
				outputPath = strings.TrimSuffix(args[0], filepath.Ext(args[0])) + "." + format
			}
			if err := os.WriteFile(outputPath, data, 0o644); err != nil {
				return fmt.Errorf("write %q: %w", outputPath, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d bytes)\n", outputPath, len(data))
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: <input>.<format>)")
	cmd.Flags().StringVarP(&format, "format", "f", "svg", "png, svg or pdf (default: the -o extension, else svg)")
	return cmd
}

func readLogical(path string) (*drawioapi.LogicalDiagram, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	var ld drawioapi.LogicalDiagram
	if err := json.Unmarshal(raw, &ld); err != nil {
		return nil, fmt.Errorf("parse LogicalDiagram JSON %q: %w", path, err)
	}
	return &ld, nil
}

// issueJSON is the --json shape of one finding.
type issueJSON struct {
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func printIssues(w io.Writer, r *drawiorender.ValidationResult, asJSON bool) error {
	var all []issueJSON
	for _, e := range r.Errors {
		all = append(all, issueJSON{"error", e.Path, e.Code, e.Message})
	}
	for _, e := range r.Warnings {
		all = append(all, issueJSON{"warning", e.Path, e.Code, e.Message})
	}
	if asJSON {
		if all == nil {
			all = []issueJSON{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(all)
	}
	if len(all) == 0 {
		_, err := fmt.Fprintln(w, "no findings")
		return err
	}
	for _, i := range all {
		if _, err := fmt.Fprintf(w, "%s %s [%s] %s\n", i.Severity, i.Path, i.Code, i.Message); err != nil {
			return err
		}
	}
	return nil
}
