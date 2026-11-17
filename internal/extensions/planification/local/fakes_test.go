// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	personasapi "github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// THE FAKES the tests share: the task service the planner submits to (a test
// sets how each run ends), the persona library, and a wait for what runs on
// its own goroutine.

// fakeTasks stands for the task service: it records the runs started, and a
// test sets how each ends.
type fakeTasks struct {
	mu      sync.Mutex
	started []taskapi.TaskRunRequest
	status  map[string]taskapi.RunStatus
	errs    map[string]string
	// autoComplete: every run is completed the first time it is asked about.
	autoComplete bool
	resultAsked  []string            // the runs whose Result was asked
	groups       []taskapi.GroupLoad // what Groups answers
	stopped      []string            // the runs stopped
	active       []taskapi.RunInfo   // what Runs answers
	setGroups    []taskapi.SetGroupRequest
	sessions     map[string]string // run → its session
	refuse       map[string]string // task → why its runs cannot start
	known        map[string]bool   // when set, the only tasks Get finds
}

func newFakeTasks() *fakeTasks {
	return &fakeTasks{status: map[string]taskapi.RunStatus{}, errs: map[string]string{}}
}

func (f *fakeTasks) Run(ctx context.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	resp, err := f.run(in)
	if err == nil && in.TaskDef["type"] == inboxapi.HumanTaskStrategy {
		err = askHuman(ctx, resp.RunID, in.Input)
	}
	return resp, err
}

// askHuman is what a human task's run does first: it puts its
// to-do, for a person, owned by the inbox, in the module of what it is about.
func askHuman(ctx context.Context, run string, input map[string]any) error {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return err
	}
	about, _ := input["about"].(string)
	module, err := cwmapi.ModuleOfObjectID(about)
	if err != nil {
		return err
	}
	kind, _ := input["kind"].(string)
	title, _ := input["title"].(string)
	_, err = todos.Put(ctx, schedapi.PutInput{Todo: schedapi.Todo{
		Module: module, Target: about, Title: title, For: schedapi.ForPerson, Start: schedapi.StartManual,
		Priority: "P2", State: schedapi.StateReady, Since: time.Now().UTC().Format(time.RFC3339), Owner: inboxapi.Owner,
		Work:  map[string]any{"doer": "person", "task": inboxapi.HumanTaskStrategy, inboxapi.WorkAsk: input, "interaction": kind, "run": run},
		Group: inboxapi.ProjectList, Key: inboxapi.HumanTaskStrategy + ":" + run,
	}})
	return err
}

func (f *fakeTasks) run(in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if why, ok := f.refuse[in.TaskName]; ok {
		return taskapi.TaskRunResponse{}, errors.New(why)
	}
	f.started = append(f.started, in)
	id := fmt.Sprintf("run-%d", len(f.started))
	f.status[id] = taskapi.RunStatusRunning
	// Like the task service: the caller's session, else a new one per run.
	session := in.SessionID
	if session == "" {
		session = "session-" + id
	}
	if f.sessions == nil {
		f.sessions = map[string]string{}
	}
	f.sessions[id] = session
	return taskapi.TaskRunResponse{RunID: id, TaskName: in.TaskName, Status: taskapi.RunStatusRunning, SessionID: session}, nil
}

func (f *fakeTasks) Status(_ context.Context, in taskapi.StatusRequest) (taskapi.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.status[in.RunID]
	if !ok {
		return taskapi.StatusResponse{}, errors.New("no run " + in.RunID)
	}
	if f.autoComplete && st == taskapi.RunStatusRunning {
		f.status[in.RunID] = taskapi.RunStatusCompleted
	}
	out := taskapi.StatusResponse{RunID: in.RunID, Status: st, SessionID: f.sessions[in.RunID]}
	for _, r := range f.active {
		if r.RunID == in.RunID {
			out.StartedAt = r.StartedAt
		}
	}
	return out, nil
}

func (f *fakeTasks) Result(_ context.Context, in taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resultAsked = append(f.resultAsked, in.RunID)
	return taskapi.ResultResponse{RunID: in.RunID, Status: string(f.status[in.RunID]), Error: f.errs[in.RunID]}, nil
}

// end sets how a run ended.
func (f *fakeTasks) end(run string, st taskapi.RunStatus, err string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[run], f.errs[run] = st, err
}

func (f *fakeTasks) runs() []taskapi.TaskRunRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]taskapi.TaskRunRequest{}, f.started...)
}

