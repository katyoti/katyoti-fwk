// SPDX-License-Identifier: Apache-2.0

package catalog

// Moved here from task/local (e24): task/local is under the projections
// import guard (TestProjectionsConsumeThroughMbapi, tests included), and this
// test needs manifestlocal's in-memory catalog as its fixture.

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	tasklocal "github.com/katyoti/katyoti-fwk/internal/extensions/task/local"
)

// TestCatalogTaskLibrary_AutoFreshAfterCatalogPut proves the TaskLibrary needs
// NO refresh wiring of its own: it re-reads the catalog on every ListTasks, so
// once the event refresher swaps/patches the underlying catalog, the new task is
// visible immediately. Part of contribution-event-incremental-refresh (ST-6).
func TestCatalogTaskLibrary_AutoFreshAfterCatalogPut(t *testing.T) {
	cat := manifestlocal.NewMemoryCatalog(nil, nil)
	lib := tasklocal.NewCatalogTaskLibrary(cat)
	ctx := context.Background()

	resp, err := lib.ListTasks(ctx, taskapi.ListTasksRequest{})
	if err != nil {
		t.Fatalf("ListTasks (empty) err: %v", err)
	}
	if len(resp.Tasks) != 0 {
		t.Fatalf("expected 0 tasks before Put, got %d", len(resp.Tasks))
	}

	body, _ := json.Marshal(mbapi.TaskBody{Name: "my-task", Type: "agent", Description: "hot-added"})
	cat.Put("ktw-x", &mbapi.Manifest{
		Provider: "ktw-x",
		Contributions: []contribapi.Contribution{{
			Provider: "ktw-x", Type: mbapi.ResourceTypeTask, ID: "my-task", Layers: contribapi.InlineData(body),
		}},
	})

	resp, err = lib.ListTasks(ctx, taskapi.ListTasksRequest{})
	if err != nil {
		t.Fatalf("ListTasks (after Put) err: %v", err)
	}
	if len(resp.Tasks) != 1 {
		t.Fatalf("expected 1 task after catalog Put, got %d — TaskLibrary is not re-reading the catalog", len(resp.Tasks))
	}
}
