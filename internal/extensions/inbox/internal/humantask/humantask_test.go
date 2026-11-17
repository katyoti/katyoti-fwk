// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// fakeTodos is the scheduler's to-dos in memory: Put (keyed), Get, Archive.
type fakeTodos struct {
	schedapi.TodoService
	mu       sync.Mutex
	todos    map[string]*schedapi.Todo
	puts     int
	archived []string
}

func (f *fakeTodos) Put(_ context.Context, in schedapi.PutInput) (schedapi.TodoOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if in.Todo.ID != "" {
		t := in.Todo
		f.todos[t.ID] = &t
		return schedapi.TodoOutput{Todo: t}, nil
	}
	for _, t := range f.todos {
		if in.Todo.Key != "" && t.Key == in.Todo.Key {
			return schedapi.TodoOutput{Todo: *t}, nil
		}
	}
	f.puts++
	t := in.Todo
	t.ID = t.Module + ".TD-" + time.Now().Format("150405.000000000")
	f.todos[t.ID] = &t
	return schedapi.TodoOutput{Todo: t}, nil
}

func (f *fakeTodos) Get(_ context.Context, in schedapi.GetInput) (schedapi.TodoOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.todos[in.ID]
	if !ok {
		return schedapi.TodoOutput{}, cwmapi.ErrNotFound
	}
	return schedapi.TodoOutput{Todo: *t}, nil
}

func (f *fakeTodos) Archive(_ context.Context, in schedapi.ArchiveInput) (schedapi.TodoOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.archived = append(f.archived, in.ID)
	return schedapi.TodoOutput{Todo: *f.todos[in.ID]}, nil
}

// List is the to-dos still scheduled, or the archived ones (done).
func (f *fakeTodos) List(_ context.Context, in schedapi.ListInput) (schedapi.TodosOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := schedapi.TodosOutput{Todos: []schedapi.Todo{}}
	for id, t := range f.todos {
		if slices.Contains(f.archived, id) == in.Done {
			out.Todos = append(out.Todos, *t)
		}
	}
	return out, nil
}

// answer is the person reporting the to-do.
func (f *fakeTodos) answer(id string, r schedapi.Result) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := f.todos[id]
	t.State = schedapi.StateReported
	t.Result = &r
}

func setup(t *testing.T) (context.Context, *fakeTodos, *eventbus.MemoryEventBus) {
	t.Helper()
	app := application.New(application.Config())
	todos := &fakeTodos{todos: map[string]*schedapi.Todo{}}
	bus := eventbus.NewMemoryEventBus()
	application.PutService[schedapi.TodoService](app, "default", todos)
	application.PutService[eventbus.EventBus](app, "default", bus)
	ctx := application.WithApplication(context.Background(), app)
	return engine.WithRunID(ctx, "run-1"), todos, bus
}

// universe is setup on a CWM universe with the inbox pack and sys-inbox
// filed: where answers are kept.
func universe(t *testing.T) (context.Context, *fakeTodos, *eventbus.MemoryEventBus, cwmapi.CWMService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t, inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	seedLists(t, ctx, cwm)
	app := application.AppFromContext(ctx)
	todos := &fakeTodos{todos: map[string]*schedapi.Todo{}}
	bus := eventbus.NewMemoryEventBus()
	application.PutService[schedapi.TodoService](app, "default", todos)
	application.PutService[eventbus.EventBus](app, "default", bus)
	return engine.WithRunID(ctx, "run-1"), todos, bus, cwm
}

func step(t *testing.T, ctx context.Context, m engine.Machine, st *engine.State, ev engine.Event) *engine.StepResult {
	t.Helper()
	act, err := m.Step(ctx, st, ev)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	res, err := act(ctx)
	if err != nil {
		t.Fatalf("action: %v", err)
	}
	st.Name, st.Data = res.NextState, res.Data
	return res
}

