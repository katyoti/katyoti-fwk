// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// compositeTaskLibrary aggregates multiple TaskLibrary sources (first match
// wins). Every source TaskService composes is a projection of the catalog —
// the loader-backed library that used to lead the chain is gone (e24 / N1).
type compositeTaskLibrary struct {
	sources []taskapi.TaskLibrary
}

// NewCompositeTaskLibrary creates a TaskLibrary that queries multiple sources.
func NewCompositeTaskLibrary(sources ...taskapi.TaskLibrary) taskapi.TaskLibrary {
	return &compositeTaskLibrary{sources: sources}
}

func (c *compositeTaskLibrary) ListTasks(ctx context.Context, in taskapi.ListTasksRequest) (taskapi.ListTasksResponse, error) {
	// Dedupe by URI (provider:name) — different providers may legitimately
	// ship a task with the same bare name (e.g., ktw-agents:hello-world
	// and ktw-demo:hello-world). Without URI dedupe we'd collapse them.
	// Falls back to bare Name when URI is empty (legacy/unattributed).
	seen := make(map[string]bool)
	var all []taskapi.TaskSummary

	for _, src := range c.sources {
		resp, err := src.ListTasks(ctx, in)
		if err != nil {
			continue // skip failing sources
		}
		for _, t := range resp.Tasks {
			key := t.URI
			if key == "" {
				key = t.Name
			}
			if !seen[key] {
				seen[key] = true
				all = append(all, t)
			}
		}
	}

	return taskapi.ListTasksResponse{Tasks: all}, nil
}

func (c *compositeTaskLibrary) GetTask(ctx context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	for _, src := range c.sources {
		info, err := src.GetTask(ctx, in)
		if err == nil {
			return info, nil
		}
	}
	return taskapi.TaskInfo{}, fmt.Errorf("task %q not found in any library", in.Name)
}
