// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"
)

// workspaceService implements syncapi.SyncWorkspaceService.
type workspaceService struct {
	workspaces *WorkspaceLibrary
}

func (s *workspaceService) List(_ context.Context) (syncapi.ListWorkspacesOutput, error) {
	return s.workspaces.List()
}

func (s *workspaceService) Create(_ context.Context, in syncapi.CreateWorkspaceInput) (syncapi.CreateWorkspaceOutput, error) {
	return s.workspaces.Create(in)
}

func (s *workspaceService) Get(_ context.Context, in syncapi.GetWorkspaceInput) (syncapi.GetWorkspaceOutput, error) {
	return s.workspaces.Get(in.Name)
}

func (s *workspaceService) Delete(_ context.Context, in syncapi.DeleteWorkspaceInput) (syncapi.DeleteWorkspaceOutput, error) {
	return s.workspaces.Delete(in.Name)
}
