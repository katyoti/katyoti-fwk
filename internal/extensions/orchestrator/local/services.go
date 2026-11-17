// SPDX-License-Identifier: Apache-2.0

// Package local registers the ktw-univers service as a PublishedService
// so AutoKTSPContainer can build its KTSP slot surface.
//
// ktw-univers exposes 5 KTSP methods (List, Get, Create, Delete, ScanInventory).
// Auto-CLI is suppressed because the orchestrator's commands/ package provides
// hand-written Cobra commands (consistent with the rest of the binary's UX).
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	orchestratorcore "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/internal/core"
)

// Extension publishes the ktw-univers service.
//
// Capabilities:
//   - WithKTSPContainer: exposes the 5 methods over KTSP for ktw-gui (or any
//     KTSP client) to consume.
//   - WithKTSPSlotTags{"cli":"false"}: suppresses auto-generated CLI commands;
//     the commands/ package provides hand-written equivalents.
//
// No HTTP server: ktw-univers is KTSP-only. No config files: defaults suffice.
func Extension() application.Extension {
	return appcore.PublishService[api.OrchestratorService, orchestratorcore.ServiceConfig](
		orchestratorcore.NewService,
		appcore.WithServiceDef(api.OrchestratorServiceDef),
		appcore.WithKTSPContainer(),
		appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
	)
}

// ServiceExtension registers the OrchestratorService for in-process DI
// WITHOUT publishing a KTSP slot — for binaries (ktw-compose) whose own
// verbs delegate to the orchestrator while ktw-univers remains the single
// bus authority for universe methods.
func ServiceExtension() application.Extension {
	return application.DeclareService[api.OrchestratorService, orchestratorcore.ServiceConfig](orchestratorcore.NewService)
}

// ChecksExtension publishes the guide's universe checks (`universe-checks`
// slot): read-only methods a recipe names as its check. They read the
// OrchestratorService of the same binary.
func ChecksExtension() application.Extension {
	return appcore.PublishSimpleService[api.UniverseChecks](
		orchestratorcore.NewChecks,
		appcore.WithServiceDef(api.UniverseChecksServiceDef),
		appcore.WithKTSPContainer(),
		appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
	)
}

// SetupExtension publishes the guide's universe setup actions
// (`universe-setup` slot): what a recipe step does for the person after they
// confirm. They use the OrchestratorService of the same binary.
func SetupExtension() application.Extension {
	return appcore.PublishSimpleService[api.UniverseSetup](
		orchestratorcore.NewSetup,
		appcore.WithServiceDef(api.UniverseSetupServiceDef),
		appcore.WithKTSPContainer(),
		appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
	)
}