func machineFor(t *testing.T, config, input map[string]any) engine.Machine {
	t.Helper()
	withKinds(t, builtinKinds(t))
	m, err := factory{}.CreateMachine(context.Background(), &taskapi.TaskExecRequest{Config: config, Input: input})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

// TestAHumanTaskAsksOnceAndItsResultIsTheAnswer: a human task
// puts one person's to-do owned by the inbox, in the module of what it is
// about, and waits durably on its answer; woken before the answer it waits
// again without asking again; once the to-do is reported, the run ends with
// the answer as its result.
func TestAHumanTaskAsksOnceAndItsResultIsTheAnswer(t *testing.T) {
	ctx, todos, _ := setup(t)
	m := machineFor(t, map[string]any{"kind": "approve", "about": "ui.F-061"},
		map[string]any{"title": "Validate the switch", "satisfies": []any{"ui.F-061/validate"}})
	st := m.InitialState(engine.Event{})

	res := step(t, ctx, m, &st, engine.Event{Type: engine.TickEvent})
	if res.Wait == nil || res.Output != nil {
		t.Fatalf("first step: wait %v output %v; want a wait and no result", res.Wait, res.Output)
	}
	if todos.puts != 1 {
		t.Fatalf("puts = %d; want 1", todos.puts)
	}
	id := dataOf(st.Data).Todo
	todo := todos.todos[id]
	if todo.For != schedapi.ForPerson || todo.Owner != api.Owner || todo.Module != "ui" || todo.Target != "ui.F-061" ||
		todo.Key != "human-task:run-1" || todo.State != schedapi.StateReady {
		t.Errorf("the to-do = %+v; want a ready person's to-do of the inbox in ui about ui.F-061 keyed by its run", *todo)
	}
	if got, _ := SpecOf(todo.Work[api.WorkAsk].(map[string]any)); got.Kind != "approve" || got.Title != "Validate the switch" || len(got.Satisfies) != 1 || got.About != "ui.F-061" {
		t.Errorf("what it asks = %+v", got)
	}
	if c := res.Wait.Conditions; len(c) != 1 || c[0].Topic != engine.HumanTaskAnsweredTopic(id) || !res.Wait.RecheckOnResume || res.Wait.Timeout == 0 {
		t.Errorf("the wait = %+v; want its answered topic, rechecked on resume and on a timeout", res.Wait)
	}

	res = step(t, ctx, m, &st, engine.Event{Type: "recheck", Label: "recheck"})
	if res.Wait == nil || todos.puts != 1 {
		t.Fatalf("woken before the answer: wait %v, puts %d; want it waiting again, asked once", res.Wait, todos.puts)
	}

	todos.answer(id, schedapi.Result{Status: schedapi.StatusSuccess, Verdict: "approved", Summary: "ok",
		By: cwmapi.Actor{ID: "jdoe", Kind: "person"}, At: "2026-10-23T16:00:00Z"})
	res = step(t, ctx, m, &st, engine.Event{Type: engine.HumanTaskAnsweredTopic(id), Label: "answered"})
	a, ok := res.Output.(api.HumanAnswer)
	if !ok || a.Task != id || a.Verdict != "approved" || a.By.ID != "jdoe" {
		t.Fatalf("result = %#v; want the person's answer", res.Output)
	}
	if res.Wait != nil || res.NextState != "done" {
		t.Errorf("answered: next %q wait %v; want done", res.NextState, res.Wait)
	}
}

// TestAHumanTaskSurvivesARestart: restored from its checkpoint (its data as
// JSON) after the answer came while no run listened, a human task reads its
// to-do at once, asks nothing again and ends with the answer.
func TestAHumanTaskSurvivesARestart(t *testing.T) {
	ctx, todos, _ := setup(t)
	cfg := map[string]any{"kind": "provide", "title": "Which currency?"}
	m := machineFor(t, cfg, nil)
	st := m.InitialState(engine.Event{})
	step(t, ctx, m, &st, engine.Event{Type: engine.TickEvent})
	id := dataOf(st.Data).Todo
	todos.answer(id, schedapi.Result{Status: schedapi.StatusSuccess, Answer: "euros", By: cwmapi.Actor{ID: "jdoe", Kind: "person"}})

	raw, _ := json.Marshal(st)
	var restored engine.State
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	m2 := machineFor(t, cfg, nil)
	res := step(t, ctx, m2, &restored, engine.Event{Type: engine.TickEvent, Replayed: true})
	if a, ok := res.Output.(api.HumanAnswer); !ok || a.Answer != "euros" {
		t.Fatalf("after the restart: %#v; want the answer", res.Output)
	}
	if todos.puts != 1 {
		t.Errorf("puts = %d; want it asked once", todos.puts)
	}
}

// TestAHumanTaskWhoseToDoIsGoneFails: no answer will come - the run fails
// instead of waiting forever.
func TestAHumanTaskWhoseToDoIsGoneFails(t *testing.T) {
	ctx, todos, _ := setup(t)
	m := machineFor(t, map[string]any{"kind": "test-by-hand", "title": "Test by hand"}, nil)
	st := m.InitialState(engine.Event{})
	step(t, ctx, m, &st, engine.Event{Type: engine.TickEvent})
	delete(todos.todos, dataOf(st.Data).Todo)
	act, _ := m.Step(ctx, &st, engine.Event{Type: "recheck"})
	if _, err := act(ctx); err == nil {
		t.Fatal("a human task whose to-do is gone waits; want it failed")
	}
}

// TestCollectArchivesAndWakesTheRun: the inbox's collector archives the
// answered to-do and publishes its answered topic.
func TestCollectArchivesAndWakesTheRun(t *testing.T) {
	ctx, todos, bus, _ := universe(t)
	todos.todos["ui.TD-1"] = &schedapi.Todo{ID: "ui.TD-1", Owner: api.Owner, Work: map[string]any{"task": api.HumanTaskStrategy},
		Result: &schedapi.Result{Status: schedapi.StatusSuccess, By: cwmapi.Actor{ID: "jdoe", Kind: "person"}}}
	woken := make(chan string, 1)
	bus.Subscribe(engine.HumanTaskAnsweredTopic("ui.TD-1"), func(_ string, p any) { woken <- p.(string) })
	if err := Collect(ctx, *todos.todos["ui.TD-1"]); err != nil {
		t.Fatal(err)
	}
	if len(todos.archived) != 1 || todos.archived[0] != "ui.TD-1" {
		t.Errorf("archived %v; want ui.TD-1", todos.archived)
	}
	select {
	case id := <-woken:
		if id != "ui.TD-1" {
			t.Errorf("woken with %q", id)
		}
	case <-time.After(time.Second):
		t.Error("the run was not told")
	}
}

// TestWhatAHumanTaskAsks: a question is required; the input overrides the
// config; a system module's object, or none, puts the to-do in the requests
// module.
func TestWhatAHumanTaskAsks(t *testing.T) {
	if _, err := SpecOf(map[string]any{"kind": "approve"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("no question: %v; want refused", err)
	}
	s, err := SpecOf(&taskapi.TaskExecRequest{Config: map[string]any{"title": "a", "kind": "approve"}, Input: map[string]any{"title": "b"}})
	if err != nil || s.Title != "b" || s.Kind != "approve" {
		t.Errorf("merged = %+v, %v; want title b, kind approve", s, err)
	}
	if s, _ := SpecOf(map[string]any{"kind": "choose", "question": "Which one?\nThe details."}); s.Title != "Which one?" {
		t.Errorf("title from the question = %q", s.Title)
	}
	for about, want := range map[string]string{"ui.F-061": "ui", "sys-inbox.x": schedapi.RequestsModule, "": schedapi.RequestsModule} {
		if got := ModuleOf(about); got != want {
			t.Errorf("ModuleOf(%q) = %q; want %q", about, got, want)
		}
	}
}

// builtinKinds are the kinds the inbox pack ships (contrib/embed/ask-kinds),
// read from its files as the catalog serves them.
func builtinKinds(t testing.TB) []api.AskKind {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "contrib", "embed", "ask-kinds", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("the built-in kinds: %v (%d files)", err, len(files))
	}
	var out []api.AskKind
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var k api.AskKind
		if err := config.DecodeBytes(raw, marshal.YAML, &k); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out = append(out, k)
	}
	return out
}

