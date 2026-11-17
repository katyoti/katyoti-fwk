// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmclient "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"
	gocodelocal "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-gocode sidecar.
// Two services are published from this binary:
//
//   - GocodeService (orchestration + items + tasktest): module.*, uc.*,
//     od.*, feature.*, tasktest.* verbs.
//   - GoToolService (Go toolchain wrappers): go.*, gofmt, goimports verbs.
//
// AutoKTSPContainer + AutoHTTPServer auto-discover both services from
// the PublishedServicesPoint EM contributions made by
// gocodelocal.Extension().
//
// `WithoutSkillContent` is set because gocodelocal.Extension() contributes
// its own skill resources internally (6 personas, 9 skills, 3 GUI tasks
// from its own embed tree). The skills RUNTIME (skill_access plugin,
// `skills` Cobra command) is still wired.
// Migrated to the sidecarkit builder 2026-05-23 (SN-12).
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: "ktw-gocode",
		Version: version,
		Short:   "ktw-gocode — Go-coding method pack (charter / UCs / tests / audits / tier)",
		Long: `ktw-gocode is the L1 method pack on top of CWM that takes a Go module
from tier 0 to rock-solid. Greenfield (` + "`module create`" + `) and
retro-engineering (` + "`module discover`" + `) flows both supported.

In v1, most verbs are stubs returning ErrNotImplemented — the published
surface (CLI / KTSP / agent-tool / HTTP) is live so consumers can wire
against the stable API while real backings land via the ms-ktw-gocode
Phase B / D / E tasks.`,
		EmbeddedFS: efs,
	},
		sidecarkit.WithDomain(gocodelocal.Extension()),
		sidecarkit.WithKTSPDescription("ktw-gocode sidecar"),
		sidecarkit.WithFlatCLI(),
		// Pack CONTENT owned by this binary: its embed/ pack (skills, tasks,
		// personas) is the kit's. gocodelocal.Extension() ships only behavior.
		sidecarkit.WithExtraExtensions(
			// CWM lives in ktw-agents: the gocode verbs reach the module's
			// objects (module, use cases, ODs, test runs) through the bus client.
			cwmclient.RemoteExtension(),
			busclientbus.Extension(), // the bus-backed Sender the generated client calls through
		),
		// This binary's own CWM type pack (embed/cwm — the Go-specific
		// code-symbol kind) is a part of its pack, read by the kit.
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
