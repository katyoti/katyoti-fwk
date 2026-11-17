// SPDX-License-Identifier: Apache-2.0

package humangate

import (
	"context"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A human-gate step starts from the request the task service gives it, its
// question in config `question` as a workflow step declares it. (It only read
// a bare map: no human-gate task could start.)
func TestAHumanGateTaskReadsItsQuestionFromItsConfig(t *testing.T) {
	_, err := (&humanGateFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{
		Config: map[string]any{"question": "Approve the plan?"},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v; want the config's question read", err)
	}
	if _, err := (&humanGateFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{}); err == nil {
		t.Error("no question: want an error")
	}
}