// withKinds makes the catalog answer these kinds of questions for the test.
func withKinds(t *testing.T, kinds []api.AskKind) {
	t.Helper()
	was := api.QueryAskKinds
	api.QueryAskKinds = func(context.Context) ([]api.AskKind, error) { return kinds, nil }
	t.Cleanup(func() { api.QueryAskKinds = was })
}

// TestAHumanTaskWithoutItsKindIsRefused: every question says
// first what is expected of the person - a human task without its kind, or
// with a kind no pack contributes, is never asked.
func TestAHumanTaskWithoutItsKindIsRefused(t *testing.T) {
	withKinds(t, builtinKinds(t))
	for _, spec := range []map[string]any{{"title": "Is it right?"}, {"kind": "  ", "title": "Is it right?"}} {
		if _, err := SpecOf(spec); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%v: %v; want refused", spec, err)
		}
	}
	if _, err := factory.CreateMachine(factory{}, context.Background(), map[string]any{"kind": "guess", "title": "Is it right?"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("an unknown kind: %v; want refused", err)
	}
	kinds, _ := Kinds(context.Background())
	for _, k := range kinds {
		if _, err := factory.CreateMachine(factory{}, context.Background(), map[string]any{"kind": k.Name, "title": "Is it right?"}); err != nil {
			t.Errorf("kind %s refused: %v", k.Name, err)
		}
	}
}

