// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

func (s *kbService) Sync(ctx context.Context, in kbapi.SyncInput) (kbapi.SyncOutput, error) {
	if in.Path == "" {
		return kbapi.SyncOutput{}, fmt.Errorf("path is required")
	}

	absPath, err := application.ProjectPath(in.Path)
	if err != nil {
		return kbapi.SyncOutput{}, fmt.Errorf("resolve path: %w", err)
	}

	kbCfg, err := LoadKnowledgeBase(absPath)
	if err != nil {
		return kbapi.SyncOutput{}, err
	}

	// Filter sources if specific ones requested
	sourceNames := kbCfg.Sources
	if len(in.Sources) > 0 {
		sourceNames = in.Sources
	}

	var out kbapi.SyncOutput

	for _, sourceName := range sourceNames {
		srcCfg, err := LoadSource(absPath, sourceName)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("load source %s: %v", sourceName, err))
			continue
		}

		adapter := srcCfg.ResolvedAdapter()
		jobDefPath := filepath.Join(absPath, "sources", sourceName+".yaml")
		destDir := filepath.Join(absPath, sourceName)

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("create dest dir %s: %v", sourceName, err))
			continue
		}

		slog.Debug("syncing source", "source", sourceName, "adapter", adapter)

		var syncArgs []string
		switch adapter {
		case "ktw-sync":
			syncArgs = []string{"job", "run-from-file", "--jobdef", jobDefPath, "--dest", destDir, "--out", "json"}
		default:
			syncArgs = []string{"sync", "--jobdef", jobDefPath, "--dest", destDir, "--out", "json"}
		}

		result, err := callSidecarJSON(ctx, adapter, syncArgs...)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("sync %s: %v", sourceName, err))
			continue
		}

		// Check for job-level errors in the sync response
		if jobErr := extractSyncJobError(result); jobErr != "" {
			out.Errors = append(out.Errors, fmt.Sprintf("sync %s: %s", sourceName, jobErr))
			continue
		}

		out.SourcesSynced++
	}

	return out, nil
}

// syncJobsResponse matches the ktw-sync RunOutput JSON structure.
type syncJobsResponse struct {
	Jobs []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error"`
	} `json:"jobs"`
}

// extractSyncJobError checks the sync response for job-level errors.
func extractSyncJobError(data []byte) string {
	var resp syncJobsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ""
	}
	for _, job := range resp.Jobs {
		if job.Status == "error" && job.Error != "" {
			return job.Error
		}
	}
	return ""
}

// callSidecarJSON calls a sidecar binary and returns the raw JSON output.
func callSidecarJSON(ctx context.Context, binary string, args ...string) ([]byte, error) {
	binPath := resolveSidecarBinary(binary)
	slog.Debug("calling sidecar", "binary", binPath, "args", args)

	cmd := exec.CommandContext(ctx, binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w\n%s", binary, err, string(output))
	}

	return output, nil
}

// resolveSidecarBinary finds a sidecar binary: first in the same directory
// as the current executable (a development bin/), then the one a marketplace
// installed (an installation keeps each program in its own store folder, and
// its folders are not on PATH), then in PATH.
func resolveSidecarBinary(name string) string {
	self, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(self)
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate = filepath.Join(dir, name+".exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if installed := catalog.InstalledBinary(name); installed != "" {
		return installed
	}
	return name
}
