// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"flag"
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
)

// worldsAtOnce is how many tests run their worlds at once when the run does
// not say (-parallel): each test boots its own universe in its own temporary
// folder, and the package's time is mostly the file writes of those
// universes, which overlap well; more would only crowd a loaded machine.
const worldsAtOnce = "4"

// TestMain runs the package's universes, all in temporary folders, without
// the file syncs a power loss would need (: they were
// most of the package's time on Windows; the writes stay atomic), and runs
// its tests in parallel, worldsAtOnce at a time (: one
// after the other they passed go test's ten minutes on a loaded machine).
func TestMain(m *testing.M) {
	_ = os.Setenv(atomicio.NoSyncEnv, "1")
	flag.Parse()
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == "test.parallel" })
	if !set {
		_ = flag.Set("test.parallel", worldsAtOnce)
	}
	os.Exit(m.Run())
}