// TestEveryHumanTasksToDoCarriesItsKindsLabel: the label every
// channel shows first is on the to-do they all read; the nine built-in kinds
// are the study's.
func TestEveryHumanTasksToDoCarriesItsKindsLabel(t *testing.T) {
	withKinds(t, builtinKinds(t))
	want := map[string]string{"approve": "Validation", "decide": "Decision", "test-by-hand": "Test by hand", "form": "Answer to give", "provide": "Data to provide", "permission": "Permission"}
	for kind, label := range want {
		s, err := Resolve(context.Background(), api.HumanTask{Kind: kind, Title: "Is it right?"})
		if err != nil || s.Label != label {
			t.Errorf("Resolve(%s) = label %q, %v; want %q", kind, s.Label, err, label)
		}
		td := TodoOf(s, "run-1", time.Now())
		if td.Work[api.WorkLabel] != label {
			t.Errorf("the to-do of a %s carries label %v; want %q", kind, td.Work[api.WorkLabel], label)
		}
	}
	kinds, _ := Kinds(context.Background())
	names := []string{}
	for _, k := range kinds {
		names = append(names, k.Name)
	}
	if strings.Join(names, ",") != "approve,decide,test-by-hand,form,provide,choose,permission,unblock,accept" {
		t.Errorf("built-in kinds %v; want the nine of the study, in the order of their counts", names)
	}
}

// TestKindsOfQuestionsAreContributions: a pack's own kind is
// asked like a built-in one; its defaults - who may answer, the permission
// code, what its answer settles - apply where the human task says nothing;
// a kind contributed twice is offered once; a catalog that cannot answer
// asks nothing.
func TestKindsOfQuestionsAreContributions(t *testing.T) {
	own := api.AskKind{Name: "sign-off", Label: "Sign-off", AnswerableBy: "person", Requires: "release:sign-off", Satisfies: []string{"gate/ready-to-release/sign-off"}, Order: 10}
	withKinds(t, append(builtinKinds(t), own, api.AskKind{Name: "approve", Label: "Again"}))
	s, err := Resolve(context.Background(), api.HumanTask{Kind: "sign-off", Title: "Sign the release off"})
	if err != nil || s.Label != "Sign-off" || s.AnswerableBy != "person" || s.Requires != "release:sign-off" || len(s.Satisfies) != 1 {
		t.Errorf("a pack's kind = %+v, %v; want its label and its defaults", s, err)
	}
	s, _ = Resolve(context.Background(), api.HumanTask{Kind: "sign-off", Title: "x", AnswerableBy: "any", Requires: "release:x", Satisfies: []string{"a", "b"}})
	if s.AnswerableBy != "any" || s.Requires != "release:x" || len(s.Satisfies) != 2 {
		t.Errorf("what the task says = %+v; want it kept over the kind's defaults", s)
	}
	kinds, _ := Kinds(context.Background())
	if len(kinds) != 10 || kinds[0].Name != "approve" || kinds[0].Label != "Validation" || kinds[9].Name != "sign-off" {
		t.Errorf("kinds = %+v; want the nine then the pack's, approve once", kinds)
	}
	api.QueryAskKinds = func(context.Context) ([]api.AskKind, error) { return nil, errors.New("no catalog") }
	if _, err := Resolve(context.Background(), api.HumanTask{Kind: "approve", Title: "x"}); err == nil {
		t.Error("asked with no catalog to say the kind exists; want refused")
	}
}

