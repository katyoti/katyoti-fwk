// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	notifylocal "github.com/katyoti/katyoti-fwk/internal/extensions/notify/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-notify sidecar.
// Migrated to the sidecarkit builder 2026-05-23 (SN-12).
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName:    "ktw-notify",
		Version:    version,
		Short:      "ktw-notify — notification sidecar",
		EmbeddedFS: efs,
	},
		sidecarkit.WithDomain(notifylocal.Extension()),
		sidecarkit.WithKTSPDescription("Notification services"),
		sidecarkit.WithFlatCLI(),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
