// SPDX-License-Identifier: Apache-2.0

// Package local provides the in-process CodeService as a PublishedService.
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	codeapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	codecmd "github.com/katyoti/katyoti-fwk/internal/extensions/code/commands"
	codecore "github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// Extension registers CodeService as a published DI service AND contributes
// the `module` Cobra command. Any binary that imports this extension gains
// `module verify <module>` for free.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishService[codeapi.CodeService, codecore.CodeServiceConfig](
			codecore.NewCodeService,
			appcore.WithServiceDef(codeapi.CodeServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithHTTPServer(8892),
			appcore.WithKTSPSlotTags(map[string]string{
				ktsp.TagSlotCLIFlat: "false",
				ktsp.TagSlotCLIName: "code",
			}),
		))

		// Contribute `module verify <module>` — UC-driven E2E gate per
		// app/ktw-code/embed/references/modules/verbs/verify.md.
		init.AddExtension(appcore.CommandExt(codecmd.NewModuleCommand()))

		// CWM substrate types AND the methods PACK CONTENT (universal baseline
		// personas + methods skills + ISA references) are now contributed by the
		// OWNING sidecar binaries — code → app/ktw-code/embed; methods →
		// app/ktw-methods/embed — NOT bundled in-process here. The in-process CWM
		// host (ktw-agents) consumes all of it from the aggregate catalog
		// (migrate-cwm-type-packs-to-sidecars + migrate-pack-content-to-sidecars).
		// codelocal.Extension now ships only BEHAVIOR (CodeService + the `module`
		// command); a language pack's `augments:` persona layers merge onto the
		// methods baselines at the persona library, all from the catalog.
	}
}