// TestOneNeedIsAskedOnce: a test by hand and its validation are
// one card - the test's one answer settles the step's validation too; a
// question whose needs an open human task holds waits on it, one whose needs a
// proven answer settled is answered by it, and a refused answer leaves the
// need open to ask again.
func TestOneNeedIsAskedOnce(t *testing.T) {
	ctx, todos, _, _ := universe(t)
	const verified, validated = "verification:shop.RQ-041", "step:shop.RQ-041/validate"
	test := machineFor(t, map[string]any{"kind": "test-by-hand", "title": "Test the export by hand", "about": "shop.RQ-041",
		"satisfies": []any{verified, validated}}, nil)
	st := test.InitialState(engine.Event{})
	res := step(t, ctx, test, &st, engine.Event{})
	first := dataOf(res.Data).Todo

	// The step's validation, asked while the test waits: no second card.
	validate := machineFor(t, map[string]any{"kind": "approve", "title": "Validate the step", "about": "shop.RQ-041",
		"satisfies": []any{validated}}, nil)
	st2 := validate.InitialState(engine.Event{})
	res2 := step(t, ctx, validate, &st2, engine.Event{})
	if got := dataOf(res2.Data).Todo; got != first || todos.puts != 1 {
		t.Fatalf("the validation asked %s (%d puts); want it to wait on the test's card %s, one put", got, todos.puts, first)
	}

	// One answer settles both.
	todos.answer(first, schedapi.Result{Status: schedapi.StatusSuccess, Summary: "the export opens in Word", By: cwmapi.Actor{ID: "jdoe", Kind: "person"}, At: "2026-10-23T18:00:00Z"})
	if a, ok := step(t, ctx, validate, &st2, engine.Event{}).Output.(api.HumanAnswer); !ok || a.Task != first {
		t.Fatalf("the validation's answer %+v; want the test's", a)
	}
	if err := Collect(ctx, *todos.todos[first]); err != nil {
		t.Fatal(err)
	}

	// Asked again once answered: answered at once, nothing put.
	again := machineFor(t, map[string]any{"kind": "approve", "title": "Validate the step", "satisfies": []any{validated}}, nil)
	st3 := again.InitialState(engine.Event{})
	if a, ok := step(t, engine.WithRunID(ctx, "run-3"), again, &st3, engine.Event{}).Output.(api.HumanAnswer); !ok || a.Task != first || todos.puts != 1 {
		t.Fatalf("a settled need asked again: %+v (%d puts); want the settled answer, no new card", a, todos.puts)
	}
	if a, ok, err := api.SettledBy(ctx, todos, verified); err != nil || !ok || a.Task != first {
		t.Errorf("SettledBy(%s) = %+v %v %v; want the test's answer", verified, a, ok, err)
	}
	if _, ok, _ := api.SettledBy(ctx, todos, "step:other/validate"); ok {
		t.Error("a need no answer settled reads settled")
	}
}

func TestARefusedAnswerLeavesTheNeedOpen(t *testing.T) {
	ctx, todos, _, _ := universe(t)
	need := []any{"step:ui.F-061/validate"}
	m := machineFor(t, map[string]any{"kind": "approve", "title": "Validate the switch", "satisfies": need}, nil)
	st := m.InitialState(engine.Event{})
	id := dataOf(step(t, ctx, m, &st, engine.Event{}).Data).Todo
	todos.answer(id, schedapi.Result{Status: schedapi.StatusSuccess, Verdict: "rejected", By: cwmapi.Actor{ID: "jdoe", Kind: "person"}, At: "2026-10-23T18:00:00Z"})
	step(t, ctx, m, &st, engine.Event{})
	if err := Collect(ctx, *todos.todos[id]); err != nil {
		t.Fatal(err)
	}
	again := machineFor(t, map[string]any{"kind": "approve", "title": "Validate the switch again", "satisfies": need}, nil)
	st2 := again.InitialState(engine.Event{})
	if got := dataOf(step(t, engine.WithRunID(ctx, "run-2"), again, &st2, engine.Event{}).Data).Todo; got == id || todos.puts != 2 {
		t.Errorf("after a rejection the need was not asked again (%s, %d puts)", got, todos.puts)
	}
	if _, ok, _ := api.SettledBy(ctx, todos, "step:ui.F-061/validate"); ok {
		t.Error("a rejected validation reads settled")
	}
}

