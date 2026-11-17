// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
)

// awaitingComms awaits as a machine, as the comms service does for a parent
// whose loop waits as one.
type awaitingComms struct{ commsapi.Service }

func (awaitingComms) Await(context.Context, commsapi.AwaitRequest) (commsapi.Event, error) {
	return commsapi.Event{}, &agentapi.ErrAwait{Topic: "parent.p.child-event"}
}

// await-child hands the comms service's machine wait to the loop — it does not
// turn it into a text result the model would read as an error.
func TestAwaitChildLetsTheLoopWaitAsAMachine(t *testing.T) {
	p := &orchPlugin{comms: awaitingComms{}}
	res, err := p.handleAwaitChild(context.Background(), json.RawMessage(`{"task_id":"c1"}`))
	var aw *agentapi.ErrAwait
	if !errors.As(err, &aw) || res != nil {
		t.Fatalf("await-child = %v, %v; want the machine wait passed to the loop", res, err)
	}
}
