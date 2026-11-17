// SPDX-License-Identifier: Apache-2.0

// Package local wires the katyoti marketplace engine + CLI as an
// application.Extension: it publishes the in-process SetupService and adds the
// top-level `marketplace` / `search` / `info` commands.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/commands"
	mktcore "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/core"
)

// NewService builds a standalone SetupService for a host WITHOUT an application
// container (e.g. ktw-gui, which has no in-process EM and calls the engine
// directly). The engine is dependency-free — it resolves its registry from the
// beside-exe seed + GlobalHomeDir — so it works with no universe active.
func NewService() (mktapi.SetupService, error) {
	return mktcore.New()
}

// KTSPServiceExtension publishes the SetupService OVER KTSP so it can be served
// in the admin universe ((modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)) — the GUI/CLI reach it
// as pure KTSP clients. Auto-CLI is suppressed (WithKTSPSlotTags cli:false): the
// hand-written `katyoti` commands own the CLI. Only KTSP-shaped methods are
// dispatched (the wire/unwire/bootstrap orchestration is not yet normalized).
func KTSPServiceExtension() application.Extension {
	return appcore.PublishSimpleService[mktapi.SetupService](
		func(_ context.Context) (mktapi.SetupService, error) {
			return mktcore.New()
		},
		appcore.WithServiceDef(mktapi.SetupServiceDef),
		appcore.WithKTSPContainer(),
		appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
	)
}

// ServiceExtension registers ONLY the in-process SetupService (no CLI
// commands) — for hosts that call the engine programmatically, e.g. ktw-gui's
// Marketplace bindings. The full Extension() = ServiceExtension() + the CLI.
func ServiceExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishSimpleService[mktapi.SetupService](
			func(_ context.Context) (mktapi.SetupService, error) {
				return mktcore.New()
			},
		))
		// InfraService is the same engine, registered IN-PROCESS ONLY. It is
		// absent from KTSPServiceExtension on purpose: replacing katyoti.exe,
		// ktw-hub.exe and ktw-gui.exe over the wire would mean a hub replacing
		// its own binary and the GUI's while both run. Off the wire, that is
		// unrepresentable rather than merely discouraged.
		init.AddExtension(appcore.PublishSimpleService[mktapi.InfraService](
			func(_ context.Context) (mktapi.InfraService, error) {
				return mktcore.NewInfra()
			},
		))
	}
}

// Extension registers the SetupService and the katyoti CLI surface.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		ServiceExtension()(init)
		init.AddExtension(appcore.CommandExt(commands.NewMarketplaceCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewSearchCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewInfoCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewInstalledCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewInstallCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewUninstallCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewOutdatedCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewUpdateCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewSelfUpdateCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewStatusCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewInstallInfraCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewRepinProgramsCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewConfigSetCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewSeedCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewKeysCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewVersionsCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewConfigureCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewDoctorCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewUniverseCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewProjectCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewSolutionCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewVaultCommand()))
		init.AddExtension(appcore.CommandExt(commands.NewPrereqsCommand()))
	}
}
