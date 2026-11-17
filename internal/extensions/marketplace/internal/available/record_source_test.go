// SPDX-License-Identifier: Apache-2.0

package available

import (
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// Record must PRESERVE the caller's Source.
//
// It used to overwrite it with the literal "marketplace", discarding the one
// field that tells a control-plane binary apart from a catalog sidecar:
// install-infra records its four with Source="infra-channel", and this
// relabelled them on the way to disk.
//
// Everything downstream filters on that field, so one hardcoded string produced
// a run of unrelated-looking bugs — `katyoti doctor` exec'd `ktw-gui sidecar
// config check` and opened a WINDOW; the GUI listed katyoti and ktw-gui as
// sidecars with an uninstall button; `katyoti installed` called them marketplace
// items while printing the infra channel as their marketplace.
func TestRecord_PreservesSource(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"infra channel is kept", "infra-channel", "infra-channel"},
		{"marketplace is kept", "marketplace", "marketplace"},
		{"empty defaults to marketplace", "", "marketplace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := entryFor(mktapi.Installed{Name: "x", Source: tc.in})
			if got.Source != tc.want {
				t.Errorf("Source = %q, want %q", got.Source, tc.want)
			}
		})
	}
}
