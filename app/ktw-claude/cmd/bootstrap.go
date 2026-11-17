// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	claudelocal "github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// buildExtensions declares the ktw-claude sidecar.
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName:    "ktw-claude",
		Version:    version,
		Short:      "ktw-claude - Claude Code workers on the person's own subscription",
		EmbeddedFS: efs,
	},
		sidecarkit.WithDomain(claudelocal.Extension()),
		sidecarkit.WithKTSPDescription("Claude Code workers"),
		sidecarkit.WithFlatCLI(),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
