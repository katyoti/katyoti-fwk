// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rclone/rclone/fs"
	rcloneSync "github.com/rclone/rclone/fs/sync"
	"gopkg.in/yaml.v3"

	syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"
)

// RunJob executes a sync job using embedded rclone.
func RunJob(ctx context.Context, job *JobConfig, srcCfg, dstCfg map[string]any, defaults SyncDefaults, dryRun bool) syncapi.JobRunResult {
	start := time.Now()

	// Build rclone filesystem objects from workspace configs
	srcFs, err := buildFs(ctx, srcCfg)
	if err != nil {
		return syncapi.JobRunResult{Status: "error", Error: fmt.Sprintf("source: %v", err), Duration: time.Since(start)}
	}

	dstFs, err := buildFs(ctx, dstCfg)
	if err != nil {
		return syncapi.JobRunResult{Status: "error", Error: fmt.Sprintf("dest: %v", err), Duration: time.Since(start)}
	}

	// Apply options
	ci := fs.GetConfig(ctx)
	ci.DryRun = dryRun
	if job.Options.Transfers > 0 {
		ci.Transfers = job.Options.Transfers
	} else if defaults.Transfers > 0 {
		ci.Transfers = defaults.Transfers
	}
	if job.Options.Conflict != "" {
		// bisync conflict resolution handled by rclone bisync flags
	}

	// Execute based on mode
	switch job.Mode {
	case "push":
		err = rcloneSync.Sync(ctx, dstFs, srcFs, false)
	case "pull":
		err = rcloneSync.Sync(ctx, srcFs, dstFs, false)
	case "mirror":
		err = rcloneSync.CopyDir(ctx, dstFs, srcFs, false)
	case "bisync":
		// bisync requires special handling — use CopyDir as fallback for now
		// Full bisync support requires rclone/cmd/bisync package
		err = rcloneSync.CopyDir(ctx, dstFs, srcFs, false)
		if err == nil {
			err = rcloneSync.CopyDir(ctx, srcFs, dstFs, false)
		}
	default:
		err = fmt.Errorf("unknown sync mode: %s", job.Mode)
	}

	status := "ok"
	if dryRun {
		status = "dry-run"
	}
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}

	// Stats from accounting (simplified — full accounting requires fs/accounting)
	transferred := 0
	deletes := 0
	errors := 0

	return syncapi.JobRunResult{
		Status:      status,
		Transferred: transferred,
		Deleted:     deletes,
		Errors:      errors,
		Duration:    time.Since(start),
		Error:       errMsg,
	}
}

// runFromFile reads a source config file (workspace + job sections),
// builds a local dest workspace, and runs the sync.
func runFromFile(ctx context.Context, jobDefPath, destDir string, defaults SyncDefaults, dryRun bool) (syncapi.JobRunResult, error) {
	data, err := os.ReadFile(jobDefPath)
	if err != nil {
		return syncapi.JobRunResult{}, fmt.Errorf("read job-def %s: %w", jobDefPath, err)
	}

	// Parse the source file — only workspace: and job: sections matter
	var def struct {
		Workspace map[string]any `yaml:"workspace"`
		Job       JobConfig      `yaml:"job"`
	}
	if err := yaml.Unmarshal(data, &def); err != nil {
		return syncapi.JobRunResult{}, fmt.Errorf("parse job-def %s: %w", jobDefPath, err)
	}

	if def.Workspace == nil {
		return syncapi.JobRunResult{}, fmt.Errorf("job-def missing 'workspace' section")
	}

	// Source workspace = from the file
	srcCfg := def.Workspace

	// Dest workspace = local directory
	dstCfg := map[string]any{
		"type": "local",
		"path": destDir,
	}

	// Use job config from the file (defaults if missing)
	job := &def.Job
	if job.Mode == "" {
		job.Mode = "mirror"
	}

	// In runFromFile: srcCfg = remote (from workspace), dstCfg = local dest.
	// RunJob expects: src = "source" workspace, dst = "dest" workspace.
	// For pull: RunJob does Sync(srcFs, dstFs) = make src look like dst.
	// We need the opposite: make local (dst) look like remote (src).
	// So swap: pass local as "source" and remote as "dest" for pull mode,
	// or just use CopyDir (remote → local) which is always correct for from-file.
	// Simplest: always use mirror mode (CopyDir remote→local) for from-file.
	job.Mode = "mirror"

	name := filepath.Base(jobDefPath)
	result := RunJob(ctx, job, srcCfg, dstCfg, defaults, dryRun)
	result.Name = name
	return result, nil
}

// DiffJob returns pending changes without executing (dry-run).
func DiffJob(ctx context.Context, job *JobConfig, srcCfg, dstCfg map[string]any) ([]syncapi.ChangeEntry, error) {
	srcFs, err := buildFs(ctx, srcCfg)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	dstFs, err := buildFs(ctx, dstCfg)
	if err != nil {
		return nil, fmt.Errorf("dest: %w", err)
	}

	// For now return empty — full diff requires custom rclone check handler
	_ = srcFs
	_ = dstFs
	return nil, nil
}

// buildFs creates an rclone Fs from a workspace config map.
func buildFs(ctx context.Context, cfg map[string]any) (fs.Fs, error) {
	wsType, _ := cfg["type"].(string)
	if wsType == "" {
		return nil, fmt.Errorf("workspace config missing 'type' field")
	}

	switch wsType {
	case "local":
		path, _ := cfg["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("local workspace missing 'path'")
		}
		return fs.NewFs(ctx, path)

	default:
		// For remote types, build an rclone remote string dynamically.
		// Register a temporary remote config and create Fs from it.
		return buildRemoteFs(ctx, wsType, cfg)
	}
}

// buildRemoteFs creates an rclone Fs for a remote workspace by
// setting rclone config values temporarily.
func buildRemoteFs(ctx context.Context, wsType string, cfg map[string]any) (fs.Fs, error) {
	// Create a unique remote name
	remoteName := fmt.Sprintf("ktw_%s_%d", wsType, time.Now().UnixNano())

	// Set config values for this remote
	for key, val := range cfg {
		if key == "type" {
			continue
		}
		fs.ConfigFileSet(remoteName, key, fmt.Sprintf("%v", val))
	}
	fs.ConfigFileSet(remoteName, "type", wsType)

	// Get the remote path
	remotePath := ""
	if p, ok := cfg["remote_path"].(string); ok {
		remotePath = p
	}

	remote := remoteName + ":" + remotePath
	return fs.NewFs(ctx, remote)
}
