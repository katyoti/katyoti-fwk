// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
	commslocal "github.com/katyoti/katyoti-fwk/internal/extensions/comms/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/internal/humantask"
	inboxlocal "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// schedulerEnv is the universe's environment with its scheduler folder in dir.
type schedulerEnv struct {
	application.AppEnvironment
	dir string
}

func (e schedulerEnv) SchedulerDir() string { return e.dir }

// TestAChildsQuestionIsAnsweredInAUniverseWithNoProject: in a
// universe no project serves - no requests module, no sys-inbox, the ordinary
// self-contained layout - a delegated child's question, put on its parent's
// holding list, lands in the universe's scheduler folder; the parent takes and
// answers it; the inbox collects it (the to-do archived with its answer) and
// the child's run is woken. The real scheduler, no fake.
func TestAChildsQuestionIsAnsweredInAUniverseWithNoProject(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.NewMemoryEventBus()
	ctx, _ := cwmapp.NewStrict(t,
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) { return bus, nil }),
		func(init *application.ApplicationInit) {
			init.AddContribute("rq013", ".env", func(app *application.Application) error {
				application.PutService[application.AppEnvironment](app, "default", schedulerEnv{AppEnvironment: env(app), dir: dir})
				return nil
			})
		},
		schedlocal.Extension(),
		inboxcontrib.Pack("ktw-agents"),
		inboxcontrib.DeclareKinds("ktw-agents"),
		inboxcontrib.OwnAskKinds("ktw-agents"),
		inboxlocal.Extension(),
		inboxlocal.AnswersExtension(),
		commslocal.ListsExtension(),
	)
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}

	const parent, child = "run-parent", "run-child-ask"
	ask := api.HumanTask{Title: "color?", Question: "color?", Kind: "form", Label: "Answer to give", AnswerableBy: "agent", Candidates: commsapi.ParentList(parent), AskedBy: "run-child"}
	todo := humantask.TodoOf(ask, child, time.Now())
	todo.For, todo.Work["doer"] = schedapi.ForAgent, "member"
	put, err := todos.Put(ctx, schedapi.PutInput{Todo: todo})
	if err != nil {
		t.Fatalf("the child asks in a universe with no project: %v", err)
	}
	if want := filepath.Join(dir, schedapi.RequestsModule, "todo"); !exists(want) {
		t.Fatalf("its to-do %s is not in the universe's scheduler folder %s", put.Todo.ID, want)
	}
	listed, err := todos.List(ctx, schedapi.ListInput{})
	if err != nil || len(listed.Todos) != 1 || listed.Todos[0].ID != put.Todo.ID {
		t.Fatalf("the scheduled to-dos = %+v (%v), want the question %s", listed.Todos, err, put.Todo.ID)
	}

	woken := make(chan string, 1)
	bus.Subscribe(engine.HumanTaskAnsweredTopic(put.Todo.ID), func(_ string, p any) { woken <- p.(string) })
	by := commsapi.ParentActor(parent)
	pulled, err := todos.Pull(ctx, schedapi.PullInput{ID: put.Todo.ID, By: by})
	if err != nil || pulled.Todo == nil {
		t.Fatalf("the parent takes the question: %+v (%v)", pulled, err)
	}
	if _, err := todos.Report(ctx, schedapi.ReportInput{ID: put.Todo.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Answer: "blue", Summary: "blue", By: by}}); err != nil {
		t.Fatalf("the parent answers: %v", err)
	}
	got, err := todos.Get(ctx, schedapi.GetInput{ID: put.Todo.ID})
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := api.AnswerOf(got.Todo); !ok || got.Todo.Archived == "" || a.Answer != "blue" {
		t.Fatalf("the answered question = %+v (answer %v, %v), want it archived with its answer blue", got.Todo, a, ok)
	}
	select {
	case <-woken:
	case <-time.After(time.Second):
		t.Fatal("the child's run was not told its question is answered")
	}
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func env(app *application.Application) application.AppEnvironment {
	e, _ := application.AppUseService[application.AppEnvironment](app)
	return e
}
