// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"
)

// A run refused at its start (a model whose key is not set) is shown in its
// own chat: a session created for it, the reason in it, the session ended
// failed - never only in the pseudo-session "error" no chat shows.
func TestARunRefusedAtItsStartShowsWhyInItsOwnChat(t *testing.T) {
	why := "start task \"new-study\": create model: anthropic: key ANTHROPIC_FOUNDRY_API_KEY is not set"
	evs := runStartFailureEvents("ktw-acme:tasks/new-study", why, time.Unix(0, 42))
	if len(evs) != 3 || evs[0].name != "session.created" || evs[1].name != "msg.new" || evs[2].name != "session.ended" {
		t.Fatalf("events = %+v", evs)
	}
	sid := evs[0].payload.(map[string]any)["session"].(map[string]any)["id"].(string)
	if sid == "" || sid == "error" || !strings.HasSuffix(sid, "new-study") {
		t.Fatalf("session id = %q", sid)
	}
	msg := evs[1].payload.(map[string]any)
	if msg["session"] != sid {
		t.Errorf("message in session %v, want %q", msg["session"], sid)
	}
	content := msg["msg"].(map[string]any)["content"].(map[string]any)
	if content["type"] != "system" || content["level"] != "error" || content["message"] != why {
		t.Errorf("content = %+v", content)
	}
	end := evs[2].payload.(map[string]string)
	if end["session"] != sid || end["status"] != "error" || end["error"] != why {
		t.Errorf("end = %+v", end)
	}
}

// A workflow writes no message of its own: the session's own run ending with
// an {ok, message, open} answer says it in the chat and opens what it made;
// refused (ok false) or failed, the reason is shown as an error. A nested
// run's end, or an answer without a message, says nothing.
func TestAWorkflowAnswerIsSaidInItsChat(t *testing.T) {
	done := map[string]any{"type": "team.workflow_done", "runId": "run-1",
		"output": map[string]any{"ok": true, "message": "Étude créée", "open": "etude-x"}}
	evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", done)
	if len(evs) != 2 || evs[0].name != "msg.new" || evs[1].name != "run.open" {
		t.Fatalf("done: %+v", evs)
	}
	msg := evs[0].payload.(map[string]any)
	content := msg["msg"].(map[string]any)["content"].(map[string]any)
	if msg["session"] != "s1" || content["type"] != "text" || content["text"] != "Étude créée" {
		t.Errorf("done message = %+v", msg)
	}
	if o := evs[1].payload.(map[string]any); o["open"] != "etude-x" || o["session"] != "s1" {
		t.Errorf("open = %+v", o)
	}

	refused := map[string]any{"runId": "run-1", "output": map[string]any{"ok": false, "message": "Le nom manque"}}
	evs = workflowAnswerEvents("s1", "run-1", "team.workflow_done", refused)
	if len(evs) != 1 {
		t.Fatalf("refused: %+v", evs)
	}
	content = evs[0].payload.(map[string]any)["msg"].(map[string]any)["content"].(map[string]any)
	if content["level"] != "error" || content["message"] != "Le nom manque" {
		t.Errorf("refused content = %+v", content)
	}

	failed := map[string]any{"runId": "run-1", "error": "call create: timeout"}
	evs = workflowAnswerEvents("s1", "run-1", "team.workflow_failed", failed)
	if len(evs) != 1 || evs[0].payload.(map[string]any)["msg"].(map[string]any)["content"].(map[string]any)["message"] != "call create: timeout" {
		t.Errorf("failed: %+v", evs)
	}

	if evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", map[string]any{"runId": "run-2", "output": done["output"]}); evs != nil {
		t.Errorf("a nested run's end said something: %+v", evs)
	}
	if evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", map[string]any{"runId": "run-1", "output": map[string]any{"ok": true}}); evs != nil {
		t.Errorf("an answer without a message said something: %+v", evs)
	}
}

