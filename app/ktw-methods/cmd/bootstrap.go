// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	acceptancelocal "github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/local"
	challengeslocal "github.com/katyoti/katyoti-fwk/internal/extensions/challenges/local"
	cwmclient "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"
	decisionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/local"
	featureslocal "github.com/katyoti/katyoti-fwk/internal/extensions/features/local"
	goalslocal "github.com/katyoti/katyoti-fwk/internal/extensions/goals/local"
	journalremote "github.com/katyoti/katyoti-fwk/internal/extensions/journal/remote"
	methodassistantlocal "github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/methodsservice"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
	usecaseslocal "github.com/katyoti/katyoti-fwk/internal/extensions/usecases/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-methods
// sidecar. Files-only: no domain service, no Go business logic. ALL
// content lives in the methods pack
// (internal/extensions/methods/local/embed) — the single source of
// truth shared with the in-process CWM host. This binary embeds
// nothing of its own; it serves the pack's `methodslocal.EmbedFS`.
func buildExtensions(efs embed.FS) []application.Extension {
	exts := sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: "ktw-methods",
		Version: version,
		Short:   "Centralized cross-domain articulation methods (ISA, interview)",
		Long: `Centralized cross-domain articulation methods (ISA, interview).

Files-only: skills (ISA + interview-technique + lifecycle) + references
(sourced from the shared methods pack) + the generic decomposition CWM
types (this binary's own embed/cwm).`,
		// This binary OWNS all methods pack content in app/ktw-methods/embed:
		// personas / skills / references + the CWM type pack (embed/cwm).
		EmbeddedFS: efs,
	},
		sidecarkit.WithKTSPDescription("Centralized cross-domain articulation methods (ISA, interview)"),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
	// This binary's own CWM type pack (embed/cwm) and its DSL body schemas
	// (`embed/cwm/data-types/<kind>.dsl.yaml` → `ktw-methods:/data-type/<kind>`)
	// are a part of its pack, read by the kit; the extensions this binary
	// hosts publish their own CWM packs under its name as parts beside it.
	return append(exts,
		// The one neutral Go service this binary hosts: the methods linters +
		// verifiers (stateless, methods-pack logic). Published as
		// `ktw-methods:methods/<verb>` so any node/item type's linter:/verifier:
		// declarations dispatch to it.
		methodsservice.Extension(),
		// CWM lives in ktw-agents: this binary's CWMService is the bus client,
		// which the services below reach the Object API through.
		cwmclient.RemoteExtension(),
		// The journal is ktw-agents' too: the Decision panel reads who decided
		// and when from a decision's last move.
		journalremote.RemoteExtension(),
		busclientbus.Extension(), // the bus-backed Sender the generated client calls through
		// The CLI's bus: the running hub's, so `ktw-methods decisions record …`
		// run by hand reaches CWM; served by the hub, the hub's bus wins.
		busclientbus.RunningHubExtension(),
		goalslocal.Extension(),      // ktw-methods:goals/<Verb> — and the Goals tab's board
		featureslocal.Extension(),   // ktw-methods:features/board — the Features tab
		challengeslocal.Extension(), // ktw-methods:challenges/challengers — the Challenged by tab
		acceptancelocal.Extension(), // ktw-methods:acceptance/verify — the verify hook of `checkable`
		decisionslocal.Extension(),  // ktw-methods:decisions/<Verb>
		usecaseslocal.Extension(),   // ktw-methods:use-cases/<Verb>
		// ktw-methods:method-assistant/<Verb>: the method panels' sources
		// (What's Next, the composition overview) and What's Next's act.
		methodassistantlocal.Extension(),
	)
}
