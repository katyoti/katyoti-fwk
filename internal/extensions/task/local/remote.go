// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/core"
)

// NewCatalogTaskLibrary creates a TaskLibrary backed by an
// `mbapi.Catalog` (in-memory or disk). Replaced the per-sidecar
// `RemoteTaskLibrary` fan-out (now deleted) with a single in-memory
// walk. Per `bus-split-aggregator-to-catalog` step 9.
var NewCatalogTaskLibrary = core.NewCatalogTaskLibrary

// NewCompositeTaskLibrary creates a TaskLibrary that queries multiple sources.
// Re-exported from internal/core for use by bootstrap wiring.
var NewCompositeTaskLibrary = core.NewCompositeTaskLibrary

// WrapTaskLibraryWithRemotes replaces the TaskService's discovery library
// with a composite of catalog projections only (projections.md, TaskLibrary
// row):
//   - the given catalog libraries (ktw-agents passes one CatalogTaskLibrary
//     over the universe view);
//   - the skill-task library (skills with `provides: [task]`, read from the
//     same catalog on every call).
//
// With no remotes the TaskService keeps its default, the catalog the
// application publishes (self view without a universe). Never a loader walk:
// the provider loaders and `TaskServiceConfig.ExtraDirs` are the EXECUTION
// map, reached only to run a task by name (e24 / N1).
//
// Must be called at Bootstrap phase (after TaskService's AppInit has run).
// Uses a local interface to avoid coupling to the concrete core.Service type.
func WrapTaskLibraryWithRemotes(app *application.Application, remotes ...taskapi.TaskLibrary) error {
	if len(remotes) == 0 {
		return nil
	}
	taskSvc, err := application.AppUseService[taskapi.TaskService](app)
	if err != nil {
		return err
	}

	sources := make([]taskapi.TaskLibrary, 0, 1+len(remotes))
	sources = append(sources, remotes...)
	sources = append(sources, core.NewSkillTaskLibrary())
	composite := core.NewCompositeTaskLibrary(sources...)

	type libraryAware interface {
		SetLibrary(taskapi.TaskLibrary)
	}
	if aware, ok := taskSvc.(libraryAware); ok {
		aware.SetLibrary(composite)
	}
	return nil
}
