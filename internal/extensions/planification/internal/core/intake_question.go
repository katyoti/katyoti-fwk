// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// AN INTAKE QUESTION IS A HUMAN TASK (decision
// ): a request that asks
// its requester one question starts one `provide` human task about the
// request, settling that question; no inbox entry is filed and no asked-in
// edge drawn. Its answer, kept under the request, is added to
// the request's details and plans its intake again (IntakeAnswerKept) - the
// inbox's requests.Answered router is not reached.

// questionMark separates a request from the moment it asked in what an intake
// question's human task settles: <request>/question@<when>.
const questionMark = "/question@"

// intakeQuestionItem is what an intake question's human task settles: that
// question of that request, asked then - a later question is another need.
func intakeQuestionItem(request string, at time.Time) string {
	return request + questionMark + at.UTC().Format(time.RFC3339Nano)
}

// questionedRequest is the request an intake question's item names.
func questionedRequest(item string) (string, bool) {
	request, _, ok := strings.Cut(item, questionMark)
	return request, ok && request != ""
}

// askRequester asks a request's requester one question as a provide human
// task and parks the request idle on it; a request that already waits on one
// is not asked another.
func askRequester(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, in api.AskInput) (api.RequestOutput, error) {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return api.RequestOutput{}, fmt.Errorf("requests.Ask: the scheduler: %w", err)
	}
	if open, err := openQuestion(ctx, todos, in.ID); err != nil {
		return api.RequestOutput{}, err
	} else if open != "" {
		return api.RequestOutput{}, invalid("Ask: %s already waits on the answer to %s", in.ID, open)
	}
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return api.RequestOutput{}, fmt.Errorf("requests.Ask: the task service: %w", err)
	}
	about := fmt.Sprintf("About the request %s: %s", in.ID, titleOf(o))
	if in.Context != "" {
		about = in.Context + "\n\n" + about
	}
	question := strings.TrimSpace(in.Question)
	ask := inboxapi.HumanTask{
		Kind: inboxapi.KindProvide, Title: question, Question: about, About: in.ID,
		Satisfies: []string{intakeQuestionItem(in.ID, time.Now())},
		AskedBy:   "planification:" + in.ID,
	}
	// The question is the requester's: a person requester is asked, and only
	// they answer it.
	if r := fieldsOf(o).Requester; profileapi.IsPerson(r) && r.ID != "" {
		requester := r
		ask.Assignee, ask.AnswerableBy = &requester, inboxapi.AnswerablePerson
	}
	// The request waits on the answer first: one that cannot wait (a split
	// one) asks nothing.
	if st := stateOf(o); st != api.PlanIdle {
		note := "waits for the answer: " + question
		if _, err := NewPlans().move(ctx, in.ID, api.PlanIdle, "wait", in.By, note, func(f *api.PlanFields) { f.Note = note }); err != nil {
			return api.RequestOutput{}, err
		}
	}
	input, err := asInput(ask)
	if err == nil {
		_, err = ts.Run(ctx, taskapi.TaskRunRequest{
			TaskDef: map[string]any{"name": "intake-question", "type": inboxapi.HumanTaskStrategy, "description": "A request asks its requester one question"},
			Input:   input,
			Origin:  &taskapi.RunOrigin{Kind: taskapi.OriginPlanner, ID: "question/" + in.ID},
			Target:  in.ID,
		})
	}
	if err != nil {
		// Not asked: the request is not left waiting on nothing - its intake
		// runs again.
		if _, perr := planIntake(ctx, cwm, in.ID, FilingActor, "the question could not be asked: "+err.Error()); perr != nil {
			return api.RequestOutput{}, fmt.Errorf("requests.Ask: %w (and its intake: %v)", err, perr)
		}
		return api.RequestOutput{}, fmt.Errorf("requests.Ask: ask the requester: %w", err)
	}
	if o, err = load(ctx, cwm, in.ID); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(o)}, nil
}

// openQuestion is the open human task an intake question of the request asks,
// if any.
func openQuestion(ctx context.Context, todos schedapi.TodoService, request string) (string, error) {
	open, err := todos.List(ctx, schedapi.ListInput{})
	if err != nil {
		return "", err
	}
	for _, t := range open.Todos {
		switch t.State {
		case schedapi.StateReady, schedapi.StateClaimed, schedapi.StateReported:
		default:
			continue
		}
		ask, ok := inboxapi.AskOf(t)
		if !ok {
			continue
		}
		for _, it := range ask.Satisfies {
			if r, ok := questionedRequest(it); ok && r == request {
				return t.ID, nil
			}
		}
	}
	return "", nil
}

// IntakeAnswerKept takes a kept answer that answers a request's intake
// question: the question and its answer are added to the request's details
// and its intake is planned again. It reports whether the answer was one; an
// ended request takes no answer.
func (s *Service) IntakeAnswerKept(ctx context.Context, answer string) (bool, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return false, err
	}
	a, err := loadObject(ctx, cwm, answer)
	if err != nil || a.Type != inboxapi.AnswerType {
		return false, err
	}
	rec, ok := answerRecordOf(a)
	if !ok {
		return false, nil
	}
	took := false
	for _, it := range rec.Satisfies {
		id, ok := questionedRequest(it)
		if !ok {
			continue
		}
		took = true
		o, err := load(ctx, cwm, id)
		if err != nil {
			return took, err
		}
		if st := stateOf(o); planEnded(st) || uncounted(st) {
			continue
		}
		note := answerNote(rec)
		if note == "" {
			note = rec.Status
		}
		if err := takeAnswer(ctx, cwm, o, rec.Question, note); err != nil {
			return took, err
		}
	}
	return took, nil
}
