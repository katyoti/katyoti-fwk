// SPDX-License-Identifier: Apache-2.0

// Package local is the gocode-acceptance module's framework-
// contribution package. By the 3-package convention this is where
// `application.Extension` factories live — wiring the module's
// public surface into the host binary.
//
// This is a fixture module, so the Extension factory below is a
// stub that does nothing. Real modules use this package to call
// `appcore.PublishService`, register skills/personas, etc.
package local

import "github.com/katyoti/katyoti-fwk/internal/core/application"

// Extension is the no-op contribution factory the host would call
// if this module were ever wired into a real binary. Lives here
// as an example of the 3-package convention's `local/` slot;
// returning a no-op extension keeps the module compilable without
// adding behavior that would skew acceptance metrics.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		// Intentionally empty — fixture module makes no
		// framework contributions.
	}
}