// An answer naming what it made opens it even with no message:
// {ok: true, open} alone emits run.open and nothing to say; refused, it
// opens nothing.
func TestAWorkflowAnswerOpensWhatItMadeWithoutAMessage(t *testing.T) {
	made := map[string]any{"runId": "run-1", "output": map[string]any{"ok": true, "open": "etude-x"}}
	evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", made)
	if len(evs) != 1 || evs[0].name != "run.open" {
		t.Fatalf("made: %+v", evs)
	}
	if o := evs[0].payload.(map[string]any); o["open"] != "etude-x" || o["session"] != "s1" {
		t.Errorf("open = %+v", o)
	}
	if evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", map[string]any{"runId": "run-1", "output": map[string]any{"open": "etude-x"}}); len(evs) != 1 || evs[0].name != "run.open" {
		t.Errorf("an answer with open and no ok: %+v", evs)
	}
	if evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", map[string]any{"runId": "run-1", "output": map[string]any{"ok": false, "open": "etude-x"}}); evs != nil {
		t.Errorf("a refused answer opened something: %+v", evs)
	}
}

// The session's own run is the engine's run the workflow events carry, never
// the terminal's client run id StartTaskRun answers (the two differ): its
// first parentless workflow start names it, a nested one does not, and its
// end then says its answer (the person's « Nouvelle étude » created a study
// and the chat stayed silent while the two ids were compared).
func TestTheRootRunIsTheEnginesNotTheClients(t *testing.T) {
	const client, engine, nested = "bc9ad453ad5c9260", "run-da21c713ca051b9a", "run-child"
	_ = client // StartTaskRun's id: no longer what the bridge compares with
	events := []struct {
		typ string
		obj map[string]any
	}{
		{"state.sync", map[string]any{}},
		{"team.workflow_started", map[string]any{"runId": engine}},
		{"team.workflow_started", map[string]any{"runId": nested, "parentRunId": engine}},
		{"team.workflow_done", map[string]any{"runId": nested, "output": map[string]any{"ok": true, "message": "nested"}}},
		{"team.workflow_done", map[string]any{"runId": engine, "output": map[string]any{"ok": true, "message": "Étude créée", "open": "etude-erp-acme"}}},
	}
	root := ""
	var said []guiEvent
	for _, e := range events {
		root = rootRunOf(root, e.typ, e.obj)
		said = append(said, workflowAnswerEvents("s1", root, e.typ, e.obj)...)
	}
	if root != engine {
		t.Fatalf("root = %q, want the engine's run %q", root, engine)
	}
	if len(said) != 2 || said[1].name != "run.open" || said[1].payload.(map[string]any)["open"] != "etude-erp-acme" {
		t.Fatalf("said = %+v, want the root's message and its study opened", said)
	}
}

// An agent task's run ends with its final result as the summary:
// a result that is an {ok, open} answer - alone or in a json fence - opens
// what it names, the result itself being already in the chat; prose, a
// refused answer or a workflow's own output carrying a summary open nothing.
func TestAnAgentResultNamingOpenOpensIt(t *testing.T) {
	done := func(summary string) map[string]any {
		return map[string]any{"runId": "run-1", "output": map[string]any{"summary": summary}}
	}
	for _, s := range []string{`{"ok":true,"open":"etude-x#panel:etude"}`, "```json\n{\"ok\":true,\"open\":\"etude-x#panel:etude\"}\n```"} {
		evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", done(s))
		if len(evs) != 1 || evs[0].name != "run.open" || evs[0].payload.(map[string]any)["open"] != "etude-x#panel:etude" {
			t.Errorf("%q: %+v", s, evs)
		}
	}
	for _, s := range []string{"J'ai créé l'étude.", `{"ok":false,"open":"etude-x"}`, `{"ok":true}`} {
		if evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", done(s)); evs != nil {
			t.Errorf("%q said something: %+v", s, evs)
		}
	}
	wf := map[string]any{"runId": "run-1", "output": map[string]any{"summary": "x", "ok": true, "open": "etude-y"}}
	if evs := workflowAnswerEvents("s1", "run-1", "team.workflow_done", wf); len(evs) != 1 || evs[0].payload.(map[string]any)["open"] != "etude-y" {
		t.Errorf("a workflow answer with a summary: %+v", evs)
	}
}
