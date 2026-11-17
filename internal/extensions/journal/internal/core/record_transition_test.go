// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// A transitioned record keeps what the move's event says beyond the states:
// which lifecycle moved, whether the guard was forced, and what its functions
// answered — the object's history is read from it, nothing else keeps them.
func TestATransitionedRecordKeepsForcedCallsAndCapability(t *testing.T) {
	p, _ := json.Marshal(cwmapi.ObjectTransitionedEventPayload{
		ObjectID: "m.g", Module: "m", Type: "goal", Workflow: "flow", Capability: "review",
		From: "open", To: "closed", Transition: "close", By: cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson},
		Note: "demo", Forced: true,
		Calls: []cwmapi.FunctionAnswer{{Function: "gate", Args: []any{"ready"}, Answer: false}},
	})
	rec, ok := recordOf(cwmapi.EventEnvelope{Topic: cwmapi.ObjectTransitionedSubject("t", "u", "w", cwmapi.SubjectToken("m.g")), Payload: p})
	if !ok || rec.Action != api.ActionTransitioned {
		t.Fatalf("record = %+v (%v)", rec, ok)
	}
	if rec.Capability != "review" || !rec.Forced || len(rec.Calls) != 1 || rec.Calls[0].Function != "gate" {
		t.Errorf("record = %+v, want capability review, forced, the gate's answer", rec)
	}
	e := entryOf(rec)
	if e.From != "open" || e.To != "closed" || !e.Forced || len(e.Calls) != 1 || e.By.ID != "ana" || e.Note != "demo" {
		t.Errorf("entry = %+v", e)
	}
}

// A file's lifecycle state, for the git fallback: a YAML object file or a
// document's frontmatter.
func TestLifecycleStateIn(t *testing.T) {
	yamlFile := "id: m.g\ntype: goal\ncapabilities:\n  lifecycle:\n    state: accepted\n"
	doc := "---\n" + yamlFile + "---\n## Body\n\nstate: nope\n"
	if got := lifecycleStateIn([]byte(yamlFile)); got != "accepted" {
		t.Errorf("yaml file: %q", got)
	}
	if got := lifecycleStateIn([]byte(doc)); got != "accepted" {
		t.Errorf("document: %q", got)
	}
	if got := lifecycleStateIn([]byte("id: m.g\n")); got != "" {
		t.Errorf("no lifecycle: %q", got)
	}
}
