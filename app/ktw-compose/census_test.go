// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/app/ktw-compose/cmd"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// TestCensus guards what this binary publishes. See internal/test/census.
func TestCensus(t *testing.T) {
	// compose-edit is a `permission-profile`: a kind the agent extension
	// declares, published by ktw-agents.
	census.AssertWithin(t, "ktw-compose", "test", cmd.Extensions(embeddedFS),
		census.DeclarationsOf(t, "ktw-agents", "internal/extensions/agent/contrib/embed"))
}
