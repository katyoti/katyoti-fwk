// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// TaskLibrary is a published service that provides task definitions.
// Sidecars implement this to expose their embedded tasks.
// ktw-agents aggregates all TaskLibrary sources (local + remote) into a composite.
type TaskLibrary interface {
	// ListTasks returns metadata for all available task definitions.
	ListTasks(ctx context.Context, in ListTasksRequest) (ListTasksResponse, error)

	// GetTask returns the full task definition by name.
	GetTask(ctx context.Context, in GetTaskRequest) (TaskInfo, error)
}

// TaskLibraryDef is the KTSP service definition for auto-discovery.
// Method CLIName overrides collapse the awkward kebab-cased Go names
// (`list-tasks`, `get-task`) to short verbs under the `task` CLI group.
var TaskLibraryDef = meta.ServiceDef{
	Name:  "task-library",
	Short: "Task definition library — lists and serves task definitions",
	Methods: map[string]meta.MethodDef{
		"ListTasks": {
			Short:   "List available task definitions",
			CLIName: "list",
		},
		"GetTask": {
			Short:   "Get task details",
			CLIName: "get",
		},
	},
}