func (f *fakeTasks) Stop(_ context.Context, in taskapi.StopRequest) (taskapi.StopResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, in.RunID)
	f.status[in.RunID] = taskapi.RunStatusCancelled
	return taskapi.StopResponse{Stopped: true}, nil
}
func (f *fakeTasks) RunStages(context.Context, taskapi.RunStagesRequest) (taskapi.RunStagesResponse, error) {
	return taskapi.RunStagesResponse{}, nil
}
func (f *fakeTasks) Runs(context.Context, taskapi.ListRunsRequest) (taskapi.ListRunsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return taskapi.ListRunsResponse{Runs: append([]taskapi.RunInfo{}, f.active...)}, nil
}
func (f *fakeTasks) List(context.Context, taskapi.ListTasksRequest) (taskapi.ListTasksResponse, error) {
	return taskapi.ListTasksResponse{}, nil
}
func (f *fakeTasks) Get(_ context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.known != nil && !f.known[in.Name] {
		return taskapi.TaskInfo{}, fmt.Errorf("task %q not found", in.Name)
	}
	return taskapi.TaskInfo{Name: in.Name}, nil
}
func (f *fakeTasks) BackgroundManager() taskapi.BackgroundTaskManager { return nil }
func (f *fakeTasks) Repeat(context.Context, taskapi.RepeatRequest) (taskapi.TaskRunResponse, error) {
	return taskapi.TaskRunResponse{}, nil
}
func (f *fakeTasks) SuspensionManager() taskapi.SuspensionManager { return nil }

// fakePersonas stands for the persona library: it knows some personas, and a
// spawn is a run of the fake task service.
type fakePersonas struct {
	tasks  *fakeTasks
	known  map[string]bool
	mu     sync.Mutex
	spawns []personasapi.SpawnInput
}

func newFakePersonas(tasks *fakeTasks, names ...string) *fakePersonas {
	p := &fakePersonas{tasks: tasks, known: map[string]bool{}}
	for _, n := range names {
		p.known[n] = true
	}
	return p
}

func (p *fakePersonas) Resolve(_ context.Context, in personasapi.ResolveInput) (personasapi.ResolveOutput, error) {
	if !p.known[in.Name] {
		return personasapi.ResolveOutput{}, personasapi.ErrNotFound
	}
	return personasapi.ResolveOutput{Persona: personasapi.Info{Name: in.Name}}, nil
}

func (p *fakePersonas) List(context.Context, personasapi.ListInput) (personasapi.ListOutput, error) {
	return personasapi.ListOutput{}, nil
}

func (p *fakePersonas) Spawn(ctx context.Context, in personasapi.SpawnInput) (personasapi.SpawnOutput, error) {
	p.mu.Lock()
	p.spawns = append(p.spawns, in)
	p.mu.Unlock()
	run, err := p.tasks.Run(ctx, taskapi.TaskRunRequest{TaskName: "persona:" + in.Name, Input: in.Inputs, SessionID: in.SessionID})
	if err != nil {
		return personasapi.SpawnOutput{}, err
	}
	return personasapi.SpawnOutput{SpawnID: "spawn-" + run.RunID, TaskID: run.RunID, Status: "running"}, nil
}

// tasksOf is the world's fake task service.
func tasksOf(t *testing.T, ctx context.Context) *fakeTasks {
	t.Helper()
	svc, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc.(*fakeTasks)
}

func (f *fakeTasks) Groups(context.Context, taskapi.GroupsRequest) (taskapi.GroupsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return taskapi.GroupsResponse{Groups: append([]taskapi.GroupLoad{}, f.groups...)}, nil
}
func (f *fakeTasks) SetGroup(_ context.Context, in taskapi.SetGroupRequest) (taskapi.GroupLoad, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setGroups = append(f.setGroups, in)
	on := in.On != nil && *in.On
	return taskapi.GroupLoad{Name: in.Group, On: on}, nil
}
func (f *fakeTasks) ClearGroup(context.Context, taskapi.ClearGroupRequest) (taskapi.ClearGroupResponse, error) {
	return taskapi.ClearGroupResponse{}, nil
}

// eventuallyWithin is the one ceiling every wait of this package uses: what
// runs on its own goroutine (the observer, a hook) answers in milliseconds
// alone, but on a loaded two-core runner (: a pull
// waited 20 s) its reactions queue behind the other worlds. A ceiling never
// slows a passing test; a fixed short deadline makes it flaky.
const eventuallyWithin = 90 * time.Second

// answerFollowed waits until the module's observer followed the last write
// of inbox entry id: a question answered while nothing listened is answered
// before a request is linked to it, never after (Settle cannot see an event
// still queued).
func answerFollowed(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) {
	t.Helper()
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the answer of "+id+" followed", func() (string, bool) {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
		if err != nil {
			return err.Error(), false
		}
		return got.Revision, planlocal.Handled(ctx, id, got.Revision)
	})
}

func eventually(t *testing.T, what string, check func() (string, bool)) string {
	t.Helper()
	// Long enough for a loaded two-core runner, where the module's reactions
	// queue behind four worlds running at once.
	deadline := time.Now().Add(eventuallyWithin)
	for {
		v, ok := check()
		if ok {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s and still no %s (last seen: %s)", eventuallyWithin, what, v)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
