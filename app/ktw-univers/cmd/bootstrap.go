// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	marketplacelocal "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/commands"
	orchestratorlocal "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/local"
	profilelocal "github.com/katyoti/katyoti-fwk/internal/extensions/profile/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-univers binary.
//
// ktw-univers is a CLI/sidecar for universe catalog operations (5 KTSP methods:
// list, get, create, delete, scan-inventory). It does NOT have a daemon mode
// and does NOT manage active universes — those live in ktw-gui
// (modules/gui/concepts.md §2 (the ActiveCore model)).
//
// Notes:
//   - NO HTTP server: ktw-univers is KTSP-only (WithoutHTTPServer).
//   - Auto-CLI is suppressed for OrchestratorService via WithKTSPSlotTags
//     (in orchestratorlocal.Extension()); we provide hand-written Cobra
//     commands here for nicer UX (via WithExtraExtensions).
//
// Migrated to the sidecarkit builder 2026-05-23 (SN-12).
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName:    "ktw-univers",
		Version:    version,
		Short:      "ktw-univers — universe catalog CLI/sidecar",
		Long:       `ktw-univers manages the universe catalog at ~/.config/katyoti/universes.yaml. Provides catalog reads (list, info), catalog writes (create, delete), and system inventory scanning. Active-universe management lives in ktw-gui, which runs one ktw-hub per active universe.`,
		EmbeddedFS: efs,
	},
		// The admin universe's management sidecar serves the management services
		// over KTSP: the universe registry (orchestrator), the marketplace
		// ((modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)) and the user's profile. One container.
		sidecarkit.WithDomain(orchestratorlocal.Extension(), marketplacelocal.KTSPServiceExtension(), profilelocal.KTSPServiceExtension(),
			// The guide's universe checks: read-only methods a recipe names as its check.
			orchestratorlocal.ChecksExtension(), orchestratorlocal.SetupExtension()),
		sidecarkit.WithKTSPDescription("ktw-univers catalog service"),
		sidecarkit.WithoutHTTPServer(),
		sidecarkit.WithExtraExtensions(
			// Hand-written Cobra commands (auto-CLI suppressed in orchestratorlocal).
			appcore.CommandExt(commands.NewListCommand()),
			appcore.CommandExt(commands.NewResolveObjectCommand()),
			appcore.CommandExt(commands.NewInfoCommand()),
			appcore.CommandExt(commands.NewInitCommand()),
			appcore.CommandExt(commands.NewCreateCommand()),
			appcore.CommandExt(commands.NewDeleteCommand()),
			appcore.CommandExt(commands.NewAddSidecarCommand()),
			appcore.CommandExt(commands.NewRemoveSidecarCommand()),
			appcore.CommandExt(commands.NewScanInventoryCommand()),
		),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
