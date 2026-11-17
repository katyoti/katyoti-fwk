// SPDX-License-Identifier: Apache-2.0

package simulator

import (
	"context"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A simulator run working when its process died starts again after the
// restart: its one step is a token-free stand-in, safe to run again.
func TestASimulatorRestartsAfterACrash(t *testing.T) {
	m, err := (&simulatorFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{Config: map[string]any{"output": map[string]any{"ok": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if !engine.RestartsAfterCrash(m) {
		t.Error("the simulator is not restarted after a crash")
	}
}
