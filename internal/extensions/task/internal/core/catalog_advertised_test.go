// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

func boolPtr(b bool) *bool { return &b }

// Visibility ≠ callability: an `advertised: false` task is hidden from the
// list surface (ListTasks → TaskSummariesFromCatalog) but stays resolvable by
// exact name/URI via GetTask. A bulk enumeration that needs the full set uses
// TaskSummariesFromCatalogAll. Mirrors the skills advertised invariant.
func TestAdvertised_HiddenFromListButCallableByName(t *testing.T) {
	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-x": {
			{Name: "headline", Type: "agent", Description: "top-level"},
			{Name: "worker", Type: "agent", Description: "sub-task", Advertised: boolPtr(false)},
			{Name: "explicit-on", Type: "agent", Advertised: boolPtr(true)},
		},
	})
	ctx := context.Background()

	// Advertised-only list: worker is excluded; the other two survive.
	summaries, err := taskapi.TaskSummariesFromCatalog(ctx, cat)
	if err != nil {
		t.Fatalf("TaskSummariesFromCatalog: %v", err)
	}
	names := map[string]bool{}
	for _, s := range summaries {
		names[s.Name] = true
	}
	if names["worker"] {
		t.Error("advertised:false 'worker' should NOT appear in the advertised-only list")
	}
	if !names["headline"] || !names["explicit-on"] {
		t.Errorf("advertised tasks missing from list: %+v", names)
	}

	// All-set: worker IS present, carrying its real flag.
	all, err := taskapi.TaskSummariesFromCatalogAll(ctx, cat)
	if err != nil {
		t.Fatalf("TaskSummariesFromCatalogAll: %v", err)
	}
	var worker *taskapi.TaskSummary
	for i := range all {
		if all[i].Name == "worker" {
			worker = &all[i]
		}
	}
	if worker == nil {
		t.Fatal("advertised:false 'worker' should appear in the all-set")
	}
	if worker.Advertised == nil || *worker.Advertised != false {
		t.Errorf("worker.Advertised = %v, want explicit false", worker.Advertised)
	}

	// Callability: GetTask resolves the hidden worker by exact name + URI.
	lib := NewCatalogTaskLibrary(cat)
	if _, err := lib.GetTask(ctx, taskapi.GetTaskRequest{Name: "worker"}); err != nil {
		t.Errorf("GetTask(worker) by bare name should succeed despite advertised:false: %v", err)
	}
	if _, err := lib.GetTask(ctx, taskapi.GetTaskRequest{Name: "ktw-x:worker"}); err != nil {
		t.Errorf("GetTask(ktw-x:worker) by URI should succeed despite advertised:false: %v", err)
	}
}
