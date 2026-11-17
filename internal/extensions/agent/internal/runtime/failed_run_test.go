// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// : a run that ends in error says why - its machine's end
// carries a Failure with the error's message (the task engine records it
// failed), and its result is written with the message, never an empty {}.
func TestARunEndingInErrorIsRecordedFailed(t *testing.T) {
	m := &AgentLoopMachine{hooks: &HookChain{}}
	cause := errors.New("LLM error on turn 9: prompt is too long: 1662333 tokens > 1000000 maximum")
	failed := m.finish("failed", &agentState{StartedAt: time.Now()}, &api.RunResult{Status: api.RunError, Error: cause}, nil, nil)
	if failed.Failure == nil || failed.Failure.Reason != cause.Error() {
		t.Fatalf("a failed end carries %+v; want the error's message as its Failure", failed.Failure)
	}
	raw, err := json.Marshal(failed.Output)
	if err != nil || !strings.Contains(string(raw), `"Error":"LLM error on turn 9: prompt is too long`) {
		t.Fatalf("the result is written as %s (%v); want its error's message", raw, err)
	}
	var back api.RunResult
	if err := json.Unmarshal(raw, &back); err != nil || back.Error == nil || back.Error.Error() != cause.Error() || back.Status != api.RunError {
		t.Fatalf("the written result reads back as %+v (%v)", back, err)
	}

	// Without an error, the summary says why; a completed end has no Failure.
	maxTurns := m.finish("failed", &agentState{StartedAt: time.Now()}, &api.RunResult{Status: api.RunMaxTurns, Summary: "Aborted: exceeded maximum turns (50)"}, nil, nil)
	if maxTurns.Failure == nil || !strings.Contains(maxTurns.Failure.Reason, "maximum turns") {
		t.Fatalf("a run stopped at its turn limit carries %+v", maxTurns.Failure)
	}
	if done := m.finish("complete", &agentState{StartedAt: time.Now()}, &api.RunResult{Status: api.RunCompleted, Summary: "ok"}, nil, nil); done.Failure != nil {
		t.Fatalf("a completed run carries a Failure: %+v", done.Failure)
	}
	if raw, _ := json.Marshal(api.RunResult{Status: api.RunCompleted}); strings.Contains(string(raw), `"Error"`) {
		t.Fatalf("a result without error writes one: %s", raw)
	}
}
