// SPDX-License-Identifier: Apache-2.0

// Package commands provides Cobra commands contributed by the code extension.
//
// `module verify <module>` runs UC-driven E2E verification per the
// `code-module verify` verb spec at app/ktw-code/embed/references/modules/verbs/verify.md.
// Phase B implementation: L1 only, with text/JUnit/JSON output.
package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/verify"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/spf13/cobra"
)

// NewModuleCommand returns the `module` Cobra root with its subcommands.
// Today: `verify`. Future: `status`, `lint`, `audit`, `promote` per the
// verb spec catalogue.
func NewModuleCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "module",
		Short: "Per-module operations (verify, status, ...)",
		Long: `Per-module operations driven by /modules/<name>/charter.md.

The command group operationalises the rock-solid module method: run the
charter's Test plan as a UC-keyed gate, emit JUnit-compatible results
that drop into VS Code Test Explorer / GitHub Actions / etc.

Run "module verify <module>" to see the per-UC tree.`,
		SilenceUsage: true,
	}
	root.AddCommand(newVerifyCommand())
	root.AddCommand(newModuleProjectCommand())
	return root
}

func newVerifyCommand() *cobra.Command {
	var (
		layer      string
		ucs        []string
		severity   []string
		format     string
		timeout    time.Duration
		project    string
		noProgress bool
		detail     bool
		keepTemp   bool
		noWrite    bool
	)
	cmd := &cobra.Command{
		Use:   "verify <module>",
		Short: "Run UC-driven E2E verification for a module",
		Long: `Run the charter's Test plan as a UC-keyed gate and emit a green/red board.

The command parses /modules/<module>/charter.md, runs go test for each named
L1 test, and reports per-UC verdicts (🟢 / 🔴 / 🟠 / ⚪ / ⏸).

Output formats:
  text   default — human-readable tree
  junit  JUnit XML — drops into VS Code Test Explorer, GitHub Actions, etc.
  json   structured form per the verb spec

Examples:
  module verify tasks
  module verify tasks --format junit > results.xml
  module verify tasks --uc UC-053 --uc UC-057
  module verify tasks --severity security --format json | jq .totals
`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			if project == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				project = cwd
			}

			runOpts := verify.Options{
				Module:         args[0],
				ProjectRoot:    project,
				Layer:          layer,
				UCFilter:       ucs,
				SeverityFilter: severity,
				Timeout:        timeout,
				KeepTemp:       keepTemp,
				NoWrite:        noWrite,
			}
			if !noWrite {
				// The run is recorded through CWM's Object API, in CWM's
				// workspace — resolved once, before any test runs.
				cwm, err := application.UseService[cwmapi.CWMService](ctx)
				if err != nil {
					return fmt.Errorf("module verify: resolve CWMService to record the run (or pass --no-write): %w", err)
				}
				// Reached before any test runs: a CWM that cannot be reached
				// (no hub for the universe) or does not know the module is
				// said now, not after the suite has run.
				if _, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: args[0]}); err != nil {
					return fmt.Errorf("module verify: CWM cannot record the run (pass --no-write to run without recording): %w", err)
				}
				runOpts.CWM = cwm
			}
			if !noProgress {
				stderr := cmd.ErrOrStderr()
				runOpts.OnProgress = func(idx, total int, ucID string, v verify.Verdict, dur time.Duration) {
					_, _ = fmt.Fprintf(stderr, "[%d/%d] %s %s %s (%.2fs)\n",
						idx, total, ucID, v.Symbol(), v.String(), dur.Seconds())
				}
			}
			report, err := verify.Run(ctx, runOpts)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			switch strings.ToLower(format) {
			case "junit":
				if err := verify.EmitJUnit(out, report); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(out)
			case "json":
				if err := verify.EmitJSON(out, report); err != nil {
					return err
				}
			case "", "text", "md":
				if err := verify.EmitTextOpts(out, report, verify.TextOptions{Detail: detail}); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported --format %q (use text, junit, or json)", format)
			}

			// Exit 1 if any UC blocks promotion (fail or no-test). Detect via
			// the report's tier-1 DoD flag — same gate the spec calls out.
			if !report.Tier1DoDMet {
				cmd.SilenceErrors = true
				return fmt.Errorf("verify: tier-1 DoD not met (%d fail, %d no-test)",
					report.Totals.Fail, report.Totals.NoTest)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&layer, "layer", "L1", "Test layer to run: L1 | L2 | L3 | all (Phase B: L1 only)")
	cmd.Flags().StringSliceVar(&ucs, "uc", nil, "Filter to specific UC IDs or patterns (e.g. UC-053, UC-04*)")
	cmd.Flags().StringSliceVar(&severity, "severity", nil, "Filter to specific severities: core | security | qol | core+security | all")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text | junit | json")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "Per-test timeout")
	cmd.Flags().StringVar(&project, "project", "", "Project root (defaults to current working directory)")
	cmd.Flags().BoolVar(&noProgress, "no-progress", false, "Suppress per-UC progress lines printed to stderr while running")
	cmd.Flags().BoolVar(&detail, "detail", false, "Expand every UC row in the text tree (default: collapse pass + skipped under group counts)")
	cmd.Flags().BoolVar(&keepTemp, "keep-temp", false, "Leave per-scenario tmp homes on disk after L2 runs (debugging)")
	cmd.Flags().BoolVar(&noWrite, "no-write", false, "Run without recording a test-run object (read-only run)")
	return cmd
}
