// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// SyncJobService manages sync jobs: CRUD + execution.
type SyncJobService interface {
	List(ctx context.Context) (ListJobsOutput, error)
	Create(ctx context.Context, in CreateJobInput) (CreateJobOutput, error)
	Get(ctx context.Context, in GetJobInput) (GetJobOutput, error)
	Update(ctx context.Context, in UpdateJobInput) (UpdateJobOutput, error)
	Delete(ctx context.Context, in DeleteJobInput) (DeleteJobOutput, error)
	Run(ctx context.Context, in RunInput) (RunOutput, error)
	RunFromFile(ctx context.Context, in RunFromFileInput) (RunOutput, error)
	Status(ctx context.Context, in StatusInput) (StatusOutput, error)
}

// SyncJobServiceDef describes the SyncJobService for KTSP/HTTP discovery.
// CLI commands: ktw-sync job list, ktw-sync job create, ktw-sync job run, etc.
var SyncJobServiceDef = meta.ServiceDef{
	Name:    "job",
	Short:   "Sync job management",
	Long:    "Create, configure, and run sync jobs between workspaces.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List":        {Short: "List all configured sync jobs", RenderTemplate: listJobsTemplate},
		"Create":      meta.Method("Create a new sync job between two workspaces"),
		"Get":         {Short: "Show details of a sync job", RenderTemplate: getJobTemplate},
		"Update":      meta.Method("Update a sync job configuration"),
		"Delete":      meta.Method("Delete a sync job"),
		"Run":         {Short: "Run a sync job (or all enabled jobs)", RenderTemplate: runResultTemplate},
		"RunFromFile": {Short: "Run sync from an external job definition file", RenderTemplate: runResultTemplate},
		"Status":      {Short: "Show sync status or dry-run diff", RenderTemplate: statusTemplate},
	},
}
