// SPDX-License-Identifier: Apache-2.0

package template

import (
	"context"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A template task starts from the request the task service gives it: its
// TASK.md body is the template, its input the data. (It only read a bare map:
// no template task could start.)
func TestATemplateTaskRendersItsBodyWithItsInput(t *testing.T) {
	m, err := (&templateFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{
		Body: "Hello {{.name}}", Input: map[string]any{"name": "Ada"},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	st := m.InitialState(engine.Event{})
	action, err := m.Step(context.Background(), &st, engine.Event{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := action(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := res.Output.(string); got != "Hello Ada" {
		if mo, ok := res.Output.(map[string]any); !ok || mo["output"] != "Hello Ada" {
			t.Errorf("output = %#v; want the body rendered with the input", res.Output)
		}
	}
}
