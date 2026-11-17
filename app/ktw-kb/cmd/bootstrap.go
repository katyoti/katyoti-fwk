// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	kblocal "github.com/katyoti/katyoti-fwk/internal/extensions/kb/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-kb sidecar.
// Migrated to the sidecarkit builder 2026-05-23 (SN-12).
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: "ktw-kb",
		Version: version,
		Short:   "ktw-kb — knowledge base pipeline",
		Long: `ktw-kb manages knowledge bases: sync document sources, extract knowledge
bundles, chunk and embed content, index into vector databases, and query
with full source traceability.`,
		EmbeddedFS: efs,
	},
		sidecarkit.WithDomain(kblocal.Extension()),
		sidecarkit.WithKTSPDescription("Knowledge base pipeline"),
		sidecarkit.WithFlatCLI(),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
