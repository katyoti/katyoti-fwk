// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"
)

// A task named by a run is the catalog's, so the entry the catalog serves is
// what runs - although the base file is still in this binary's pack,
// contributed by its provider. The catalog here serves what it holds once a
// project's override entry is applied (the override's body under the base's
// provider; the builder applying it is proven by task/contrib's CatalogTask
// tests): a projection reads it through mbapi only.
func TestResolveTask_RunsTheCatalogsOverride(t *testing.T) {
	files := map[string]string{"tasks/greet.md": "---\nname: greet\ntype: agent\ndescription: d\n---\nSay hello.\n"}
	pack := taskcontrib.TaskProviders("ktw-example", loader.NewMapLoader(files), "tasks")

	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-example": {{Name: "greet", Type: "agent", Description: "d", Body: "Say bonjour."}},
	})

	app := application.New(application.Config(), func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
		init.AddContribute("test", ".pack", func(app *application.Application) error {
			for _, p := range pack {
				if err := application.Contribute(app, contribapi.ContributionProvider(p)); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	application.PutService[mbapi.Catalog](app, "default", cat)
	ctx := application.WithApplication(context.Background(), app)

	ts, err := NewTaskService(ctx, &TaskServiceConfig{})
	if err != nil {
		t.Fatalf("NewTaskService: %v", err)
	}
	svc := ts.(*Service)
	defer svc.Close()

	for _, name := range []string{"greet", "ktw-example:greet"} {
		got, err := svc.resolveTask(ctx, taskapi.TaskRunRequest{TaskName: name})
		if err != nil {
			t.Fatalf("resolveTask(%s): %v", name, err)
		}
		if got.Body != "Say bonjour." || got.Provider != "ktw-example" {
			t.Errorf("resolveTask(%s) = %s %q; want the override's body under ktw-example", name, got.URI(), got.Body)
		}
	}
}
