// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/remote"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/remote/internal/genspec"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// TestGeneratedClientIsCurrent refuses a client generated from an older
// TaskService: a method added to the interface fails the build, but one
// removed or re-typed would linger here unnoticed. Fix: go generate
// ./internal/extensions/task/remote/
func TestGeneratedClientIsCurrent(t *testing.T) {
	want, err := busclient.Generate(genspec.Spec())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(genspec.FileName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Fatalf("%s is stale — run: go generate ./internal/extensions/task/remote/", genspec.FileName)
	}
}

// hostTasks is the task service on the host: the runs it was asked to start.
type hostTasks struct {
	taskapi.TaskService
	started []taskapi.TaskRunRequest
}

func (h *hostTasks) Run(_ context.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	h.started = append(h.started, in)
	return taskapi.TaskRunResponse{RunID: "r1", TaskName: in.TaskName, Status: taskapi.RunStatusRunning}, nil
}

func (h *hostTasks) RunStages(_ context.Context, in taskapi.RunStagesRequest) (taskapi.RunStagesResponse, error) {
	if in.Run != "r1" {
		return taskapi.RunStagesResponse{}, errors.New("no run " + in.Run)
	}
	return taskapi.RunStagesResponse{Stages: []taskapi.RunStage{{ID: "asis-sources", Label: "asis-sources", State: "running", Total: 1}}}, nil
}

// : a consumer's TaskService is the bus client - a run it starts
// is started on the host, the run's stages come back, a host error comes
// back as an error, and the host-local managers are not faked.
func TestClientRoundTripsToTheHost(t *testing.T) {
	const tenant, universe = "local", "test"
	bus := teststub.New()
	t.Cleanup(func() { _ = bus.Drain(context.Background()) })

	host := &hostTasks{}
	def := taskapi.TaskServiceDef
	endpoint := ktsp.NewEndpoint(remote.Host, "task host", "test")
	endpoint.AddSlot(ktslocal.BuildKTSPSlotWithTags(def.ResolvedBusName(), ktsp.DefaultInstance, def.Short, taskapi.TaskService(host), nil, def.Methods))
	group, err := messagebus.Subscribe(context.Background(), bus, tenant, universe, remote.Host, endpoint)
	if err != nil {
		t.Fatalf("host subscribe: %v", err)
	}
	t.Cleanup(func() { _ = group.Unsubscribe() })

	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		remote.RemoteExtension(),
		busclientbus.Extension(),
		application.DeclareSimpleService[mbapi.MessageBus](func(context.Context) (mbapi.MessageBus, error) { return bus, nil }),
	)
	app.SetProvider("ktw-consumer")
	if err := app.Run(); err != nil {
		t.Fatalf("consumer app.Run: %v", err)
	}
	ctx := messagebus.WithScope(application.WithApplication(context.Background(), app), tenant, universe)

	tasks, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	run, err := tasks.Run(ctx, taskapi.TaskRunRequest{TaskName: "asis-study-run", Input: map[string]any{"dossier_id": "etude-x"}, Target: "etude-x"})
	if err != nil || run.RunID != "r1" {
		t.Fatalf("Run over the bus: %v %+v", err, run)
	}
	if len(host.started) != 1 || host.started[0].TaskName != "asis-study-run" || host.started[0].Input["dossier_id"] != "etude-x" {
		t.Fatalf("the run is not started on the host: %+v", host.started)
	}
	st, err := tasks.RunStages(ctx, taskapi.RunStagesRequest{Run: "r1"})
	if err != nil || len(st.Stages) != 1 || st.Stages[0].State != "running" {
		t.Fatalf("RunStages over the bus: %v %+v", err, st)
	}
	if _, err := tasks.RunStages(ctx, taskapi.RunStagesRequest{Run: "nothing"}); err == nil {
		t.Fatal("a host error did not cross the bus")
	}
	if tasks.BackgroundManager() != nil || tasks.SuspensionManager() != nil {
		t.Error("a host-local manager is faked over the bus")
	}
}
