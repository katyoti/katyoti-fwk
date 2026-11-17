// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestFinish_StructuredOutputBecomesMachineOutput pins the last hop of the
// output-schema → task.output chain: when the agent state carries a structured
// output envelope (set from the complete tool's full input for an output-schema
// task), finish makes it the machine StepResult.Output — so task_service.go's
// map cast succeeds and task.output is the declared envelope. Schema-less runs
// keep the RunResult struct.
func TestFinish_StructuredOutputBecomesMachineOutput(t *testing.T) {
	m := &AgentLoopMachine{hooks: &HookChain{}} // finish needs hooks; no LLM/deps
	result := &api.RunResult{Status: api.RunCompleted, Summary: "done"}

	// Schema-less: Output stays the RunResult struct (prior behavior).
	plain := m.finish("complete", &agentState{StartedAt: time.Now()}, result, nil, nil)
	if _, ok := plain.Output.(*api.RunResult); !ok {
		t.Fatalf("schema-less finish must keep the RunResult struct; got %T", plain.Output)
	}

	// Output-schema task: Output is the structured envelope map.
	envelope := map[string]any{"verdict": "pass", "child_session_id": "019x", "unmet_criteria": []any{}}
	as := &agentState{StartedAt: time.Now(), StructuredOutput: envelope}
	got := m.finish("complete", as, result, nil, nil)
	out, ok := got.Output.(map[string]any)
	if !ok {
		t.Fatalf("structured finish must set Output to the envelope map; got %T", got.Output)
	}
	if out["verdict"] != "pass" || out["child_session_id"] != "019x" {
		t.Fatalf("envelope must pass through verbatim; got %v", out)
	}
}
