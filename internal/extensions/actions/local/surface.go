// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	actionscore "github.com/katyoti/katyoti-fwk/internal/extensions/actions/internal/core"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// SurfaceExtension publishes the action surface (`<provider>:action-surface/List`)
// over the catalog's tasks. It needs a CWMService in the binary.
func SurfaceExtension() application.Extension {
	return SurfaceExtensionWith(catalogTasks)
}

// SurfaceExtensionWith publishes the action surface over the given tasks.
func SurfaceExtensionWith(tasks actionsapi.TaskActionSource) application.Extension {
	return publish.PublishSimpleService[actionsapi.SurfaceService](
		func(context.Context) (actionsapi.SurfaceService, error) { return actionscore.NewSurface(tasks), nil },
		publish.WithServiceDef(actionsapi.SurfaceServiceDef),
		publish.WithKTSPContainer(),
	)
}

// catalogTasks is every task of the universe's catalog that offers itself as
// an action (`kind: object|collection|global`); a task with no kind does not.
// Read live per list, so a task deployed later is offered at once.
func catalogTasks(ctx context.Context) []actionsapi.TaskAction {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return nil
	}
	summaries, err := taskapi.TaskSummariesFromCatalog(ctx, cat)
	if err != nil {
		return nil
	}
	out := make([]actionsapi.TaskAction, 0, len(summaries))
	for _, t := range summaries {
		if t.Kind == "" {
			continue
		}
		label := t.Title
		if label == "" {
			label = t.Name
		}
		icon := t.Icon
		if icon == "" {
			icon = t.IconName
		}
		out = append(out, actionsapi.TaskAction{
			Provider: t.Provider, Name: t.Name, Label: label, Icon: icon,
			Description: t.Description, Kind: t.Kind, Types: t.Types, Filter: t.Filter,
			SessionMode: t.SessionMode,
		})
	}
	return out
}
