// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/cli"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// assertTheCommandLineGenerates builds the command line generated from every
// service this binary publishes, as the binary does at start: an input field
// named like a flag every method carries (--out, --of) panics there, and the
// binary would not start at all.
// It answers the generated tree, nil when it did not build.
func assertTheCommandLineGenerates(t *testing.T, app *application.Application) (generated *cobra.Command) {
	t.Helper()
	container, err := application.AppUseService[*ktsp.Endpoint](app)
	if err != nil {
		t.Fatalf("no KTSP endpoint: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			generated = nil
			t.Errorf("the command line does not build - the binary would not start: %v", fmt.Sprint(r))
		}
	}()
	generated = &cobra.Command{Use: "ktw-agents"}
	cli.GenerateCLI(generated, container)
	return generated
}

// assertGeneratedFlagsAreDashed walks every generated command: no flag is
// spelled with an underscore (a person types --dry-run, never --dry_run), and
// a flag whose field is spelled with one keeps that spelling as a hidden alias
// that parses to the same flag.
func assertGeneratedFlagsAreDashed(t *testing.T, root *cobra.Command) {
	t.Helper()
	if root == nil {
		return
	}
	aliased := 0
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if strings.Contains(f.Name, "_") {
				t.Errorf("`%s --%s`: a generated flag is spelled with dashes", c.CommandPath(), f.Name)
			}
			if alias := strings.ReplaceAll(f.Name, "-", "_"); alias != f.Name && c.Flags().Lookup(alias) == f {
				aliased++
			}
		})
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	if aliased == 0 {
		t.Error("no generated flag keeps its underscore spelling as an alias")
	}
}
