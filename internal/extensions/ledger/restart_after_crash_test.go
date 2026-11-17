// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"context"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A ledger running when its process died starts again from its last state:
// applying a patch and emitting a snapshot are safe to do again.
func TestALedgerRestartsAfterACrash(t *testing.T) {
	m, err := (&ledgerFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{Config: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !engine.RestartsAfterCrash(m) {
		t.Error("the ledger is not restarted after a crash")
	}
}
