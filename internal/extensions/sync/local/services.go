// SPDX-License-Identifier: Apache-2.0

// Package local provides the in-process sync services as PublishedServices.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"
	synccmds "github.com/katyoti/katyoti-fwk/internal/extensions/sync/commands"
	synccore "github.com/katyoti/katyoti-fwk/internal/extensions/sync/internal/core"
)

// Extension registers all sync services and their internal dependencies.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		// Internal DI services
		init.AddExtension(synccore.WorkspaceLibraryExtension)
		init.AddExtension(synccore.JobLibraryExtension)

		// SyncJobService — ktw-sync job list/create/run/...
		init.AddExtension(appcore.PublishService[syncapi.SyncJobService, synccore.SyncServiceConfig](
			newJobService,
			appcore.WithServiceDef(syncapi.SyncJobServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithHTTPServer(8894),
			appcore.WithConfigDirs("config", "sync", "sync/workspaces", "sync/jobs"),
			appcore.WithTools(synccmds.ToolDefs()...),
		))

		// SyncWorkspaceService — ktw-sync workspace list/create/...
		init.AddExtension(appcore.PublishSimpleService[syncapi.SyncWorkspaceService](
			newWorkspaceService,
			appcore.WithServiceDef(syncapi.SyncWorkspaceServiceDef),
			appcore.WithKTSPContainer(),
		))

		// SyncProviderService — ktw-sync provider list/authorize
		init.AddExtension(appcore.PublishSimpleService[syncapi.SyncProviderService](
			newProviderService,
			appcore.WithServiceDef(syncapi.SyncProviderServiceDef),
			appcore.WithKTSPContainer(),
		))
	}
}

func newJobService(ctx context.Context, cfg *synccore.SyncServiceConfig) (syncapi.SyncJobService, error) {
	workspaces, err := application.UseService[*synccore.WorkspaceLibrary](ctx)
	if err != nil {
		return nil, err
	}
	jobs, err := application.UseService[*synccore.JobLibrary](ctx)
	if err != nil {
		return nil, err
	}
	return synccore.NewJobService(cfg, workspaces, jobs)
}

func newWorkspaceService(ctx context.Context) (syncapi.SyncWorkspaceService, error) {
	workspaces, err := application.UseService[*synccore.WorkspaceLibrary](ctx)
	if err != nil {
		return nil, err
	}
	return synccore.NewWorkspaceService(workspaces)
}

func newProviderService(ctx context.Context) (syncapi.SyncProviderService, error) {
	return synccore.NewProviderService()
}
