// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/agent/contrib"
	composelocal "github.com/katyoti/katyoti-fwk/internal/extensions/compose/local"
	cwmclient "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"
	manifestuniverse "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/universe"
	scenariolocal "github.com/katyoti/katyoti-fwk/internal/extensions/scenario/local"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-compose sidecar.
//
// ktw-compose is the FILES-ONLY creator of Track A (ms-self-creation): it
// composes katyoti solutions from existing sidecars + files-only packs.
// The verb surface (compose-verb-surface, #11) is ComposeService —
// universe composition (orchestrator service in-process), files-only
// sidecar scaffolding, and artifact emission from the canonical templates.
// WithFlatCLI flattens the methods into kebab <verb>-<target> CLI verbs
// (create-universe, create-sidecar, create-skill, …).
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: "ktw-compose",
		Version: version,
		Short:   "ktw-compose — files-only solution composer",
		Long: `ktw-compose scaffolds and composes katyoti solutions from existing
sidecars plus files-only packs (skills, tasks, personas, configs — no Go code).

Verbs: create-universe, add-member/remove-member, create-sidecar,
apply-universe (or the interactive compose-universe), create-skill,
create-task, create-persona, create-reference, lint-skill, lint-task.
File-emitting verbs are in-repo only: run them from the ktw-agents repo root.`,
		EmbeddedFS: efs,
	},
		sidecarkit.WithDomain(composelocal.Extension(efs)),
		sidecarkit.WithKTSPDescription("Files-only solution composer (universe composition + scaffolding + artifact emission)"),
		sidecarkit.WithFlatCLI(),
		// Its embed/ pack is the kit's: the authoring/testing skills
		// (create-artifact, sidecar-testing), the references, and the
		// `compose-artifact` + `asset-intake` wizards (embed/tasks/), run by
		// the ktw-agents host (they need the CWM + agent runtime) — the
		// composition / triage node types' `actions:` launch them.
		//
		// This binary's own CWM node/item type pack (embed/cwm) is a part
		// of its pack, read by the kit.
		sidecarkit.WithExtraExtensions(
			// Scenario test cases: install a case into its own universe, run its
			// stages through the hub, checkpoint and replay (internal/extensions/scenario).
			scenariolocal.Extension(),
			// The compose-edit permission profile its wizards run under
			// (permission_profile: compose-edit): a `permission-profile`
			// contribution of this binary, the kind declared by the agent
			// extension.
			agentcontrib.ContributeProfiles("", loader.NewEmbedFSLoader(efs, "."), "embed/permission-profiles"),
			// Universe reach. ktw-compose authors content whose kinds and
			// schemas are declared elsewhere in the universe, so it has to be
			// able to ask what exists beyond itself.
			//
			// Costs two packages (manifest/universe + manifest/remote), linked
			// here on purpose — a pure sidecar links neither. Nothing is fetched and no
			// connection is opened until something actually queries the
			// universe, so the file-emitting verbs — which are entirely local
			// — run exactly as before, hub or no hub.
			manifestuniverse.UniverseCatalogExtension(),
			// The schema service, hosted IN-PROCESS rather than reached over the
			// bus. Packaging checks a pack against its own declared schemas
			// before it ships, and authoring is an offline flow: requiring a
			// reachable aggregator to package a folder would tax exactly the
			// friction-free path files-only content exists to provide.
			//
			// Measured cost: five packages, because this binary already links
			// most of the service's closure. Publish-only sidecars still link
			// zero — the boundary that keeps them small is a matter of who
			// imports the parser, and nothing here changes what they import.
			schemalocal.Extension(),
			// CWM over the bus: the --target project verbs (sys-contributions,
			// override objects, promote of an override object) reach the
			// project through the CWM service ktw-agents hosts; nothing is
			// connected until one of them runs.
			cwmclient.RemoteExtension(),
			// `compose-universe` — the interactive headline flow (huh forms);
			// hand-written because forms can't auto-derive from the
			// ServiceDef. Converges on ComposeService.ApplyUniverse.
			appcore.CommandExt(newComposeUniverseCommand()),
			// NOTE: build-marketplace / snapshot-universe / keygen / sign-marketplace
			// / publish-marketplace are now ComposeService METHODS (see
			// compose/api/marketplace.go) — the AutoKTSPContainer generates their
			// flat CLI verbs AND makes them KTSP-callable + object-type-addressable
			// actions, so they are no longer hand-written cobra commands.
		),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
