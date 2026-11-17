// SPDX-License-Identifier: Apache-2.0

package toolproxy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// longAction answers as the action surface does for a method declared long:
// its run and its panel at once, no value.
type longAction struct{}

func (longAction) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (longAction) Run(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	return actionsapi.RunOutput{URI: in.URI, TaskID: "run-1", Panel: "pnl-1"}, nil
}

// fakeRuns answers the method run's result as the TaskService does once it
// ended.
type fakeRuns struct {
	res     taskapi.ResultResponse
	err     error
	stopped []string
}

func (f *fakeRuns) Result(ctx context.Context, in taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	if f.err != nil {
		return taskapi.ResultResponse{}, f.err
	}
	f.res.RunID = in.RunID
	return f.res, nil
}

func (f *fakeRuns) Stop(_ context.Context, in taskapi.StopRequest) (taskapi.StopResponse, error) {
	f.stopped = append(f.stopped, in.RunID)
	return taskapi.StopResponse{Stopped: true}, nil
}

func longProxy(runs runWaiter) *proxyPlugin {
	u, _ := mbapi.ParseToolURI("ktw-docs:docs/render")
	return &proxyPlugin{action: longAction{}, byName: map[string]mbapi.ToolURI{"render": u}, runs: runs}
}

// AN AGENT'S TOOL CALL OF A LONG METHOD ANSWERS ITS VALUE: the
// action surface answers a long method's run and panel at once; the tool call
// waits for that run and gives the agent the method's value, never « task
// started ».
func TestAToolCallOfALongMethodAnswersTheMethodsValue(t *testing.T) {
	runs := &fakeRuns{res: taskapi.ResultResponse{
		Status: string(taskapi.RunStatusCompleted),
		// The method run's output as it answers in process: {panel, value}.
		Output: struct {
			Panel string          `json:"panel"`
			Value json.RawMessage `json:"value"`
		}{"pnl-1", json.RawMessage(`{"outputPath":"out.pdf","format":"pdf"}`)},
	}}
	res, err := longProxy(runs).HandleToolCall(context.Background(), "render", json.RawMessage(`{"input":"a.md","output":"out.pdf"}`))
	if err != nil {
		t.Fatalf("HandleToolCall: %v", err)
	}
	raw, ok := res.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("the tool answered %#v, want the method's value", res.Data)
	}
	var v struct {
		OutputPath string `json:"outputPath"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || v.OutputPath != "out.pdf" {
		t.Fatalf("the tool answered %s, want the method's value {outputPath: out.pdf}", raw)
	}

	// Its kept result decodes as a map: the same value.
	runs.res.Output = map[string]any{"panel": "pnl-1", "value": map[string]any{"outputPath": "kept.pdf"}}
	res, err = longProxy(runs).HandleToolCall(context.Background(), "render", nil)
	if err != nil || !strings.Contains(string(res.Data.(json.RawMessage)), "kept.pdf") {
		t.Fatalf("from a kept result the tool answered %v (%v), want its value", res, err)
	}
}

// A long method that failed or was cancelled is the tool call's error; the
// agent's call ending first stops the run.
func TestAToolCallOfALongMethodFailsWithItsRun(t *testing.T) {
	failed := &fakeRuns{res: taskapi.ResultResponse{Status: string(taskapi.RunStatusFailed), Error: "method ktw-docs:docs/render: pandoc not found"}}
	if _, err := longProxy(failed).HandleToolCall(context.Background(), "render", nil); err == nil || !strings.Contains(err.Error(), "pandoc not found") {
		t.Fatalf("a failed run answered %v, want its error", err)
	}
	cancelled := &fakeRuns{res: taskapi.ResultResponse{Status: string(taskapi.RunStatusCancelled)}}
	if _, err := longProxy(cancelled).HandleToolCall(context.Background(), "render", nil); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("a cancelled run answered %v, want an error saying so", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gone := &fakeRuns{err: context.Canceled}
	if _, err := longProxy(gone).HandleToolCall(ctx, "render", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("an abandoned call answered %v, want its context's end", err)
	}
	if len(gone.stopped) != 1 || gone.stopped[0] != "run-1" {
		t.Fatalf("the abandoned call stopped %v, want its method run", gone.stopped)
	}
}
