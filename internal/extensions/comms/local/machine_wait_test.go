// SPDX-License-Identifier: Apache-2.0

package commslocal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A run that waits as a machine never blocks in a comms call: a child's
// question and a parent's await of its children each return "await this
// topic, then call me again" — the run holds no slot and no goroutine — and,
// called again, say what happened.

// awaited is the ErrAwait err must be, its Resume as saved with the run and
// read back.
func awaited(t *testing.T, err error) *agentapi.ErrAwait {
	t.Helper()
	var aw *agentapi.ErrAwait
	if !errors.As(err, &aw) {
		t.Fatalf("err = %v; want the call to await as a machine", err)
	}
	b, _ := json.Marshal(aw.Resume)
	aw.Resume = nil
	if err := json.Unmarshal(b, &aw.Resume); err != nil {
		t.Fatal(err)
	}
	return aw
}

func calledAgain(ctx context.Context, aw *agentapi.ErrAwait) context.Context {
	return agentapi.WithAwaitResume(ctx, aw.Resume)
}

// told records the topics published on the channel.
func told(ch *engine.Channel) func(string) bool {
	topics := make(chan string, 16)
	ch.Bus().SubscribeAny(func(topic string, _ any) { topics <- topic })
	return func(want string) bool {
		for {
			select {
			case got := <-topics:
				if got == want {
					return true
				}
			case <-time.After(time.Second):
				return false
			}
		}
	}
}

// questionRuns are the human tasks a child's questions start (the task
// service's runs of the human-task strategy) and their to-dos (the
// scheduler's): a report of a to-do ends its run with the answer, as the
// inbox's collector does.
type questionRuns struct {
	taskapi.TaskService
	mu      sync.Mutex
	asked   []taskapi.TaskRunRequest
	ended   map[string]inboxapi.HumanAnswer
	todos   map[string]*schedapi.Todo
	pulls   []schedapi.PullInput
	reports []schedapi.ReportInput
}

func newQuestionRuns() *questionRuns {
	return &questionRuns{ended: map[string]inboxapi.HumanAnswer{}, todos: map[string]*schedapi.Todo{}}
}

func (q *questionRuns) Run(_ context.Context, req taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.asked = append(q.asked, req)
	run := fmt.Sprintf("ht-%d", len(q.asked))
	in := req.Input
	group, _ := in["candidates"].(string)
	q.todos[run] = &schedapi.Todo{ID: "sys-planification.TD-" + run, Key: inboxapi.HumanTaskStrategy + ":" + run, Owner: inboxapi.Owner,
		For: schedapi.ForAgent, Group: group, State: schedapi.StateReady, Work: map[string]any{inboxapi.WorkAsk: in}}
	return taskapi.TaskRunResponse{RunID: run}, nil
}

func (q *questionRuns) Status(_ context.Context, req taskapi.StatusRequest) (taskapi.StatusResponse, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.todos[req.RunID]; !ok {
		return taskapi.StatusResponse{}, errors.New("no such run")
	}
	if _, ok := q.ended[req.RunID]; ok {
		return taskapi.StatusResponse{RunID: req.RunID, Status: taskapi.RunStatusCompleted}, nil
	}
	return taskapi.StatusResponse{RunID: req.RunID, Status: taskapi.RunStatusRunning}, nil
}

