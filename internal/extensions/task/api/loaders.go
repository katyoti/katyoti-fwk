// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// TaskLoader bundles a (Loader, Root, Provider) triple for the task-discovery
// walk (taskcontrib.DiscoverTasksFrom): the folders the catalog does not
// carry, the task service's ExtraDirs. The tasks a binary ships are
// contributed as providers (taskcontrib.ContributeTaskResources) and run from
// the catalog, never walked.
type TaskLoader struct {
	Loader   loader.Loader
	Root     string // root path within the loader (e.g., "embed/tasks")
	Provider string // sidecar/binary name that hosts these tasks
}
