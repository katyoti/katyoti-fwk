// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	cwmclient "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/commands"
	docscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/docs/contrib"
	docslocal "github.com/katyoti/katyoti-fwk/internal/extensions/docs/local"
	drawiocontrib "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/contrib/drawiokind"
	drawiolocal "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
	slidedeckcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib/deckkind"
	slidedecklocal "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-docs sidecar.
// DocService is published with KTSP + HTTP capabilities; Auto* extensions
// read these to generate container and HTTP server automatically.
// Migrated to the sidecarkit builder 2026-05-23 (SN-12).
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName:    "ktw-docs",
		Version:    version,
		Short:      "ktw-docs — document and diagram transformation server",
		Long:       `ktw-docs is a document and diagram transformation sidecar for the ktw-agents ecosystem.`,
		EmbeddedFS: efs,
	},
		// Its pack (embed/: data-types, contribution-types, references,
		// skills, and tasks — the slide-deck engine's « Création d'une
		// présentation », run by ktw-agents) is the kit's; the DocService is
		// its domain.
		sidecarkit.WithDomain(docslocal.Extension()),
		sidecarkit.WithKTSPDescription("ktw-docs sidecar"),
		sidecarkit.WithFlatCLI(),
		sidecarkit.WithExtraExtensions(
			// The drawio kinds (notation, viewpoint, drawio-template): built-in
			// leaves, published here since its drawio content sits under
			// embed/drawio, not at its pack root.
			builtin.ContributeLeafKinds("", drawiokind.Leaves()...),
			// The deck kinds deck-theme and deck-asset: built-in leaves,
			// published here since its built-in theme (katyoti) sits in the
			// slide-deck engine, not at its pack root; slide-group is held by
			// its pack (embed/slide-groups: slides any deck may include).
			builtin.ContributeLeafKinds("", deckkind.Theme(), deckkind.Asset()),
			// The drawio engine: its built-in notations, viewpoints and
			// template libraries (embed/drawio/), its layout presets
			// (embed/elk-layouts/) and its CLI.
			drawiocontrib.ContributeDrawio("ktw-docs", loader.NewEmbedFSLoader(efs, "."), "embed/drawio"),
			drawiolocal.Extension(efs),
			// The slide-deck engine: its built-in layouts (deck-layout, kind
			// and data-type in this pack) and theme (deck-theme katyoti, read
			// as a pack) and its CLI (`deck lint|layouts|themes`).
			slidedeckcontrib.ContributeLayouts("ktw-docs"),
			slidedeckcontrib.ContributeBuiltinThemes("ktw-docs"),
			// …and its CWM pack: the slide-deck object type, the `deck`
			// field-group and the deck panel on its pane.
			slidedeckcontrib.Pack("ktw-docs"),
			// Content templates (th21): what a deliverable must say, whatever
			// its format — the kind is in embed/contribution-types, the
			// templates in embed/content-templates; the pack holds the
			// content-template object type a project keeps its own as.
			docscontrib.ContributeTemplates("", loader.NewEmbedFSLoader(efs, "."), "embed/content-templates"),
			docscontrib.Pack("ktw-docs"),
			slidedecklocal.Extension(),
			// CWM lives in ktw-agents: the deck service reaches the project's
			// deck-theme objects (the lint hook's themes, an imported theme
			// stored in a module) through the bus client.
			cwmclient.RemoteExtension(),
			busclientbus.Extension(),           // the bus-backed Sender the generated client calls through
			busclientbus.RunningHubExtension(), // a one-shot CLI's bus: the running hub's (`deck lint|themes --universe`)
			appcore.CommandExt(commands.NewFilterCommand()),
			appcore.CommandExt(commands.NewDocCommand(efs)),
		),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
