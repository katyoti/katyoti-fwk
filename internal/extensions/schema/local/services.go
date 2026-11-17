// SPDX-License-Identifier: Apache-2.0

// Package local declares the SchemaService as a published service. The
// Extension() function is the canonical 1-line integration point: any binary
// (ktw-agents subcommand, future ktw-schema sidecar, or arbitrary other
// container) drops `schemalocal.Extension()` into its `buildExtensions()` slice
// and gets the parser+normalizer behind KTSP / HTTP / Cobra surfaces
// automatically.
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemacmd "github.com/katyoti/katyoti-fwk/internal/extensions/schema/commands"
	schemacore "github.com/katyoti/katyoti-fwk/internal/extensions/schema/internal/core"
)

// Extension publishes SchemaService with KTSP capabilities AND contributes the
// `schema` Cobra command. Any binary that imports this extension gains both
// the in-process service and the LLM-drivable CLI surface.
//
// Mirrors the skills-extension pattern: the hand-written `schema` command is
// the canonical CLI; auto-CLI generation from the ServiceDef is suppressed via
// WithKTSPSlotTags("cli": "false") so we don't get duplicate verbs.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishSimpleService[api.SchemaService](
			schemacore.New,
			appcore.WithServiceDef(api.SchemaServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
		))

		// Contribute the `schema` Cobra command. Any binary that imports this
		// extension automatically gains the LLM-drivable CLI surface — no
		// per-sidecar wiring required.
		init.AddExtension(appcore.CommandExt(schemacmd.NewSchemaCommand()))
	}
}
