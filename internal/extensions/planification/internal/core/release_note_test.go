// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A candidate's letter names its release once: by its version when that is
// its name (the default), else by its name and its version.
func TestALetterNamesItsReleaseOnce(t *testing.T) {
	for _, c := range []struct{ name, version, want string }{
		{"1.1.0", "1.1.0", "# Quoi de neuf - 1.1.0 (rc1)\n"},
		{"ArchiFactory", "2026.10", "# Quoi de neuf - ArchiFactory 2026.10 (rc1)\n"},
	} {
		got := letterOf(c.name, c.version, "rc1", api.NoteOutput{})
		if !strings.HasPrefix(got, c.want) {
			t.Errorf("letterOf(%q, %q) = %q; want it to start with %q", c.name, c.version, got, c.want)
		}
	}
}
