// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// startedRuns is a TaskService that records the runs it is asked to start.
type startedRuns struct {
	taskapi.TaskService
	got []taskapi.TaskRunRequest
}

func (s *startedRuns) Run(_ context.Context, req taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	s.got = append(s.got, req)
	return taskapi.TaskRunResponse{RunID: "run-1"}, nil
}

// : a method declared long, run from the action
// surface, starts a method run and answers its run and its panel at once; the
// run calling the method (its context naming the panel) runs it as before;
// Describe says it is asynchronous.
func TestActionService_LongMethodAnswersItsRunAndPanelAtOnce(t *testing.T) {
	app := application.New(application.Config())
	app.SetProvider("ktw-test")
	endpoint := ktsp.NewEndpoint("ktw-test", "", "0")
	endpoint.AddSlot(ktslocal.BuildKTSPSlotWithTags("echo", ktsp.DefaultInstance, "", echoService{}, nil,
		map[string]meta.MethodDef{"SayBack": {Short: "says it back", LongRunning: true}}))
	endpoint.AddSlot(ktslocal.BuildKTSPSlot("loud", ktsp.DefaultInstance, "", echoService{}))
	application.PutService[*ktsp.Endpoint](app, "default", endpoint)
	runs := &startedRuns{}
	application.PutService[taskapi.TaskService](app, "default", taskapi.TaskService(runs))
	ctx := application.WithApplication(context.Background(), app)
	svc, err := New(ctx, &Config{})
	if err != nil {
		t.Fatal(err)
	}

	out, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-test:echo/say-back", Input: json.RawMessage(`{"text":"hello"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.TaskID != "run-1" || !panel.ValidID(out.Panel) || len(out.Value) != 0 {
		t.Fatalf("a long method answers its run and its panel at once, no value: %+v", out)
	}
	if len(runs.got) != 1 {
		t.Fatalf("one method run started, got %d", len(runs.got))
	}
	def, cfg := runs.got[0].TaskDef, runs.got[0].TaskDef["config"].(map[string]any)
	if def["type"] != taskapi.MethodMachine || cfg["uri"] != "ktw-test:echo/say-back" || cfg["panel"] != out.Panel || runs.got[0].Input["text"] != "hello" {
		t.Errorf("the run is the method's, under the answered panel, with its input: %+v", runs.got[0])
	}

	if _, has := cfg["object"]; has || runs.got[0].Target != "" {
		t.Errorf("a long method run on no object names none: %+v", runs.got[0])
	}

	// : run on an object (_target), the run works on it and its
	// panel names it - a kept outcome lands there.
	if _, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-test:echo/say-back", Input: json.RawMessage(`{"text":"on"}`), Context: map[string]any{"_target": "m.D-001"}}); err != nil {
		t.Fatal(err)
	}
	if len(runs.got) != 2 || runs.got[1].Target != "m.D-001" || runs.got[1].TaskDef["config"].(map[string]any)["object"] != "m.D-001" {
		t.Errorf("a long method run on an object carries it: %+v", runs.got[len(runs.got)-1])
	}
	runs.got = runs.got[:1]

	// The method run calling it: run as before, no second run.
	again, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-test:echo/say-back", Input: json.RawMessage(`{"text":"hi"}`), Context: map[string]any{"panel": out.Panel}})
	var said sayOutput
	if err != nil || json.Unmarshal(again.Value, &said) != nil || said.Said != "you said hi" || len(runs.got) != 1 {
		t.Errorf("the run's own call runs the method: %+v %v (runs %d)", again, err, len(runs.got))
	}

	// A method not declared long runs at once, as before.
	if plain, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-test:loud/say-back", Input: json.RawMessage(`{"text":"x"}`)}); err != nil || plain.TaskID != "" || len(plain.Value) == 0 {
		t.Errorf("a method not declared long: %+v %v", plain, err)
	}
	if d, err := svc.Describe(ctx, actionsapi.DescribeInput{URI: "ktw-test:echo/say-back"}); err != nil || !d.IsAsync {
		t.Errorf("a long method is described asynchronous: %+v %v", d, err)
	}
	if d, _ := svc.Describe(ctx, actionsapi.DescribeInput{URI: "ktw-test:loud/say-back"}); d.IsAsync {
		t.Error("a method not declared long is described synchronous")
	}
}
