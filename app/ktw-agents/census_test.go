// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/app/ktw-agents/cmd"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/contrib/drawiokind"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
	"github.com/katyoti/katyoti-fwk/internal/test/schemalang"
)

// TestCensus guards what this binary publishes. See internal/test/census.
//
// ktw-agents constructs its application through its own bootstrap rather than
// from a plain extension list, so it isolates first and hands the built
// application to AssertApp.
func TestCensus(t *testing.T) {
	census.Isolate(t)
	app := cmd.BuildApp(embeddedFS)
	// The portfolio pack publishes its drawio content (a notation, a template
	// library, a viewpoint): kinds ktw-docs publishes from their
	// built-in leaf.
	census.AssertAppWithin(t, "ktw-agents", "test", app, append(
		census.DeclarationsOf(t, "ktw-docs", "app/ktw-docs/embed"),
		census.LeafDeclarationsOf(t, "ktw-docs", drawiokind.Leaves()...)...))
	// Riding on the census's application rather than building a second one:
	// strategies reach the domain only when an application runs, and a process
	// may build exactly one. This binary carries every strategy, so checking
	// here covers them all.
	schemalang.AssertStrategyDefaults(t, "ktw-agents")
	assertRestoresRunsWhenServing(t, app)
	generated := assertTheCommandLineGenerates(t, app)
	assertTasksNameRealCommands(t, generated)
	assertGeneratedFlagsAreDashed(t, generated)
}
