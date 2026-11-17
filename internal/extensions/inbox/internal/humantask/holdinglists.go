// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// HOLDING LISTS: a human task's to-do is in the group of its
// holding list - its candidates, else the project's list - and the scheduler
// asks the inbox who the list's members are (Members): only they pull its
// questions. A group that is no holding list - none filed under that id -
// is open, as before.

// GroupMembers is the inbox's source of the scheduler's group members.
var GroupMembers = schedapi.GroupMembers{Name: "inbox-holding-lists", Members: Members}

// systemInbox is the system module holding lists are filed in.
const systemInbox = cwmapi.SystemModulePrefix + "inbox"

// Members answers a group's members when the group is a holding list - an
// object of type holding-list in sys-inbox - and whether it is one. A list
// that cannot be read is logged and taken as not listed, so it never stops
// the scheduler handing out the other groups' work.
func Members(ctx context.Context, group string) ([]cwmapi.Actor, bool, error) {
	module, err := cwmapi.ModuleOfObjectID(group)
	if err != nil || module != systemInbox {
		return nil, false, nil
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return nil, false, nil
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: group})
	if errors.Is(err, cwmapi.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		slog.Warn("inbox: a holding list cannot be read; its questions are open to any worker meanwhile", "list", group, "err", err)
		return nil, false, nil
	}
	if got.Object.Type != api.HoldingListType {
		return nil, false, nil
	}
	return membersOf(got.Object), true, nil
}

// ListMembers answers a group's members from every source of holding lists
// the scheduler reads - the inbox's filed lists, a parent run's list
// - and whether one lists it; the inbox's own alone where no
// source is contributed.
func ListMembers(ctx context.Context, group string) ([]cwmapi.Actor, bool, error) {
	srcs := groupSources(ctx)
	if len(srcs) == 0 {
		return Members(ctx, group)
	}
	for _, src := range srcs {
		if src.Members == nil {
			continue
		}
		ms, listed, err := src.Members(ctx, group)
		if err != nil {
			return nil, false, err
		}
		if listed {
			return ms, true, nil
		}
	}
	return nil, false, nil
}

// groupSources are the sources of holding lists contributed to the scheduler
// (a test sets it).
var groupSources = func(ctx context.Context) []schedapi.GroupMembers {
	return application.UseExtensions[schedapi.GroupMembers](ctx)
}

// membersOf are a holding list's members as the scheduler's workers.
func membersOf(o cwmapi.Object) []cwmapi.Actor {
	var f api.HoldingListFields
	if raw := o.FieldsJSON(); len(raw) > 0 {
		_ = json.Unmarshal(raw, &f)
	}
	out := make([]cwmapi.Actor, 0, len(f.Members))
	for _, m := range f.Members {
		if m.ID == "" || m.Kind == "" {
			continue
		}
		out = append(out, cwmapi.Actor{ID: m.ID, Kind: m.Kind})
	}
	return out
}

// listOf is the holding list a human task waits on: its candidates, else
// the project's.
func listOf(s api.HumanTask) string {
	if s.Candidates != "" {
		return s.Candidates
	}
	return api.ProjectList
}

// seatOf opens a question to the agents of its holding list: a
// question an agent may answer (answerable_by agent or any) whose list is a
// filed holding list becomes a to-do its agent members pull and answer
// through the same input - the scheduler lets only the list's members take
// it, persons included. A question only a person may answer, or one whose
// list is open (no holding list filed), stays a person's to-do: no agent
// outside a list ever sees it.
func seatOf(ctx context.Context, t schedapi.Todo, s api.HumanTask) (schedapi.Todo, error) {
	if !api.OpenToAgents(s.AnswerableBy) {
		return t, nil
	}
	_, listed, err := ListMembers(ctx, t.Group)
	if err != nil {
		return t, err
	}
	if listed {
		t.For = schedapi.ForAgent
		t.Work["doer"] = "member"
	}
	return t, nil
}
