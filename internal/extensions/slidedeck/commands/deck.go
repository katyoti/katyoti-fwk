// SPDX-License-Identifier: Apache-2.0

// Package commands holds the slide-deck engine's CLI: `deck lint`,
// `deck layouts`, `deck themes`, `deck import-theme` and `deck export`.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/internal/pptx"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/remote"
	"github.com/spf13/cobra"
)

// NewDeckCommand builds the `deck` command group.
func NewDeckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deck",
		Short: "Check and export slide decks; list their layouts and themes",
		Long: `The slide-deck engine. A deck is one markdown file: a frontmatter block
(title, theme, aspect, transition), slides separated by '---' ('--' for a
vertical slide), a '<!-- slide: key=value -->' first line for a slide's
settings, '::name::' for a layout slot and 'Note:' for speaker notes. A theme
styles the deck; a slide picks one of its looks with 'variant=<name>' and
writes its pictograms as '![label](icon:<name>)'.
Grammar, layouts and themes: modules/slidedeck/concepts.md.`,
	}
	cmd.AddCommand(newLintCommand(), newLayoutsCommand(), newThemesCommand(), newImportThemeCommand(), newExportCommand())
	return cmd
}

// queryOf is the reach a command reads: the universe's catalog (over the bus
// from a sidecar), or this binary's own. The flag is --all-sidecars, never
// --universe: every binary reads `--universe <name>` at start-up to choose its
// universe (cenv.Preflight), and took the deck file after it for one.
func queryOf(ctx context.Context, universe bool) mbapi.Q {
	if universe {
		return remote.Universe(ctx)
	}
	return mbapi.Q{Reach: mbapi.ReachSelf}
}

