// SPDX-License-Identifier: Apache-2.0

package census

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// SystemProvider is the provider name the framework's own declarations carry
// (`system:/contribution-type/*`, `system:/data-type/*`).
const SystemProvider = "system"

// SystemProviderTypes are the kinds the system provider publishes, one
// provider registration each (internal/core/contribution/system_loader.go).
var SystemProviderTypes = []string{contribapi.ResourceTypeContributionType, contribapi.ResourceTypeDataType}

// NotSystemCarriers are the binaries that do not carry the system provider,
// and why. Rule: every binary carries and exposes it identically
// (modules/contributions/decisions/OD-004-which-binaries-carry-the-system-provider-and-how.cwm.md); a binary named here
// is an exception the ADR states, not a place to park a regression.
var NotSystemCarriers = map[string]string{
	"ktw-test": "test infrastructure (the virtual terminal harness): it publishes no contributions at all, so it has no kind to declare or validate",
}

// assertSystemProviderCarried: unless the binary is a declared non-carrier,
// its manifest registers the system provider exactly once per system type
// and carries at least one system kind declaration (e15).
//
// Exactly once, because two registrations are one provider wired twice
// (ktw-agents named it explicitly AND received it through appcore.SidecarExt
// until 2026-09-25): the entries dedupe by URI later, the provider rows do
// not, and a doubled row is a wiring fault the golden only shows as noise.
func assertSystemProviderCarried(t *testing.T, name string, m *mbapi.Manifest) {
	t.Helper()
	if _, exempt := NotSystemCarriers[name]; exempt {
		return
	}
	if bad := SystemProviderFindings(m); len(bad) > 0 {
		t.Errorf("census[%s]: the system provider is not carried exactly once (the single-system-provider decision):\n%s",
			name, strings.Join(bad, "\n"))
	}
}

// SystemProviderFindings is the decision: one line per violation, sorted.
// Exported for the compiled-binaries check, which asks the same question of
// the executable.
func SystemProviderFindings(m *mbapi.Manifest) []string {
	var bad []string
	count := map[string]int{}
	for _, p := range m.Providers {
		if p.Name == SystemProvider {
			count[p.Type]++
		}
	}
	for _, typ := range SystemProviderTypes {
		if n := count[typ]; n != 1 {
			bad = append(bad, fmt.Sprintf("  provider %s/%s registered %d time(s), want 1", SystemProvider, typ, n))
		}
	}
	kinds := 0
	for _, c := range m.Contributions {
		if c.Provider == SystemProvider && c.Type == contribapi.ResourceTypeContributionType {
			kinds++
		}
	}
	if kinds == 0 {
		bad = append(bad, "  no system:/contribution-type/ entry: the binary cannot validate its own kinds offline")
	}
	sort.Strings(bad)
	return bad
}
