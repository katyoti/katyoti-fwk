// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// answered is a reported human task's to-do about what, answered by whom.
func answered(id, about string, by cwmapi.Actor) schedapi.Todo {
	return schedapi.Todo{ID: id, Owner: api.Owner, Work: map[string]any{"task": api.HumanTaskStrategy,
		api.WorkAsk: map[string]any{"kind": "approve", "title": "Validate the release", "about": about, "satisfies": []any{"gate/x"}}},
		State: schedapi.StateReported, Result: &schedapi.Result{Status: schedapi.StatusSuccess, Verdict: "approved", Summary: "ok", By: by, At: "2026-10-23T20:00:00Z",
			Relayed: &schedapi.Relayed{By: "session-1", Confirmed: "passkey"}}}
}

// TestAnAnswerIsKeptOnWhatItIsAbout: before its to-do is
// archived and its asker told, an answer is kept as an answer record under
// the object it is about - who answered, the proof, what it settled - once;
// one about nothing, or about an object gone, is kept under sys-inbox.
func TestAnAnswerIsKeptOnWhatItIsAbout(t *testing.T) {
	ctx, todos, _, cwm := universe(t)
	person := cwmapi.Actor{ID: "jdoe", Kind: "person"}
	td := answered("ui.TD-7", api.ProjectList, person)
	todos.todos[td.ID] = &td
	if err := Collect(ctx, td); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "sys-inbox", ID: "sys-inbox.answer-td-7"})
	if err != nil {
		t.Fatalf("the kept answer: %v", err)
	}
	if got.Object.Type != api.AnswerType || got.Object.ParentID() != api.ProjectList {
		t.Errorf("kept as %s under %s; want an answer under %s", got.Object.Type, got.Object.ParentID(), api.ProjectList)
	}
	f := got.Object.Fields
	if f["by_id"] != "jdoe" || f["verdict"] != "approved" || f["proof"] != "passkey" || f["task"] != "ui.TD-7" || f["kind"] != "approve" {
		t.Errorf("fields %v; want who, the verdict, the proof, the task and the kind", f)
	}
	if _, err := Store(ctx, td); err != nil {
		t.Errorf("kept twice: %v; want one record, no error", err)
	}
	gone := answered("ui.TD-8", "ui.F-999", person)
	id, err := Store(ctx, gone)
	if err != nil || id != "sys-inbox.answer-td-8" {
		t.Errorf("about an object gone: %q, %v; want it kept under sys-inbox", id, err)
	}
}

// denyAll is a permission service that holds no grant.
type denyAll struct{ profileapi.PermissionService }

func (denyAll) Check(_ context.Context, in profileapi.PermissionCheckInput) (profileapi.PermissionCheckOutput, error) {
	return profileapi.PermissionCheckOutput{Allowed: false, Code: in.Code, Reason: "no grant"}, nil
}

// TestAnAnswerNotKeptGoesNoFurther: an answerer who does not
// hold the question's permission code is refused, and an answer that cannot
// be kept stops the collect: its to-do is not archived and no asker is told.
func TestAnAnswerNotKeptGoesNoFurther(t *testing.T) {
	ctx, todos, bus, _ := universe(t)
	application.PutService[profileapi.PermissionService](application.AppFromContext(ctx), "default", denyAll{})
	td := answered("ui.TD-9", "", cwmapi.Actor{ID: "relecteur", Kind: "agent"})
	todos.todos[td.ID] = &td
	told := 0
	bus.Subscribe(engine.HumanTaskAnsweredTopic(td.ID), func(string, any) { told++ })
	err := Collect(ctx, td)
	if !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("an answerer without the code: %v; want refused", err)
	}
	if len(todos.archived) != 0 || told != 0 {
		t.Errorf("archived %v, told %d; want nothing gone on", todos.archived, told)
	}
}
