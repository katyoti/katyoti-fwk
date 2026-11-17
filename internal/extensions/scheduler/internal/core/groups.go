// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// HOLDING LISTS: a to-do's group can be a holding list whose
// members are its workers. Only a member pulls its to-dos, a to-do given to
// one worker only by that worker; a group no source lists is open.

// groupOf is a to-do's group: its own, or the default one.
func groupOf(t api.Todo) string {
	if t.Group != "" {
		return t.Group
	}
	return api.DefaultGroup
}

// same reports one actor.
func same(a, b cwmapi.Actor) bool { return a.ID == b.ID && a.Kind == b.Kind }

// members answers a group's members from the first source that lists it;
// listed is false for an open group.
func members(ctx context.Context, group string) ([]cwmapi.Actor, bool, error) {
	for _, src := range application.UseExtensions[api.GroupMembers](ctx) {
		if src.Members == nil {
			continue
		}
		ms, listed, err := src.Members(ctx, group)
		if err != nil {
			return nil, false, fmt.Errorf("todos: the members of %s (%s): %w", group, src.Name, err)
		}
		if listed {
			return ms, true, nil
		}
	}
	return nil, false, nil
}

// memberships caches the members of each group for one look at the queue.
type memberships struct {
	ctx    context.Context
	groups map[string]membership
}

type membership struct {
	members []cwmapi.Actor
	listed  bool
}

func newMemberships(ctx context.Context) *memberships {
	return &memberships{ctx: ctx, groups: map[string]membership{}}
}

func (m *memberships) of(group string) (membership, error) {
	if got, ok := m.groups[group]; ok {
		return got, nil
	}
	ms, listed, err := members(m.ctx, group)
	if err != nil {
		return membership{}, err
	}
	m.groups[group] = membership{members: ms, listed: listed}
	return m.groups[group], nil
}

// isMember reports a worker of the group: any worker of an open group.
func (m *memberships) isMember(group string, by cwmapi.Actor) (bool, error) {
	got, err := m.of(group)
	if err != nil {
		return false, err
	}
	return !got.listed || slices.ContainsFunc(got.members, func(a cwmapi.Actor) bool { return same(a, by) }), nil
}

// mayTake says why a worker may not take a to-do - given to another worker,
// or held for its holding list's members - or "" when it may.
func (m *memberships) mayTake(t api.Todo, by cwmapi.Actor) (string, error) {
	why, _, err := m.mayWait(t, by)
	return why, err
}

// mayWait is mayTake with its structured wait: assigned (the worker it is
// given to) or members (its holding list).
func (m *memberships) mayWait(t api.Todo, by cwmapi.Actor) (string, api.QueueWait, error) {
	if t.Assignee != nil && !same(*t.Assignee, by) {
		a := *t.Assignee
		return "given to " + t.Assignee.ID, api.QueueWait{Cause: api.WaitAssigned, Holder: &a}, nil
	}
	group := groupOf(t)
	ok, err := m.isMember(group, by)
	if err != nil {
		return "", api.QueueWait{}, err
	}
	if !ok {
		return "held for the members of " + group, api.QueueWait{Cause: api.WaitMembers, Group: group}, nil
	}
	return "", api.QueueWait{}, nil
}

// only is who alone may take a to-do, whoever asks: the worker it is given to
// (assigned) or its holding list's members (members) - the rule mayWait
// applies, never who is on the list; nil when any worker may.
func (m *memberships) only(t api.Todo) (*api.QueueWait, error) {
	if t.Assignee != nil {
		a := *t.Assignee
		return &api.QueueWait{Todo: t.ID, Cause: api.WaitAssigned, Holder: &a}, nil
	}
	group := groupOf(t)
	got, err := m.of(group)
	if err != nil || !got.listed {
		return nil, err
	}
	return &api.QueueWait{Todo: t.ID, Cause: api.WaitMembers, Group: group}, nil
}

// Assign gives a to-do to one member of its holding list: only that worker
// pulls it then; a claim another holds ends.
func (s *Service) Assign(ctx context.Context, in api.AssignInput) (api.TodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.TodoOutput{}, err
	}
	if err := checkActor(in.To); err != nil {
		return api.TodoOutput{}, invalid("Assign: to: %v", err)
	}
	ms := newMemberships(ctx)
	var out api.Todo
	err := s.change(ctx, in.ID, func(t *api.Todo) error {
		if t.State != api.StateReady && t.State != api.StateClaimed {
			return invalid("Assign: %s is %s: only a ready or a pulled to-do is given", t.ID, t.State)
		}
		group := groupOf(*t)
		ok, err := ms.isMember(group, in.To)
		if err != nil {
			return err
		}
		if !ok {
			return invalid("Assign: %s is not a member of %s", in.To.ID, group)
		}
		holds := held(*t) && same(t.Claim.By, in.By)
		member, err := ms.isMember(group, in.By)
		if err != nil {
			return err
		}
		listed, err := ms.of(group)
		if err != nil {
			return err
		}
		if in.By.Kind != "person" && !holds && !(listed.listed && member) {
			return invalid("Assign: %s gives %s neither as a person, a member of %s nor its holder", in.By.ID, t.ID, group)
		}
		to := in.To
		t.Assignee = &to
		if t.State == api.StateClaimed && !same(t.Claim.By, to) {
			t.State, t.Claim = api.StateReady, nil
		}
		if in.Note != "" {
			t.Note = in.Note
		}
		out = *t
		return nil
	})
	if err == nil {
		wake(ctx)
	}
	return api.TodoOutput{Todo: out}, err
}
