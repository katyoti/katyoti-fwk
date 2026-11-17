// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
)

type doneWith string

func (d doneWith) DoneError() string { return string(d) }

// : a run whose Done said an error (its model call failed, it
// could not start) ends its session "error" with the reason - the chat's
// banner says « Tâche en échec », never « Tâche terminée »; a run that ended
// well ends "done".
func TestARunThatFailedEndsItsSessionFailed(t *testing.T) {
	why := "terminal: run task: LLM error on turn 1: anthropic: key ANTHROPIC_FOUNDRY_API_KEY is not set"
	if end := sessionEnd(doneWith(why), "s1"); end["status"] != "error" || end["error"] != why || end["session"] != "s1" {
		t.Fatalf("a failed run ends %+v", end)
	}
	if end := sessionEnd(doneWith(""), "s1"); end["status"] != "done" || end["session"] != "s1" {
		t.Fatalf("a run that ended well ends %+v", end)
	}
	if end := sessionEnd(struct{}{}, "s1"); end["status"] != "done" {
		t.Fatalf("a transport that says no Done ends %+v", end)
	}
}

type doneWithMsg struct {
	why string
	msg *usermsg.Msg
}

func (d doneWithMsg) DoneError() string     { return d.why }
func (d doneWithMsg) DoneMsg() *usermsg.Msg { return d.msg }

// A run that fails at once (its model's key not set) has its error message
// published a hair after its Done: the chat never got it and stayed empty
// under « Tâche en échec » (rc5, new-study-v1). The run's end says the
// reason in the chat, keyed so the person reads it in their language - once:
// a run whose error message did reach the chat ends with no second one.
func TestARunThatFailedSaysWhyInTheChat(t *testing.T) {
	why := "terminal: run task: create machine: create model: anthropic: key ANTHROPIC_FOUNDRY_API_KEY is not set"
	key := &usermsg.Msg{Key: "agent.modelKeyMissing", Params: map[string]any{"model": "foundry-sonnet"}}
	evs := runEndEvents(doneWithMsg{why, key}, "s1", false)
	if len(evs) != 2 || evs[0].name != "msg.new" || evs[1].name != "session.ended" {
		t.Fatalf("a failed run with no error in its chat ends with %+v", evs)
	}
	if !isErrorMessage(evs[0].name, evs[0].payload) {
		t.Fatalf("its message is no error: %+v", evs[0].payload)
	}
	ev := evs[0].payload.(map[string]any)
	msg := ev["msg"].(map[string]any)
	content := msg["content"].(map[string]any)
	if ev["session"] != "s1" || content["message"] != why || content["msg"] != key {
		t.Fatalf("its message is %+v", ev)
	}
	if evs := runEndEvents(doneWithMsg{why, key}, "s1", true); len(evs) != 1 || evs[0].name != "session.ended" {
		t.Fatalf("a failed run whose error reached the chat ends with %+v", evs)
	}
	if evs := runEndEvents(doneWith(""), "s1", false); len(evs) != 1 || evs[0].name != "session.ended" {
		t.Fatalf("a run that ended well ends with %+v", evs)
	}
	// The core's own error message counts as said.
	said := map[string]any{"type": "msg.new", "msg": map[string]any{"content": map[string]any{"type": "system", "level": "error", "message": why}}}
	if !isErrorMessage("msg.new", said) || isErrorMessage("msg.delta", said) {
		t.Fatal("isErrorMessage misreads the core's error message")
	}
}
