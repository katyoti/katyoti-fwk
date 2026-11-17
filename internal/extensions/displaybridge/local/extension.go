// SPDX-License-Identifier: Apache-2.0

// Package local declares the SchemaBridge as a published service. The
// Extension() function is the canonical 1-line integration point: any binary
// drops `displaybridgelocal.Extension()` into its `buildExtensions()` slice
// and gets the bridge service published with bun lazy-spawn behind it.
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
	bridge "github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/internal/core"
)

// Extension publishes SchemaBridge with KTSP capabilities AND wires the bun
// external process via WithExternalProcess. On first call to any bridge slot,
// the supervisor lazy-spawns ktw-display, probes /healthz until ready, and
// keeps the process alive (with restart-on-crash + exponential backoff).
//
// Slot tags: cli="false" suppresses auto-CLI generation. The legacy
// `ktw-agents schema parse` Cobra command stays in place during the
// coexistence period; CLI rewire happens in `deprecate-go-schema-extension`.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishSimpleService[api.SchemaBridge](
			bridge.New,
			appcore.WithServiceDef(api.SchemaBridgeServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
			publish.WithExternalProcess(bridge.BunSpec()),
		))
	}
}