func newLintCommand() *cobra.Command {
	var (
		asJSON   bool
		universe bool
	)
	cmd := &cobra.Command{
		Use:   "lint <deck.md>",
		Short: "Check a deck against the grammar, the layouts and the themes",
		Long: `Reports every problem with its line: frontmatter, slide settings, unknown
layouts and slots, missing required slots, misplaced settings lines,
unclosed code blocks, an unknown theme, a variant or an icon the deck's
theme does not declare. Exits 1 when there is an error.

Examples:
  ktw-docs deck lint review.md
  ktw-docs deck lint review.md --json
  ktw-docs deck lint review.md --all-sidecars   # layouts and themes from every sidecar`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			g, err := deckapi.GrammarAt(cmd.Context(), queryOf(cmd.Context(), universe))
			if err != nil {
				return fmt.Errorf("read layouts and themes: %w", err)
			}
			findings := deckapi.Lint(string(src), g)
			// The files it names beside it: in the file's folder.
			dir := filepath.Dir(args[0])
			findings = append(findings, deckapi.LintAssets(string(src), func(rel string) bool {
				_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
				return err == nil
			})...)
			sort.SliceStable(findings, func(i, j int) bool { return findings[i].Line < findings[j].Line })
			if asJSON {
				if findings == nil {
					findings = []deckapi.Finding{}
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(findings); err != nil {
					return err
				}
			} else {
				for _, f := range findings {
					fmt.Fprintf(cmd.OutOrStdout(), "%s:%d: %s %s: %s\n", args[0], f.Line, f.Severity, f.Rule, f.Message)
				}
				if len(findings) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: ok (%d slides)\n", args[0], len(deckapi.Parse(string(src)).Slides))
				}
			}
			if deckapi.HasErrors(findings) {
				return fmt.Errorf("%s: deck has errors", args[0])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the findings as JSON")
	cmd.Flags().BoolVar(&universe, "all-sidecars", false, "Check against the layouts and themes of every sidecar of the universe, not only this one")
	return cmd
}

func newLayoutsCommand() *cobra.Command {
	var (
		asJSON   bool
		universe bool
	)
	cmd := &cobra.Command{
		Use:          "layouts",
		Short:        "List the slide layouts and their slots",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			layouts, err := deckapi.Layouts(cmd.Context(), queryOf(cmd.Context(), universe))
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(layouts)
			}
			for _, l := range layouts {
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s %s\n", l.Name, l.Description)
				for _, s := range l.Slots {
					req := ""
					if s.Required {
						req = " (required)"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  ::%s::%s %s\n", s.Name, req, s.Description)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the layouts as JSON")
	cmd.Flags().BoolVar(&universe, "all-sidecars", false, "List layouts from every sidecar of the universe")
	return cmd
}

func newThemesCommand() *cobra.Command {
	var (
		asJSON   bool
		universe bool
	)
	cmd := &cobra.Command{
		Use:   "themes",
		Short: "List the deck themes, their variants and icons",
		Long: `Lists every deck theme in reach with the sidecar that publishes it, the
variants it declares per layout and how many icons it has. A deck picks a
theme with 'theme: <name>' in its frontmatter.

Examples:
  ktw-docs deck themes
  ktw-docs deck themes --all-sidecars   # themes of every sidecar
  ktw-docs deck themes --json`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			themes, err := deckapi.Themes(cmd.Context(), queryOf(cmd.Context(), universe))
			if err != nil {
				return err
			}
			if asJSON {
				type row struct {
					Provider string            `json:"provider"`
					Theme    deckapi.ThemeBody `json:"theme"`
				}
				rows := make([]row, 0, len(themes))
				for _, t := range themes {
					rows = append(rows, row{Provider: t.Provider, Theme: t.Body})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			out := cmd.OutOrStdout()
			for _, t := range themes {
				fmt.Fprintf(out, "%-12s %s — %s\n", t.Body.Name, t.Provider, t.Body.Description)
				layouts := make([]string, 0, len(t.Body.Layouts))
				for l := range t.Body.Layouts {
					layouts = append(layouts, l)
				}
				sort.Strings(layouts)
				for _, l := range layouts {
					if vs := t.Body.VariantNames(l); len(vs) > 0 {
						fmt.Fprintf(out, "  %s: variant=%s\n", l, strings.Join(vs, "|"))
					}
				}
				if n := len(t.Body.Icons); n > 0 {
					fmt.Fprintf(out, "  %d icons (icon:<name>)\n", n)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the themes as JSON")
	cmd.Flags().BoolVar(&universe, "all-sidecars", false, "List themes from every sidecar of the universe")
	return cmd
}

func newImportThemeCommand() *cobra.Command {
	var (
		name, description, out, report, decisionsFile string
		apply, asJSON                                 bool
	)
	cmd := &cobra.Command{
		Use:   "import-theme <template.pptx>",
		Short: "Propose a deck theme from a PowerPoint template",
		Long: `Reads a PowerPoint template (.pptx or .potx) and proposes the deck theme it
makes: colours and type scale from its master, the chrome (rules, logo,
footer), each layout mapped to a deck layout by its name then its structure,
the covers as variants named after their background, the pictograms drawn
on its slides as SVG icons, a sample deck. Deterministic: the same file gives
the same theme.

Without --apply it prints the import report (the mapping, the colours, what
to review) and writes nothing. With --apply it writes the theme into the pack
rooted at --out (<out>/deck-themes/<name>.yaml and <out>/deck-assets/<name>/)
and the report to --report. Nothing reads that folder: a theme is in reach
only when a sidecar ships it (--out the sidecar's embed/, then the sidecar
rebuilt). Review the report: names are guesses, a template
carries no font files and no brand charter.

--decisions names a JSON file of decisions applied over the proposal: a
template layout (its name, or its part such as slideLayout7.xml) to the deck
layout it becomes or "drop"; a variant renamed ("cover/sand": "sable"); the
theme's house rules ("guidance"):
  {"layouts": {"Page liens": "content"}, "variants": {"cover/sand": "sable"},
   "guidance": "Ouvrez par une couverture..."}

Examples:
  ktw-docs deck import-theme "MASQUE PPT.pptx" --name acme
  ktw-docs deck import-theme master.pptx --name acme --apply --out embed`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			var decisions deckapi.ImportDecisions
			if decisionsFile != "" {
				raw, err := os.ReadFile(decisionsFile)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(raw, &decisions); err != nil {
					return fmt.Errorf("%s: %w", decisionsFile, err)
				}
			}
			// The sample deck is written from the layouts' own examples, as
			// the universe serves them (a project's override included).
			layouts := deckapi.LayoutsInReach(cmd.Context(), remote.Universe(cmd.Context()))
			target := ""
			if apply {
				target = out
			}
			res, err := pptx.Import(data, pptx.Options{Name: name, Description: description, Layouts: layouts, Decisions: decisions}, filepath.Base(args[0]), target)
			if err != nil {
				return err
			}
			if apply {
				if report == "" {
					report = name + ".import-report.md"
				}
				if err := os.WriteFile(report, []byte(res.Report), 0o644); err != nil {
					return err
				}
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprint(cmd.OutOrStdout(), res.Report)
			if apply {
				fmt.Fprintf(cmd.OutOrStdout(), "\nWrote %s and %s/ (%d files); report: %s\n", filepath.Join(out, "deck-themes", name+".yaml"), filepath.Join(out, "deck-assets", name), len(res.Written)-1, report)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\nDry run: nothing written (--apply writes the theme folder).")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "The theme's name (kebab-case), as a deck's theme: names it")
	cmd.Flags().StringVar(&description, "description", "", "The theme's one-line description")
	cmd.Flags().StringVar(&out, "out", ".", "The pack root the theme is written into: deck-themes/ and deck-assets/ (with --apply)")
	cmd.Flags().StringVar(&report, "report", "", "Where --apply writes the import report (default <name>.import-report.md)")
	cmd.Flags().StringVar(&decisionsFile, "decisions", "", "A JSON file of decisions applied over the proposal (layouts, variants, guidance)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Write the theme into the pack; without it, a dry run")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the proposal as JSON")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}
