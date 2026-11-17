// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/app/ktw-claude/cmd"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// TestCensus guards what this binary publishes. See internal/test/census.
func TestCensus(t *testing.T) {
	census.Assert(t, "ktw-claude", "test", cmd.Extensions(embeddedFS))
}
