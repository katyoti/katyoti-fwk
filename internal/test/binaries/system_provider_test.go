// SPDX-License-Identifier: Apache-2.0

package binaries

import (
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/test/census"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// TestCompiledBinariesCarryTheSystemProviderOnce asks the e15 rule of the
// shipped executables: every compiled sidecar registers the system provider
// exactly once per system type (contribution-type, data-type) and carries the
// system kind declarations — the same decision the in-process census makes
// (census.SystemProviderFindings), judged on the artifact.
//
// The in-process census cannot see what a `cmd/` wires that the extension
// list does not; this can. Skips without bin/ (hubharness.MissingBinary).
func TestCompiledBinariesCarryTheSystemProviderOnce(t *testing.T) {
	long.Test(t)
	bins := builtBinaries(t)
	if len(bins) == 0 {
		return
	}
	names := make([]string, 0, len(bins))
	for name := range bins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if why, ok := notSidecars[name]; ok {
				t.Skipf("%s has no `sc resources manifest` surface: %s", name, why)
			}
			if why, ok := census.NotSystemCarriers[name]; ok {
				t.Skipf("%s is a declared non-carrier: %s", name, why)
			}
			m := readManifest(t, name, bins[name])
			if bad := census.SystemProviderFindings(m); len(bad) > 0 {
				t.Errorf("%s: the system provider is not carried exactly once (the single-system-provider decision):\n%s",
					name, strings.Join(bad, "\n"))
			}
		})
	}
}
