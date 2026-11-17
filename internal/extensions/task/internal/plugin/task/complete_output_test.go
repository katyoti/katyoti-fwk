// SPDX-License-Identifier: Apache-2.0

package task

import (
	"encoding/json"
	"errors"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// completeErr runs handleComplete and returns the ErrRunComplete it signals.
func completeErr(t *testing.T, p *taskPlugin, input string) *agentapi.ErrRunComplete {
	t.Helper()
	_, err := p.handleComplete(json.RawMessage(input))
	var rc *agentapi.ErrRunComplete
	if !errors.As(err, &rc) {
		t.Fatalf("handleComplete must return *ErrRunComplete; got %v", err)
	}
	return rc
}

// TestHandleComplete_StructuredOutput: an output-schema task captures the FULL
// complete input as the Output envelope (so task.output is the declared map),
// while keeping a meaningful run summary.
func TestHandleComplete_StructuredOutput(t *testing.T) {
	p := &taskPlugin{completeSchema: []byte(`{"type":"object"}`)}
	rc := completeErr(t, p, `{"verdict":"pass","unmet_criteria":[],"child_session_id":"019abc"}`)

	if rc.Output == nil {
		t.Fatal("schema task must capture the structured Output envelope")
	}
	if rc.Output["verdict"] != "pass" || rc.Output["child_session_id"] != "019abc" {
		t.Fatalf("Output must carry the declared fields verbatim; got %v", rc.Output)
	}
	if rc.Summary == "" {
		t.Error("run summary must stay meaningful even with no summary field")
	}
}

// TestHandleComplete_GenericNoOutput: a schema-less task keeps {summary,files}
// and a nil Output (the prior behavior — no envelope to carry).
func TestHandleComplete_GenericNoOutput(t *testing.T) {
	p := &taskPlugin{} // completeSchema nil
	rc := completeErr(t, p, `{"summary":"did the thing","files":["a.md"]}`)

	if rc.Output != nil {
		t.Errorf("schema-less task must leave Output nil; got %v", rc.Output)
	}
	if rc.Summary != "did the thing" || len(rc.Files) != 1 {
		t.Fatalf("generic shape must round-trip summary+files; got %+v", rc)
	}
}

// A task that declares its output and completes with none (null) is told to
// give it — it "completed" with no output.
func TestCompleteWithoutTheDeclaredOutputIsRefused(t *testing.T) {
	p := &taskPlugin{completeSchema: []byte(`{"type":"object"}`)}
	_, err := p.handleComplete(json.RawMessage(`null`))
	var done *agentapi.ErrRunComplete
	if err == nil || errors.As(err, &done) {
		t.Fatalf("complete(null) = %v; want an error asking for the output", err)
	}
}
