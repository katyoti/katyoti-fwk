// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	thlocal "github.com/katyoti/katyoti-fwk/internal/extensions/testharness/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-test sidecar.
func buildExtensions(efs embed.FS) []application.Extension {
	return []application.Extension{
		appcore.NewAppExt(appcore.AppConfig{
			AppName:    "ktw-test",
			Version:    version,
			Short:      "ktw-test — virtual terminal test harness",
			EmbeddedFS: efs,
		}),
		thlocal.Extension(),
		// `ktw-test test` — run the integration suites (go test over
		// ./internal/test/...). The harness server above spawns single
		// processes; this runs the hubharness-backed end-to-end suites.
		appcore.CommandExt(newTestCommand()),
	}
}