func (q *questionRuns) Result(_ context.Context, req taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	for {
		q.mu.Lock()
		a, ok := q.ended[req.RunID]
		q.mu.Unlock()
		if ok {
			return taskapi.ResultResponse{RunID: req.RunID, Status: "completed", Output: a}, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (t questionTodos) List(context.Context, schedapi.ListInput) (schedapi.TodosOutput, error) {
	q := t.r
	q.mu.Lock()
	defer q.mu.Unlock()
	out := schedapi.TodosOutput{}
	for _, t := range q.todos {
		out.Todos = append(out.Todos, *t)
	}
	return out, nil
}

func (t questionTodos) Pull(_ context.Context, in schedapi.PullInput) (schedapi.PullOutput, error) {
	q := t.r
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pulls = append(q.pulls, in)
	for _, t := range q.todos {
		if t.ID == in.ID {
			t.State, t.Claim = schedapi.StateClaimed, &schedapi.Claim{By: in.By}
			c := *t
			return schedapi.PullOutput{Todo: &c}, nil
		}
	}
	return schedapi.PullOutput{Nothing: "no such to-do"}, nil
}

func (t questionTodos) Report(_ context.Context, in schedapi.ReportInput) (schedapi.TodoOutput, error) {
	q := t.r
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reports = append(q.reports, in)
	for run, t := range q.todos {
		if t.ID == in.ID {
			r := in.Result
			t.State, t.Result = schedapi.StateReported, &r
			q.ended[run] = inboxapi.HumanAnswer{Task: t.ID, Status: r.Status, Answer: r.Answer, Summary: r.Summary, By: r.By}
			return schedapi.TodoOutput{Todo: *t}, nil
		}
	}
	return schedapi.TodoOutput{}, errors.New("no such to-do")
}

// questionTodos is the scheduler's side of questionRuns.
type questionTodos struct {
	schedapi.TodoService
	r *questionRuns
}

var _ schedapi.TodoService = questionTodos{}

// asking is a comms service whose task service and scheduler are q.
func asking(t *testing.T) (*service, *questionRuns, *engine.Channel, context.Context) {
	t.Helper()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	q := newQuestionRuns()
	application.PutService[taskapi.TaskService](app, "default", q)
	application.PutService[schedapi.TodoService](app, "default", questionTodos{r: q})
	svc, ch, ctx := newTestServiceWithChannel()
	return svc, q, ch, application.WithApplication(ctx, app)
}

// TestAChildsQuestionIsAMachineWait: a child's
// question to its parent is a human task - an answer to give (a choice when it
// offers options) on the holding list of its parent's run, answerable by an
// agent -; its parent is told and shown it; the child awaits the task's end as
// a machine, holding nothing; the parent answers through the task's input -
// it takes the to-do and reports it as its run -; called again, the child
// gets the answer.
func TestAChildsQuestionIsAMachineWait(t *testing.T) {
	svc, q, ch, base := asking(t)
	isTold := told(ch)
	child := agentapi.WithMachineWaits(base)
	ask := commsapi.AskRequest{Question: "which colour?", Options: []string{"blue", "red"}, CallerRunID: "child-1", ParentRunID: "parent-1", CallerName: "painter"}

	_, err := svc.Ask(child, ask)
	aw := awaited(t, err)
	qid, _ := aw.Resume[resumeQuestion].(string)
	if qid == "" || aw.Topic != engine.RunEndedTopic(qid) {
		t.Fatalf("await = %+v; want the human task's end", aw)
	}
	if len(q.asked) != 1 {
		t.Fatalf("human tasks started = %d; want 1", len(q.asked))
	}
	run := q.asked[0]
	in := run.Input
	if run.TaskDef["type"] != inboxapi.HumanTaskStrategy || run.ParentRunID != "child-1" || in["kind"] != inboxapi.KindChoose ||
		in["candidates"] != commsapi.ParentList("parent-1") || in["answerable_by"] != inboxapi.AnswerableAgent || in["asked_by"] != "child-1" ||
		in["title"] != "which colour?" {
		t.Fatalf("the human task = %+v %+v; want a choice on the parent's list, answerable by an agent, asked by the child", run, in)
	}
	if !isTold(engine.ChildEventsTopic("parent-1")) {
		t.Error("the parent was not told its child asked")
	}
	if qs := svc.PendingQuestions(base, "parent-1"); len(qs) != 1 || qs[0].ID != qid || qs[0].ChildTaskID != "child-1" {
		t.Fatalf("parent's questions = %+v", qs)
	}

	// Called again before the answer: it awaits again.
	if _, err := svc.Ask(calledAgain(child, aw), ask); err == nil {
		t.Fatal("answered before its parent answered")
	} else {
		awaited(t, err)
	}

	if res, err := svc.Answer(base, commsapi.AnswerRequest{QuestionID: qid, Answer: "blue"}); err != nil || !res.Delivered {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	parent := commsapi.ParentActor("parent-1")
	if len(q.pulls) != 1 || q.pulls[0].By != parent || len(q.reports) != 1 || q.reports[0].By != parent ||
		q.reports[0].Answer != "blue" || q.reports[0].Status != schedapi.StatusSuccess {
		t.Fatalf("pulls %+v reports %+v; want the to-do taken and reported by the parent's run", q.pulls, q.reports)
	}
	got, err := svc.Ask(calledAgain(child, aw), ask)
	if err != nil || got.Text != "blue" {
		t.Fatalf("called again = %+v, %v; want the parent's answer", got, err)
	}
	// Answered once: a second answer finds nothing to answer.
	if res, err := svc.Answer(base, commsapi.AnswerRequest{QuestionID: qid, Answer: "red"}); err != nil || res.Delivered {
		t.Fatalf("second answer = %+v, %v; want not delivered", res, err)
	}
}

// A child that does not wait as a machine blocks on the human task's end and
// gets its parent's answer.
func TestABlockingChildGetsItsParentsAnswer(t *testing.T) {
	svc, _, _, base := asking(t)
	defer func(d time.Duration) { questionWait = d }(questionWait)
	questionWait = 2 * time.Second
	got := make(chan commsapi.Answer, 1)
	go func() {
		a, err := svc.Ask(base, commsapi.AskRequest{Question: "size?", CallerRunID: "child-2", ParentRunID: "parent-2"})
		if err != nil {
			t.Errorf("ask: %v", err)
		}
		got <- a
	}()
	var qs []agentapi.ChildQuestion
	for i := 0; i < 100 && len(qs) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
		qs = svc.PendingQuestions(base, "parent-2")
	}
	if len(qs) != 1 {
		t.Fatalf("the parent sees %d questions; want 1", len(qs))
	}
	if res, err := svc.Answer(base, commsapi.AnswerRequest{QuestionID: qs[0].ID, Answer: "large"}); err != nil || !res.Delivered {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	select {
	case a := <-got:
		if a.Text != "large" {
			t.Fatalf("the child got %+v; want large", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the child never got its answer")
	}
}

// A question whose human task is gone (its process restarted before it was
// kept) is said lost, not awaited for ever.
func TestALostQuestionIsSaidLost(t *testing.T) {
	svc, _, _, base := asking(t)
	resume := agentapi.WithAwaitResume(agentapi.WithMachineWaits(base), map[string]any{resumeQuestion: "ht-gone"})
	_, err := svc.Ask(resume, commsapi.AskRequest{Question: "?", CallerRunID: "c", ParentRunID: "p"})
	if err == nil || !strings.Contains(err.Error(), "no longer asked") {
		t.Fatalf("err = %v; want the question said lost", err)
	}
}

// The scheduler's holding list of a parent run has its run as its one
// member; any other group is not one of them.
func TestAParentRunsListHasItsRunAsItsMember(t *testing.T) {
	ms, listed, err := ParentLists.Members(context.Background(), commsapi.ParentList("parent-1"))
	if err != nil || !listed || len(ms) != 1 || ms[0] != commsapi.ParentActor("parent-1") {
		t.Fatalf("members = %+v listed %v, %v; want the parent's run", ms, listed, err)
	}
	for _, g := range []string{"sys-inbox.project", "planning", commsapi.ParentListPrefix} {
		if _, listed, _ := ParentLists.Members(context.Background(), g); listed {
			t.Errorf("%q is taken for a parent's list", g)
		}
	}
}

// children is the background manager of a parent's children.
type children struct {
	taskapi.BackgroundTaskManager
	records map[string]taskapi.TaskRecord
}

func (c *children) TaskStatus(id string) (*taskapi.TaskRecord, error) {
	r, ok := c.records[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &r, nil
}

func (c *children) Children(parent string) []taskapi.TaskRecord {
	var out []taskapi.TaskRecord
	for _, r := range c.records {
		if r.ParentRunID == parent {
			out = append(out, r)
		}
	}
	return out
}

func (c *children) set(id string, s taskapi.TaskLifecycle) {
	c.records[id] = taskapi.TaskRecord{ID: id, ParentRunID: "parent-1", Status: s}
}

type childTasks struct {
	taskapi.TaskService
	bg *children
}

func (c childTasks) BackgroundManager() taskapi.BackgroundTaskManager { return c.bg }

func parentAwaiting(t *testing.T) (*service, *children, context.Context) {
	t.Helper()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	bg := &children{records: map[string]taskapi.TaskRecord{}}
	application.PutService[taskapi.TaskService](app, "default", childTasks{bg: bg})
	svc, _, ctx := newTestServiceWithChannel()
	return svc, bg, agentapi.WithMachineWaits(application.WithApplication(ctx, app))
}

// A parent awaiting its named child as a machine: it awaits its children's
// events until the child ends, then says how it ended.
func TestAParentAwaitsItsChildAsAMachine(t *testing.T) {
	svc, bg, ctx := parentAwaiting(t)
	bg.set("child-1", taskapi.TaskRunning)
	req := commsapi.AwaitRequest{ChildTaskID: "child-1", TimeoutS: 600, ParentRunID: "parent-1"}

	_, err := svc.Await(ctx, req)
	aw := awaited(t, err)
	if aw.Topic != engine.ChildEventsTopic("parent-1") || aw.RecheckEvery <= 0 || aw.RecheckEvery > machineRecheck {
		t.Fatalf("await = %+v; want its children's events, checked again", aw)
	}
	if _, err := svc.Await(calledAgain(ctx, aw), req); err == nil {
		t.Fatal("the child still runs; want the parent to await again")
	}
	bg.set("child-1", taskapi.TaskFailed)
	if ev, err := svc.Await(calledAgain(ctx, aw), req); err != nil || ev.Kind != commsapi.EventFailed {
		t.Fatalf("after the child failed = %+v, %v", ev, err)
	}

	// A child its process no longer knows (restarted) is said so.
	gone := commsapi.AwaitRequest{ChildTaskID: "child-gone", ParentRunID: "parent-1"}
	if _, err := svc.Await(ctx, gone); err == nil || !strings.Contains(err.Error(), "not known") {
		t.Fatalf("err = %v; want the child said unknown", err)
	}
}

// Awaiting any child: one that had ended before the await began is not news;
// the next to end, or a question, is; the time up says so.
func TestAParentAwaitsAnyChildAsAMachine(t *testing.T) {
	svc, bg, ctx := parentAwaiting(t)
	bg.set("old", taskapi.TaskCompleted)
	bg.set("new", taskapi.TaskRunning)
	req := commsapi.AwaitRequest{TimeoutS: 600, ParentRunID: "parent-1"}

	_, err := svc.Await(ctx, req)
	var live *agentapi.ErrAwait
	if errors.As(err, &live) {
		// Called again in the same process, its Resume as it was returned.
		if _, err := svc.Await(calledAgain(ctx, live), req); err == nil {
			t.Fatal("a child that had ended before the await was taken for news (in memory)")
		}
	}
	aw := awaited(t, err)
	if _, err := svc.Await(calledAgain(ctx, aw), req); err == nil {
		t.Fatal("a child that had ended before the await was taken for news")
	}
	bg.set("new", taskapi.TaskCompleted)
	if ev, err := svc.Await(calledAgain(ctx, aw), req); err != nil || ev.Kind != commsapi.EventCompleted {
		t.Fatalf("after a child completed = %+v, %v", ev, err)
	}

	svc.broker.Post("parent-1", agentapi.ChildQuestion{ID: "q1", ChildTaskID: "new"})
	bg.set("new", taskapi.TaskRunning)
	if ev, err := svc.Await(ctx, req); err != nil || ev.Kind != commsapi.EventAsked {
		t.Fatalf("with a question asked = %+v, %v", ev, err)
	}

	svc.broker.Take("q1")
	past := agentapi.WithAwaitResume(ctx, map[string]any{
		resumeDeadline: time.Now().Add(-time.Second).Format(time.RFC3339Nano), resumeSeen: []any{"old"}})
	if ev, err := svc.Await(past, req); err != nil || ev.Kind != commsapi.EventTimeout {
		t.Fatalf("past its deadline = %+v, %v; want timeout", ev, err)
	}
}

// A parent awaiting as a machine where no task service is reachable is told
// its children cannot be checked — it waited to its deadline.
func TestAnAwaitWithNoTaskServiceSaysSo(t *testing.T) {
	svc, _, ctx := newTestServiceWithChannel()
	_, err := svc.Await(agentapi.WithMachineWaits(ctx), commsapi.AwaitRequest{ParentRunID: "parent-1", TimeoutS: 600})
	var aw *agentapi.ErrAwait
	if err == nil || errors.As(err, &aw) || !strings.Contains(err.Error(), "cannot be checked") {
		t.Fatalf("await = %v; want it to say its children cannot be checked", err)
	}
}
