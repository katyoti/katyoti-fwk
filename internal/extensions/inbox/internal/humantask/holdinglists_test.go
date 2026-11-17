// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"context"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// seedLists files sys-inbox, the project's holding list (the person and the
// review agent) and an object that is no list.
func seedLists(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) {
	t.Helper()
	mod := cwmapi.Object{ID: "sys-inbox", Type: cwmapi.SystemModuleType}
	mod.Name = "sys-inbox"
	project := cwmapi.Object{ID: api.ProjectList, Type: api.HoldingListType, Parent: "sys-inbox"}
	project.Name = "Project"
	project.Fields = map[string]any{"members": []any{
		map[string]any{"id": "jdoe", "kind": "person"},
		map[string]any{"id": "relecteur", "kind": "agent"},
		map[string]any{"id": "", "kind": "agent"},
	}}
	inbox := cwmapi.Object{ID: "sys-inbox.inbox", Type: "inbox", Parent: "sys-inbox"}
	inbox.Name = "Inbox"
	for _, o := range []cwmapi.Object{mod, project, inbox} {
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "sys-inbox", Object: o}); err != nil {
			t.Fatalf("seed %s: %v", o.ID, err)
		}
	}
}

// TestAHoldingListsMembersAreItsGroupsWorkers: a holding list
// filed in sys-inbox answers its members - persons and agents - as the
// scheduler's workers of its group; a group that is no holding list (none
// filed, another type, outside sys-inbox) stays open.
func TestAHoldingListsMembersAreItsGroupsWorkers(t *testing.T) {
	ctx, cwm := cwmapp.New(t, inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	seedLists(t, ctx, cwm)

	ms, listed, err := Members(ctx, api.ProjectList)
	if err != nil || !listed {
		t.Fatalf("the project's list: listed %v, %v; want listed", listed, err)
	}
	if len(ms) != 2 || ms[0] != (cwmapi.Actor{ID: "jdoe", Kind: "person"}) || ms[1] != (cwmapi.Actor{ID: "relecteur", Kind: "agent"}) {
		t.Errorf("members = %+v; want the person and the review agent, the blank one left out", ms)
	}
	for _, group := range []string{"sys-inbox.none", "sys-inbox.inbox", "planning", "ui.F-061"} {
		if _, listed, err := Members(ctx, group); listed || err != nil {
			t.Errorf("%s: listed %v, %v; want an open group", group, listed, err)
		}
	}
}

// TestAHumanTaskWaitsOnItsHoldingList: its to-do's group is
// the holding list it names, else the project's, and the one it is given to
// is its assignee.
func TestAHumanTaskWaitsOnItsHoldingList(t *testing.T) {
	td := TodoOf(api.HumanTask{Kind: "approve", Title: "Is it right?"}, "run-1", time.Now())
	if td.Group != api.ProjectList || td.Assignee != nil {
		t.Errorf("by default: group %q assignee %v; want the project's list and no one", td.Group, td.Assignee)
	}
	marie := cwmapi.Actor{ID: "marie", Kind: "person"}
	td = TodoOf(api.HumanTask{Kind: "approve", Title: "Is it right?", Candidates: "sys-inbox.reviewers", Assignee: &marie}, "run-1", time.Now())
	if td.Group != "sys-inbox.reviewers" || td.Assignee == nil || *td.Assignee != marie {
		t.Errorf("named: group %q assignee %v; want sys-inbox.reviewers given to marie", td.Group, td.Assignee)
	}
}

// TestAnAgentSitsInAPersonsSeatOnlyWhereAListSeatsIt: a question
// an agent may answer (any, agent) whose list is a filed holding list becomes
// a to-do the list's members pull - its agents among them, the scheduler
// letting no one else take it; a question only a person may answer, or one
// whose list is open, stays a person's to-do no agent pulls.
func TestAnAgentSitsInAPersonsSeatOnlyWhereAListSeatsIt(t *testing.T) {
	ctx, cwm := cwmapp.New(t, inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	seedLists(t, ctx, cwm)
	seat := func(s api.HumanTask) schedapi.Todo {
		t.Helper()
		td, err := seatOf(ctx, TodoOf(s, "run-1", time.Now()), s)
		if err != nil {
			t.Fatal(err)
		}
		return td
	}
	if td := seat(api.HumanTask{Kind: "approve", Title: "Review the chapter", AnswerableBy: api.AnswerableAny}); td.For != schedapi.ForAgent || td.Group != api.ProjectList {
		t.Errorf("any, on the project's list: for %q group %q; want its members' (agents too)", td.For, td.Group)
	}
	if td := seat(api.HumanTask{Kind: "approve", Title: "Release 1.006", AnswerableBy: api.AnswerablePerson}); td.For != schedapi.ForPerson {
		t.Errorf("person only: for %q; want a person's to-do", td.For)
	}
	if td := seat(api.HumanTask{Kind: "approve", Title: "Release 1.006"}); td.For != schedapi.ForPerson {
		t.Errorf("no answerable_by: for %q; want a person's to-do", td.For)
	}
	if td := seat(api.HumanTask{Kind: "approve", Title: "Review", AnswerableBy: api.AnswerableAny, Candidates: "sys-inbox.none"}); td.For != schedapi.ForPerson {
		t.Errorf("any, on a list never filed (open): for %q; want a person's to-do - no agent outside a list sees it", td.For)
	}
}

// TestTheRecordSaysWhichAgentAnsweredAsWhichListsMemberOnWhichRun.
func TestTheRecordSaysWhichAgentAnsweredAsWhichListsMemberOnWhichRun(t *testing.T) {
	relecteur := cwmapi.Actor{ID: "relecteur", Kind: "agent"}
	td := schedapi.Todo{ID: "mail.TD-1", Owner: api.Owner, Group: api.ProjectList,
		Claim:  &schedapi.Claim{By: relecteur, Run: "run-42", Session: "sess-9"},
		Result: &schedapi.Result{Status: schedapi.StatusSuccess, Verdict: "approved", By: relecteur, At: "2026-10-23T19:00:00Z"}}
	a, ok := api.AnswerOf(td)
	if !ok || a.By != relecteur || a.List != api.ProjectList || a.Run != "run-42" || a.Session != "sess-9" {
		t.Errorf("record %+v; want relecteur, member of %s, on run-42", a, api.ProjectList)
	}
	td.Result.By = cwmapi.Actor{ID: "jdoe", Kind: "person"}
	if a, _ := api.AnswerOf(td); a.Run != "" {
		t.Errorf("a person's answer carries the agent's run %q", a.Run)
	}
}

// TestAListAnotherSourceListsSeatsItsAgent: a holding list need
// not be filed in sys-inbox - any source of the scheduler's holding lists
// lists it (a parent run's list, its one member the parent's run): a
// question an agent may answer on it is its members' to-do; the inbox's own
// filed lists still answer for theirs.
func TestAListAnotherSourceListsSeatsItsAgent(t *testing.T) {
	ctx, cwm := cwmapp.New(t, inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	seedLists(t, ctx, cwm)
	parent := cwmapi.Actor{ID: "run-parent", Kind: "agent"}
	runs := schedapi.GroupMembers{Name: "runs", Members: func(_ context.Context, group string) ([]cwmapi.Actor, bool, error) {
		if group != "agent-run:run-parent" {
			return nil, false, nil
		}
		return []cwmapi.Actor{parent}, true, nil
	}}
	defer func(f func(context.Context) []schedapi.GroupMembers) { groupSources = f }(groupSources)
	groupSources = func(context.Context) []schedapi.GroupMembers { return []schedapi.GroupMembers{GroupMembers, runs} }

	s := api.HumanTask{Kind: "form", Title: "Which colour?", AnswerableBy: api.AnswerableAgent, Candidates: "agent-run:run-parent"}
	td, err := seatOf(ctx, TodoOf(s, "run-q", time.Now()), s)
	if err != nil || td.For != schedapi.ForAgent || td.Group != "agent-run:run-parent" || td.Work["doer"] != "member" {
		t.Fatalf("on the parent's list: %+v, %v; want its member's to-do", td, err)
	}
	if ms, listed, err := ListMembers(ctx, "agent-run:run-parent"); err != nil || !listed || len(ms) != 1 || ms[0] != parent {
		t.Errorf("the parent's list = %+v listed %v, %v", ms, listed, err)
	}
	if _, listed, _ := ListMembers(ctx, api.ProjectList); !listed {
		t.Error("the inbox's filed list is no longer listed")
	}
	if _, listed, _ := ListMembers(ctx, "planning"); listed {
		t.Error("an open group is taken for a list")
	}
}