// TestAChoiceAndAFormKeepTheirOptionsAndDefinition: what
// request-validation asks - a choose question's options, a form's
// definition - reaches the human task's to-do as fields its card draws, not
// as text appended to the question, and reads back from the stored to-do.
func TestAChoiceAndAFormKeepTheirOptionsAndDefinition(t *testing.T) {
	withKinds(t, builtinKinds(t))
	const form = "fields:\n  - name: reason\n    type: text\n"
	for _, in := range []string{
		`{"kind":"choose","title":"Which store?","question":"Pick one.","options":["files","sqlite"]}`,
		`{"kind":"form","title":"Describe the export","form_def":` + strconvQuote(form) + `}`,
	} {
		var asked map[string]any // what request-validation starts the human task with
		if err := json.Unmarshal([]byte(in), &asked); err != nil {
			t.Fatal(err)
		}
		s, err := SpecOf(asked)
		if err == nil {
			s, err = Resolve(context.Background(), s)
		}
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		raw, err := json.Marshal(TodoOf(s, "run-1", time.Now())) // as the scheduler stores it
		if err != nil {
			t.Fatal(err)
		}
		var stored schedapi.Todo
		if err := json.Unmarshal(raw, &stored); err != nil {
			t.Fatal(err)
		}
		got, ok := api.AskOf(stored)
		if !ok {
			t.Fatalf("%s: the stored to-do reads as no human task", in)
		}
		switch got.Kind {
		case "choose":
			if !slices.Equal(got.Options, []string{"files", "sqlite"}) || got.Question != "Pick one." {
				t.Errorf("choose: options %v, question %q; want the two options kept apart from the question", got.Options, got.Question)
			}
		case "form":
			if got.FormDef != form {
				t.Errorf("form: definition %q; want %q", got.FormDef, form)
			}
		}
	}
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

// TestThePermissionKindDeclaresItsAnswer: the
// permission kind's answer is its data-type - allow or deny, once or always,
// and the rule an always keeps -, and every answer a built-in kind declares is
// a data-type the inbox pack ships.
func TestThePermissionKindDeclaresItsAnswer(t *testing.T) {
	withKinds(t, builtinKinds(t))
	s, err := Resolve(context.Background(), api.HumanTask{Kind: api.KindPermission, Title: "Allow t_write?"})
	if err != nil {
		t.Fatal(err)
	}
	kinds, _ := Kinds(context.Background())
	declared := map[string]string{}
	for _, k := range kinds {
		declared[k.Name] = k.Answer
	}
	if declared[api.KindPermission] != "ktw-agents:/data-type/permission-answer" || s.Label != "Permission" {
		t.Errorf("the permission kind's answer = %q; want ktw-agents:/data-type/permission-answer", declared[api.KindPermission])
	}
	for name, uri := range declared {
		if uri == "" {
			continue
		}
		dt := strings.TrimPrefix(uri, "ktw-agents:/data-type/")
		raw, err := os.ReadFile(filepath.Join("..", "..", "contrib", "embed", "cwm", "data-types", dt+".dsl.yaml"))
		if err != nil || !strings.Contains(string(raw), "\nname: "+dt+"\n") {
			t.Errorf("kind %s answers %s: the inbox pack ships no such data-type (%v)", name, uri, err)
		}
	}
	raw, _ := os.ReadFile(filepath.Join("..", "..", "contrib", "embed", "cwm", "data-types", "permission-answer.dsl.yaml"))
	for _, want := range []string{"allow-once", "allow-always", "deny-once", "deny-always", "rule:"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("permission-answer lacks %s", want)
		}
	}
}
