// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

func TestWithSessionValuesPreservesReactionCause(t *testing.T) {
	reaction := cause.Cause{Kind: cause.KindReaction, ID: "subscription@event", Depth: 1}
	ctx := cause.WithCause(context.Background(), reaction)
	got := machineWithRunID().withSessionValues(ctx)

	if actual, ok := cause.CauseFrom(got); !ok || actual != reaction {
		t.Fatalf("cause = (%+v, %t), want (%+v, true)", actual, ok, reaction)
	}
	if runID := api.RunIDFromContext(got); runID != "agent-run" {
		t.Fatalf("run id = %q, want agent-run", runID)
	}
}

func TestWithSessionValuesAttributesNonReactionWritesToAgentRun(t *testing.T) {
	previous := cause.Cause{Kind: cause.KindPerson, ID: "ada", Depth: 1}
	ctx := cause.WithCause(context.Background(), previous)
	got := machineWithRunID().withSessionValues(ctx)

	want := cause.Cause{Kind: cause.KindAgent, ID: "agent-run", Depth: previous.Depth}
	if actual, ok := cause.CauseFrom(got); !ok || actual != want {
		t.Fatalf("cause = (%+v, %t), want (%+v, true)", actual, ok, want)
	}
}

func machineWithRunID() *AgentLoopMachine {
	loop := NewLoopContext()
	loop.SetMeta("_runId", "agent-run")
	return &AgentLoopMachine{
		loop:       loop,
		runRequest: &api.AgentRunRequest{},
	}
}
