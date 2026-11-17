// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/app/ktw-methods/cmd"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// TestCensus guards what this binary publishes. See internal/test/census.
//
// Riding on the census's application (a process builds exactly one), it also
// checks every `action run --uri ktw-methods:…` an embedded task or skill of
// any binary runs: the method is one this binary publishes.
func TestCensus(t *testing.T) {
	census.Isolate(t)
	app := application.New(application.Config(), cmd.Extensions(embeddedFS)...)
	census.AssertApp(t, "ktw-methods", "test", app)
	census.AssertActionURIsResolve(t, "ktw-methods", app, 2)
}
