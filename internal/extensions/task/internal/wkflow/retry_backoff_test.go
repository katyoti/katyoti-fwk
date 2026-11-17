// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A retry waits before it runs again, longer each time, at most its cap, give
// or take a fifth — it ran again at once.
func TestARetryWaitsLongerEachTime(t *testing.T) {
	def := &OnErrorDef{BackoffMs: 100, MaxBackoffMs: 300}
	for n, want := range map[int]time.Duration{1: 100 * time.Millisecond, 2: 200 * time.Millisecond, 3: 300 * time.Millisecond, 6: 300 * time.Millisecond} {
		for i := 0; i < 20; i++ {
			got := retryBackoff(def, n)
			if got < want*8/10 || got > want*12/10 {
				t.Fatalf("retry %d waits %s; want %s ± 20 %%", n, got, want)
			}
		}
	}
	if d := retryBackoff(&OnErrorDef{}, 3); d != 0 {
		t.Errorf("no backoff declared: waits %s; want at once", d)
	}
}

// The wait before a retry is a timer the run waits on as a machine (no slot,
// saved), then the step runs again: a call failing twice succeeds on its
// third attempt after its two waits.
func TestARetryWaitsAsAMachineThenRunsAgain(t *testing.T) {
	var calls atomic.Int32
	withStubActions(t, stubActions{run: func(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
		if calls.Add(1) < 3 {
			return actionsapi.RunOutput{}, fmt.Errorf("transient boom")
		}
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`"recovered"`)}, nil
	}})
	start := time.Now()
	output, _ := runCallWorkflow(t, []any{
		map[string]any{
			"name": "c", "kind": "call", "uri": "ktw-x:svc/m",
			"on_error": map[string]any{"strategy": "retry", "max_retries": 3, "backoff_ms": 100},
		},
		map[string]any{"kind": "return", "value": "${results}"},
	})
	if om := output.(map[string]any); om["c"] != "recovered" {
		t.Fatalf("results = %#v; want recovered on the third attempt", om)
	}
	if took := time.Since(start); took < 240*time.Millisecond {
		t.Errorf("took %s; want two waits (100 ms then 200 ms, ± 20 %%)", took)
	}

	// The wait itself: a timer, no condition.
	node := &Node{OnError: &OnErrorDef{Strategy: OnErrorRetry, MaxRetries: 2, BackoffMs: 50}}
	ws := &WorkflowState{Active: []ActivationFrame{{NodeID: "n", Local: map[string]any{}}}}
	res, err := (&workflowMachine{}).handleNodeError(ws, &ws.Active[0], node, "boom")
	if err != nil {
		t.Fatal(err)
	}
	if res.Wait == nil || len(res.Wait.Conditions) != 0 || res.Wait.Timeout <= 0 || ws.Active[0].Phase != retryWaitPhase {
		t.Errorf("retry = %+v, phase %q; want a timer wait in the retry-wait phase", res.Wait, ws.Active[0].Phase)
	}
	_ = machine.WaitAny
}
