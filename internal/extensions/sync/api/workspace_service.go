// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// SyncWorkspaceService manages workspace configurations (storage endpoints).
type SyncWorkspaceService interface {
	List(ctx context.Context) (ListWorkspacesOutput, error)
	Create(ctx context.Context, in CreateWorkspaceInput) (CreateWorkspaceOutput, error)
	Get(ctx context.Context, in GetWorkspaceInput) (GetWorkspaceOutput, error)
	Delete(ctx context.Context, in DeleteWorkspaceInput) (DeleteWorkspaceOutput, error)
}

// SyncWorkspaceServiceDef describes the SyncWorkspaceService for KTSP/HTTP discovery.
// CLI commands: ktw-sync workspace list, ktw-sync workspace create, etc.
var SyncWorkspaceServiceDef = meta.ServiceDef{
	Name:    "workspace",
	Short:   "Workspace management",
	Long:    "Create and manage storage endpoint configurations (local, GDrive, S3, SFTP, etc.).",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List":   {Short: "List all configured workspaces", RenderTemplate: listWorkspacesTemplate},
		"Create": meta.Method("Create a new workspace (local or remote storage endpoint)"),
		"Get":    {Short: "Show details of a workspace", RenderTemplate: getWorkspaceTemplate},
		"Delete": meta.Method("Delete a workspace configuration"),
	},
}
