// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// --- Workspace types ---

type CreateWorkspaceInput struct {
	Name string `json:"name" desc:"Workspace name (used as file name)" required:"true"`
	Type string `json:"type" desc:"Provider type (local, drive, s3, sftp, onedrive, webdav, ...)" required:"true"`
}

type CreateWorkspaceOutput struct {
	Name     string `json:"name"     desc:"Created workspace name"`
	FilePath string `json:"filePath" desc:"Path to the workspace config file"`
}

type ListWorkspacesOutput struct {
	Workspaces []WorkspaceInfo `json:"workspaces" desc:"Configured workspaces"`
}

type WorkspaceInfo struct {
	Name string `json:"name" desc:"Workspace name"`
	Type string `json:"type" desc:"Provider type"`
	Path string `json:"path" desc:"Path or remote location"`
}

type GetWorkspaceInput struct {
	Name string `json:"name" desc:"Workspace name" required:"true"`
}

type GetWorkspaceOutput struct {
	Name   string         `json:"name"   desc:"Workspace name"`
	Type   string         `json:"type"   desc:"Provider type"`
	Config map[string]any `json:"config" desc:"Full workspace configuration"`
}

type DeleteWorkspaceInput struct {
	Name string `json:"name" desc:"Workspace name" required:"true"`
}

type DeleteWorkspaceOutput struct {
	Name    string `json:"name"    desc:"Deleted workspace name"`
	Deleted bool   `json:"deleted" desc:"Whether the workspace was deleted"`
}

// --- Job types ---

type CreateJobInput struct {
	Name   string `json:"name"   desc:"Job name" required:"true"`
	Source string `json:"source" desc:"Source workspace name" required:"true"`
	Dest   string `json:"dest"   desc:"Destination workspace name" required:"true"`
	Mode   string `json:"mode"   desc:"Sync mode: push, pull, mirror, bisync" required:"true"`
}

type CreateJobOutput struct {
	Name     string `json:"name"     desc:"Created job name"`
	FilePath string `json:"filePath" desc:"Path to the job config file"`
}

type ListJobsOutput struct {
	Jobs []JobInfo `json:"jobs" desc:"Configured sync jobs"`
}

type JobInfo struct {
	Name     string `json:"name"     desc:"Job name"`
	Source   string `json:"source"   desc:"Source workspace"`
	Dest     string `json:"dest"     desc:"Destination workspace"`
	Mode     string `json:"mode"     desc:"Sync mode"`
	Enabled  bool   `json:"enabled"  desc:"Whether the job is enabled"`
	Schedule string `json:"schedule" desc:"Cron schedule (empty = manual only)"`
}

type GetJobInput struct {
	Name string `json:"name" desc:"Job name" required:"true"`
}

type GetJobOutput struct {
	Name   string         `json:"name"   desc:"Job name"`
	Config map[string]any `json:"config" desc:"Full job configuration"`
}

type UpdateJobInput struct {
	Name    string `json:"name"              desc:"Job name" required:"true"`
	Enabled *bool  `json:"enabled,omitempty" desc:"Enable or disable the job"`
	Mode    string `json:"mode,omitempty"    desc:"New sync mode"`
}

type UpdateJobOutput struct {
	Name    string `json:"name"    desc:"Updated job name"`
	Updated bool   `json:"updated" desc:"Whether the job was updated"`
}

type DeleteJobInput struct {
	Name string `json:"name" desc:"Job name" required:"true"`
}

type DeleteJobOutput struct {
	Name    string `json:"name"    desc:"Deleted job name"`
	Deleted bool   `json:"deleted" desc:"Whether the job was deleted"`
}

// --- Execution types ---

type RunInput struct {
	Name   string `json:"name,omitempty" desc:"Job name to run (empty = --all)"`
	All    bool   `json:"all,omitempty"  desc:"Run all enabled jobs"`
	DryRun bool   `json:"dryRun"        desc:"Simulate without making changes"`
}

type RunOutput struct {
	Jobs []JobRunResult `json:"jobs" desc:"Results for each job run"`
}

type JobRunResult struct {
	Name        string        `json:"name"       desc:"Job name"`
	Status      string        `json:"status"     desc:"Result status (ok, error, skipped, dry-run)"`
	Transferred int           `json:"transferred" desc:"Files transferred"`
	Deleted     int           `json:"deleted"    desc:"Files deleted"`
	Errors      int           `json:"errors"     desc:"Transfer errors"`
	Duration    time.Duration `json:"duration"   desc:"Sync duration"`
	Error       string        `json:"error,omitempty" desc:"Error message if failed"`
}

type StatusInput struct {
	Name string `json:"name" desc:"Job name" required:"true"`
}

type StatusOutput struct {
	Name       string        `json:"name"       desc:"Job name"`
	LastRun    string        `json:"lastRun"    desc:"Last run timestamp"`
	LastStatus string        `json:"lastStatus" desc:"Last run result"`
	Changes    []ChangeEntry `json:"changes"    desc:"Pending changes (dry-run diff)"`
}

type ChangeEntry struct {
	Action string `json:"action" desc:"Change action (copy, delete, move)"`
	Path   string `json:"path"   desc:"File path"`
	Size   int64  `json:"size"   desc:"File size in bytes"`
}

// --- RunFromFile ---

type RunFromFileInput struct {
	JobDef string `json:"jobdef" desc:"Path to a source config file (workspace + job sections)" required:"true"`
	Dest   string `json:"dest"   desc:"Destination directory (local path)" required:"true"`
	DryRun bool   `json:"dryRun" desc:"Simulate without making changes"`
}

// --- Login & discovery ---

type LoginInput struct {
	Backend string `json:"backend" desc:"Provider backend type (drive, s3, onedrive, ...)" required:"true"`
}

type LoginOutput struct {
	Status  string `json:"status"  desc:"Login result (ok, error)"`
	Token   string `json:"token"   desc:"OAuth token JSON (paste into source config workspace.token)"`
	Message string `json:"message" desc:"Result message"`
}

type ListProvidersInput struct {
	Query string `json:"query,omitempty" desc:"Fuzzy search filter (matches name and description)"`
}

type ListProvidersOutput struct {
	Providers []ProviderInfo `json:"providers" desc:"Available storage providers"`
}

type ProviderInfo struct {
	Name        string `json:"name"        desc:"Provider name (rclone backend name)"`
	Description string `json:"description" desc:"Provider description (max 40 chars in CLI)"`
}
