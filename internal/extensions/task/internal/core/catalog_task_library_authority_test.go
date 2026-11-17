// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// inProcessTaskProvider publishes one task in THIS process's provider point.
// It carries both provider contracts (Get today, Open(id, layer) after e13) so
// the test compiles on either side of that change.
type inProcessTaskProvider struct{}

func (inProcessTaskProvider) Type() string     { return mbapi.ResourceTypeTask }
func (inProcessTaskProvider) Provider() string { return "local-pack" }
func (inProcessTaskProvider) Identity() contribapi.Identity {
	return contribapi.Identity{}
}
func (inProcessTaskProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	body, _ := json.Marshal(mbapi.TaskBody{Name: "provider-only"})
	return []contribapi.Contribution{{
		Provider: "local-pack", Type: mbapi.ResourceTypeTask, ID: "provider-only",
		Layers: contribapi.InlineData(body),
	}}, nil
}
func (inProcessTaskProvider) Open(context.Context, string, string) (io.ReadCloser, contribapi.LayerRef, error) {
	return io.NopCloser(bytes.NewReader(nil)), contribapi.LayerRef{}, nil
}

// TestCatalogTaskLibrary_CatalogIsItsOnlyInput is e17's TaskLibrary row: the
// library has no input but the catalog. A task a provider in this very process
// publishes, but which the catalog does not carry, is neither listed nor
// resolvable — there is no second path to it. (The nil-catalog half is
// TestCatalogTaskLibrary_NilCatalog.)
func TestCatalogTaskLibrary_CatalogIsItsOnlyInput(t *testing.T) {
	// Structural: one field, and it is the catalog.
	typ := reflect.TypeOf(CatalogTaskLibrary{})
	if typ.NumField() != 1 || typ.Field(0).Type != reflect.TypeOf((*mbapi.Catalog)(nil)).Elem() {
		t.Fatalf("CatalogTaskLibrary has fields %v — any input beside the catalog is a second discovery path", fieldsOf(typ))
	}

	ext := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("task.authority_test", ".point", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
		init.AddContribute("task.authority_test", ".provider", func(app *application.Application) error {
			return application.Contribute(app, contribapi.ContributionProvider(inProcessTaskProvider{}))
		})
	}
	app := application.New(application.Config(), ext)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)
	if n := len(application.UseExtensions[contribapi.ContributionProvider](ctx)); n != 1 {
		t.Fatalf("provider point holds %d providers, want the one task provider", n)
	}

	lib := NewCatalogTaskLibrary(buildCatalog(t, nil)) // an empty, non-nil catalog
	resp, err := lib.ListTasks(ctx, taskapi.ListTasksRequest{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(resp.Tasks) != 0 {
		t.Errorf("ListTasks over an empty catalog listed %v — it found tasks outside the catalog", resp.Tasks)
	}
	if _, err := lib.GetTask(ctx, taskapi.GetTaskRequest{Name: "local-pack:provider-only"}); err == nil {
		t.Error("GetTask resolved a task the catalog does not carry")
	}
}

func fieldsOf(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		out = append(out, t.Field(i).Name+" "+t.Field(i).Type.String())
	}
	return out
}
