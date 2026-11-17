// SPDX-License-Identifier: Apache-2.0

package task

import (
	"strings"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// stubLoop is a minimal LoopIntrospector for asserting mode-gated prompt blocks.
type stubLoop struct{ mode string }

func (s stubLoop) RunID() string                { return "" }
func (s stubLoop) Mode() string                 { return s.mode }
func (s stubLoop) Messages() []agentapi.Message { return nil }
func (s stubLoop) InheritedPlugins() []any      { return nil }
func (s stubLoop) PermissionProfile() string    { return "" }

func TestSystemMessage_CompletionGatedByChatMode(t *testing.T) {
	// Chat mode: `complete` is filtered from the catalog, so the Completion
	// block must NOT appear (it would contradict the chat protocol).
	chat := &taskPlugin{loop: stubLoop{mode: "chat"}}
	if got := chat.SystemMessage(); strings.Contains(got, "## Completion") {
		t.Errorf("chat-mode SystemMessage must omit ## Completion; got:\n%s", got)
	}

	// Worker mode: completion guidance is present.
	worker := &taskPlugin{loop: stubLoop{mode: "worker"}}
	if got := worker.SystemMessage(); !strings.Contains(got, "## Completion") {
		t.Errorf("worker-mode SystemMessage must include ## Completion; got:\n%s", got)
	}
}

func TestSystemMessage_StructuredCompletionWhenOutputSchema(t *testing.T) {
	// No output-schema → generic "with a summary" wording.
	generic := &taskPlugin{loop: stubLoop{mode: "worker"}}
	if got := generic.SystemMessage(); !strings.Contains(got, "with a summary") {
		t.Errorf("schema-less task must use the generic summary wording; got:\n%s", got)
	}

	// With an output-schema → structured wording that does NOT tell the model to
	// pass a freeform summary (the bug: it dumped markdown into `summary` and
	// task.output came back empty).
	structured := &taskPlugin{loop: stubLoop{mode: "worker"}, completeSchema: []byte(`{"type":"object"}`)}
	got := structured.SystemMessage()
	if !strings.Contains(got, "## Completion") {
		t.Fatalf("structured task must still include ## Completion; got:\n%s", got)
	}
	if strings.Contains(got, "with a summary") {
		t.Errorf("structured-output task must NOT steer toward a freeform summary; got:\n%s", got)
	}
	if !strings.Contains(got, "schema IS this task's declared output") {
		t.Errorf("structured task must say the complete schema IS the output; got:\n%s", got)
	}
}

func TestSystemMessage_DelegationGatedByTaskService(t *testing.T) {
	// No TaskService → no delegate tool → no Delegation block.
	noSvc := &taskPlugin{loop: stubLoop{mode: "worker"}}
	if got := noSvc.SystemMessage(); strings.Contains(got, "## Delegation") {
		t.Errorf("SystemMessage without TaskService must omit ## Delegation; got:\n%s", got)
	}
}
