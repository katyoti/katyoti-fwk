// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// newTestCommand returns the `ktw-test test` subcommand — a thin wrapper
// over `go test` for the project's integration suites (the hubharness-
// backed end-to-end tests under internal/test/, e.g. the catalog contract
// test that spins up a real ktw-hub + sidecars and exercises
// `ktw-hub:/CatalogService/*` over the bus).
//
// With no package args it runs ./internal/test/... . Requires the Go
// toolchain + freshly-built binaries (`make` + `make ktw-hub`); the
// hubharness skips suites whose binaries are missing rather than failing.
//
//	ktw-test test                              # all integration suites
//	ktw-test test ./internal/test/catalog/ -v  # one suite, verbose
//	ktw-test test --run TestCatalogContract -v
func newTestCommand() *cobra.Command {
	var (
		verbose bool
		run     string
		timeout string
	)
	cmd := &cobra.Command{
		Use:   "test [packages...]",
		Short: "Run ktw integration test suites (go test; defaults to ./internal/test/...)",
		Long: `Run the project's integration test suites via the Go toolchain.

With no package arguments, runs ./internal/test/... — the hubharness-backed
end-to-end suites (catalog contract, demos, …) that spawn a real ktw-hub +
sidecars and drive the aggregator's catalog service over the bus.

Requires Go on PATH and freshly-built binaries (make + make ktw-hub). Run
from the repo root. Suites whose binaries are missing skip themselves.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgs := args
			if len(pkgs) == 0 {
				pkgs = []string{"./internal/test/..."}
			}
			goArgs := append([]string{"test"}, pkgs...)
			if verbose {
				goArgs = append(goArgs, "-v")
			}
			if run != "" {
				goArgs = append(goArgs, "-run", run)
			}
			if timeout != "" {
				goArgs = append(goArgs, "-timeout", timeout)
			}
			c := exec.CommandContext(cmd.Context(), "go", goArgs...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Stdin = os.Stdin
			return c.Run()
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output (go test -v)")
	cmd.Flags().StringVar(&run, "run", "", "only run tests matching this regexp (go test -run)")
	cmd.Flags().StringVar(&timeout, "timeout", "600s", "test timeout (go test -timeout)")
	return cmd
}
