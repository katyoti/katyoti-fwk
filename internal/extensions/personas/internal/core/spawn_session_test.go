// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// capturingTasks records the run a spawn asks for.
type capturingTasks struct {
	taskapi.TaskService
	got []taskapi.TaskRunRequest
}

func (c *capturingTasks) Run(_ stdctx.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	c.got = append(c.got, in)
	return taskapi.TaskRunResponse{RunID: "run-1", Status: taskapi.RunStatusRunning}, nil
}

// A spawn given a session runs in it — a request's work, persona or task,
// shares one session.
func TestSpawn_JoinsTheSessionItIsGiven(t *testing.T) {
	raw, _ := json.Marshal(reviewer())
	app := application.New(application.Config())
	application.PutService[mbapi.Catalog](app, "default", &stubCatalog{entries: []contribapi.Contribution{{
		Provider: "test-sidecar", Type: mbapi.ResourceTypePersona, ID: "reviewer", Layers: contribapi.InlineData(raw),
	}}})
	tasks := &capturingTasks{}
	application.PutService[taskapi.TaskService](app, "default", tasks)
	ctx := application.WithApplication(stdctx.Background(), app)

	out, err := New().Spawn(ctx, api.SpawnInput{Name: "reviewer", Target: "billing.RQ-001", SessionID: "s-42_request"})
	if err != nil || out.TaskID != "run-1" {
		t.Fatalf("Spawn = %+v, %v", out, err)
	}
	if len(tasks.got) != 1 || tasks.got[0].SessionID != "s-42_request" {
		t.Fatalf("run = %+v; want it in session s-42_request", tasks.got)
	}
}

// A spawn passes its group, priority, origin and target to the run: a
// persona's run is governed by its task group like any run.
func TestSpawn_RunsInTheGroupItIsGiven(t *testing.T) {
	raw, _ := json.Marshal(reviewer())
	app := application.New(application.Config())
	application.PutService[mbapi.Catalog](app, "default", &stubCatalog{entries: []contribapi.Contribution{{
		Provider: "test-sidecar", Type: mbapi.ResourceTypePersona, ID: "reviewer", Layers: contribapi.InlineData(raw),
	}}})
	tasks := &capturingTasks{}
	application.PutService[taskapi.TaskService](app, "default", tasks)
	ctx := application.WithApplication(stdctx.Background(), app)

	origin := &taskapi.RunOrigin{Kind: taskapi.OriginPlanner, ID: "billing.RQ-001"}
	if _, err := New().Spawn(ctx, api.SpawnInput{Name: "reviewer", Target: "billing.RQ-001", Group: "planning", Priority: 3, Origin: origin}); err != nil {
		t.Fatal(err)
	}
	got := tasks.got[0]
	if got.Group != "planning" || got.Priority != 3 || got.Origin == nil || *got.Origin != *origin || got.Target != "billing.RQ-001" {
		t.Fatalf("run = group %q priority %d origin %+v target %q; want planning, 3, the planner, billing.RQ-001", got.Group, got.Priority, got.Origin, got.Target)
	}
}
