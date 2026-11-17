// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// appCatalogTaskLibrary is CatalogTaskLibrary over whatever `mbapi.Catalog`
// the application publishes, resolved on every call: the self view
// (manifestlocal.Extension) in a binary without a universe, the universe view
// once UniverseCatalogExtension replaces it. It is TaskService's default
// discovery source, so `tasks list` reaches its own binary's tasks through the
// catalog's self view — never through the provider loaders (e24 / N1).
//
// Resolved lazily, never at construction: the catalog may itself be built
// from providers whose construction must not wait on TaskService.
type appCatalogTaskLibrary struct {
	appCtx context.Context
}

func newAppCatalogTaskLibrary(appCtx context.Context) taskapi.TaskLibrary {
	return &appCatalogTaskLibrary{appCtx: appCtx}
}

func (l *appCatalogTaskLibrary) catalog(ctx context.Context) mbapi.Catalog {
	if cat, err := application.UseService[mbapi.Catalog](ctx); err == nil && cat != nil {
		return cat
	}
	if l.appCtx != nil {
		if cat, err := application.UseService[mbapi.Catalog](l.appCtx); err == nil && cat != nil {
			return cat
		}
	}
	return nil
}

func (l *appCatalogTaskLibrary) ListTasks(ctx context.Context, in taskapi.ListTasksRequest) (taskapi.ListTasksResponse, error) {
	cat := l.catalog(ctx)
	if cat == nil {
		return taskapi.ListTasksResponse{}, nil
	}
	return NewCatalogTaskLibrary(cat).ListTasks(ctx, in)
}

func (l *appCatalogTaskLibrary) GetTask(ctx context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	cat := l.catalog(ctx)
	if cat == nil {
		return taskapi.TaskInfo{}, fmt.Errorf("task %q not found: no catalog published", in.Name)
	}
	return NewCatalogTaskLibrary(cat).GetTask(ctx, in)
}

var _ taskapi.TaskLibrary = (*appCatalogTaskLibrary)(nil)
