// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"slices"
	"sort"

	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// ONE NEED, ONE QUESTION - a human task names what its answer
// settles (`satisfies`: gate requirements, verifications, steps) and its one
// answer settles them all: a test by hand and its validation are one card. A
// question whose needs an open human task already asks is not asked again -
// it waits on that one; one whose needs a proven answer already settled is not
// asked at all - that answer is its answer. An asker reads what settled an
// item with SettledBy before it asks.

// IsHumanTask reports a human task's to-do: the inbox's, run by the human-task
// strategy.
func IsHumanTask(t schedapi.Todo) bool {
	if t.Owner != Owner {
		return false
	}
	task, _ := t.Work["task"].(string)
	return task == HumanTaskStrategy
}

// AskOf is what a human task's to-do asks.
func AskOf(t schedapi.Todo) (HumanTask, bool) {
	if !IsHumanTask(t) {
		return HumanTask{}, false
	}
	raw, err := json.Marshal(t.Work[WorkAsk])
	if err != nil {
		return HumanTask{}, false
	}
	var ask HumanTask
	if err := json.Unmarshal(raw, &ask); err != nil {
		return HumanTask{}, false
	}
	return ask, true
}

// Covers reports a human task's to-do whose satisfies hold every item.
func Covers(t schedapi.Todo, items []string) bool {
	if len(items) == 0 {
		return false
	}
	ask, ok := AskOf(t)
	if !ok {
		return false
	}
	for _, it := range items {
		if !slices.Contains(ask.Satisfies, it) {
			return false
		}
	}
	return true
}

// settles reports an answer that settles what it was asked: a success, and
// not a rejection - a refused validation or a failed test leaves the need
// open, to be asked again once the work is redone.
func settles(a HumanAnswer) bool {
	return a.Status == schedapi.StatusSuccess && a.Verdict != "rejected"
}

// Covering is the human task that already holds these needs: an open one
// (ready, claimed or reported) - the question to wait on -, else the latest
// answer that settled them all. Nil when none does: the question is asked.
func Covering(ctx context.Context, todos schedapi.TodoService, items []string) (*schedapi.Todo, error) {
	if len(items) == 0 {
		return nil, nil
	}
	open, err := todos.List(ctx, schedapi.ListInput{})
	if err != nil {
		return nil, err
	}
	for _, t := range open.Todos {
		switch t.State {
		case schedapi.StateReady, schedapi.StateClaimed, schedapi.StateReported:
			if Covers(t, items) {
				t := t
				return &t, nil
			}
		}
	}
	done, err := todos.List(ctx, schedapi.ListInput{Done: true})
	if err != nil {
		return nil, err
	}
	var answered []schedapi.Todo
	for _, t := range done.Todos {
		if a, ok := AnswerOf(t); ok && settles(a) && Covers(t, items) {
			answered = append(answered, t)
		}
	}
	if len(answered) == 0 {
		return nil, nil
	}
	sort.SliceStable(answered, func(i, j int) bool { return answered[i].Result.At > answered[j].Result.At })
	return &answered[0], nil
}

// SettledBy is the latest proven answer that settled one item - a gate
// requirement, a verification, a step - and whether one did: what an asker
// reads before it asks.
func SettledBy(ctx context.Context, todos schedapi.TodoService, item string) (HumanAnswer, bool, error) {
	t, err := Covering(ctx, todos, []string{item})
	if err != nil || t == nil {
		return HumanAnswer{}, false, err
	}
	a, ok := AnswerOf(*t)
	if !ok || !settles(a) {
		return HumanAnswer{}, false, nil
	}
	return a, true, nil
}
