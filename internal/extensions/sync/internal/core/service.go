// SPDX-License-Identifier: Apache-2.0

package core

import syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"

// NewJobService creates a SyncJobService.
func NewJobService(cfg *SyncServiceConfig, workspaces *WorkspaceLibrary, jobs *JobLibrary) (syncapi.SyncJobService, error) {
	return &jobService{cfg: cfg, jobs: jobs, workspaces: workspaces}, nil
}

// NewWorkspaceService creates a SyncWorkspaceService.
func NewWorkspaceService(workspaces *WorkspaceLibrary) (syncapi.SyncWorkspaceService, error) {
	return &workspaceService{workspaces: workspaces}, nil
}

// NewProviderService creates a SyncProviderService.
func NewProviderService() (syncapi.SyncProviderService, error) {
	return &providerService{}, nil
}
