// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// TestTaskServiceList_IsTheCatalogOnly is the TaskService row of
// modules/contributions/projections.md (e24 / N1): discovery is the catalog
// only, and `TaskServiceConfig.ExtraDirs` is execution-only. A task present
// only in ExtraDirs runs when named and is absent from List; a task the
// catalog carries is listed although no loader of this process holds it.
func TestTaskServiceList_IsTheCatalogOnly(t *testing.T) {
	registerSimulatorForCore(t)

	dir := t.TempDir()
	taskDir := filepath.Join(dir, "extra-only")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: extra-only\ntype: simulator\ndescription: present only in ExtraDirs\nconfig:\n  duration_ms: 0\n  output:\n    summary: ran\n---\n"
	if err := os.WriteFile(filepath.Join(taskDir, "TASK.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	app := application.New(application.Config())
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	application.PutService[mbapi.Catalog](app, "default", buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-x": {{Name: "catalog-task", Type: "simulator", Description: "carried by the catalog"}},
	}))
	ctx := application.WithApplication(context.Background(), app)

	ts, err := NewTaskService(ctx, &TaskServiceConfig{ExtraDirs: []string{dir}})
	if err != nil {
		t.Fatalf("NewTaskService: %v", err)
	}
	svc := ts.(*Service)
	defer svc.Close()

	// Discovery: the catalog's task, and nothing from ExtraDirs.
	resp, err := svc.List(ctx, taskapi.ListTasksRequest{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var listed []string
	sawCatalog := false
	for _, s := range resp.Tasks {
		listed = append(listed, s.URI)
		if s.Name == "extra-only" {
			t.Errorf("List returned the ExtraDirs-only task %q — ExtraDirs became discovery", s.URI)
		}
		if s.URI == "ktw-x:catalog-task" {
			sawCatalog = true
		}
	}
	if !sawCatalog {
		t.Errorf("List = %v, want the catalog's ktw-x:catalog-task", listed)
	}

	// Execution: the ExtraDirs-only task still runs by name.
	run, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "extra-only"})
	if err != nil {
		t.Fatalf("Run(extra-only): %v — ExtraDirs must stay executable", err)
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := svc.Result(rctx, taskapi.ResultRequest{RunID: run.RunID})
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if res.Status != string(taskapi.RunStatusCompleted) {
		t.Errorf("extra-only run status = %q (error %q), want completed", res.Status, res.Error)
	}
}
