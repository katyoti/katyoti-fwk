// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmclient "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"
	demosignofflocal "github.com/katyoti/katyoti-fwk/internal/extensions/demosignoff/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-demo sidecar.
//
// ktw-demo is the reference extension: beside its demo tasks, data-types and
// deck theme, it ships the `signoff` capability — every mechanism a sidecar can
// give CWM, as contributions (embed/cwm, embed/tasks, embed/skills,
// embed/guide) served by one published service (internal/extensions/
// demosignoff). The guide that walks it: modules/cwm/objects/
// what-a-capability-and-a-sidecar-give.cwm.md §7.
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: "ktw-demo",
		Version: version,
		Short:   "ktw-demo — demo tasks and the reference sign-off capability",
		Long: `ktw-demo is an opt-in sidecar that ships demo / hello-world
tasks for the ktw-agents ecosystem, and the reference extension: a
sign-off capability using every mechanism a sidecar can give CWM — an
object type, a capability with its hooks and function, a workflow and
its gate, an aspect, a policy, an edge type, two panels, an agent
context, a task, a skill and a guide recipe. Production universes can
leave it out to keep their catalog clean.`,
		EmbeddedFS: efs,
	},
		// The sign-off service, as its slot `ktw-demo:signoff/<verb>` (panel
		// sources, acts, the function, the hooks CWM asks) — every `:signoff/`
		// URI of the pack.
		sidecarkit.WithDomain(demosignofflocal.Extension()),
		// Its embed/ pack is the kit's: the demo tasks, the data-types (the
		// Book record type for the schema-driven CRUD demos resolves as
		// ktw-demo:/data-type/book), the guide recipe that teaches the
		// sign-off capability and a demo deck theme (deck-themes/demo.yaml,
		// its files under deck-assets/demo/) - a deck written with
		// `theme: demo` shows in it.
		//
		// The sign-off pack (embed/cwm) and its data-types: a part of its
		// pack, read by the kit.
		sidecarkit.WithKTSPDescription("Demo tasks and the reference sign-off capability"),
		sidecarkit.WithoutHTTPServer(),
		sidecarkit.WithExtraExtensions(
			// CWM lives in ktw-agents: the sign-off service reaches it over the
			// bus.
			cwmclient.RemoteExtension(),
			busclientbus.Extension(),           // the bus-backed Sender the generated client calls through
			busclientbus.RunningHubExtension(), // the CLI's bus: the running hub's
		),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
