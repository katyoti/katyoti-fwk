// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/app/ktw-demo/cmd"
	decklint "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/lint"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// TestCensus guards what this binary publishes. See internal/test/census.
// Its deck theme's kinds are built-in leaves its pack holds: it declares them
// itself.
func TestCensus(t *testing.T) {
	census.Assert(t, "ktw-demo", "test", cmd.Extensions(embeddedFS))
}

// The demo deck theme is sound: every file it names is shipped and every
// shipped file named, names kebab, each file within the control band.
func TestDeckThemesAreSound(t *testing.T) {
	for _, err := range decklint.CheckPack(embeddedFS, "embed") {
		t.Error(err)
	}
}
