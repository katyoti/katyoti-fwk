// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

var (
	alice  = cwmapi.Actor{ID: "session-a", Kind: "agent"}
	bob    = cwmapi.Actor{ID: "session-b", Kind: "agent"}
	person = cwmapi.Actor{ID: "jdoe", Kind: "person"}
)

// collected is what the test owner's collector saw.
type collected struct {
	mu  sync.Mutex
	ids []string
}

func (c *collected) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.ids...)
}

func world(t *testing.T, extra ...application.Extension) (context.Context, cwmapi.CWMService, api.TodoService, *collected) {
	t.Helper()
	got := &collected{}
	exts := append([]application.Extension{
		// A to-do's module is an object carrying the module capability.
		cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")}}),
		schedlocal.Extension(),
		func(init *application.ApplicationInit) {
			init.AddContribute("scheduler-test", ".collector", func(app *application.Application) error {
				return schedlocal.ContributeCollector(app, api.Collector{Owner: "test", Collect: func(ctx context.Context, td api.Todo) error {
					got.mu.Lock()
					got.ids = append(got.ids, td.ID)
					got.mu.Unlock()
					svc, err := application.UseService[api.TodoService](ctx)
					if err != nil {
						return err
					}
					_, err = svc.Archive(ctx, api.ArchiveInput{ID: td.ID, Note: "collected"})
					return err
				}})
			})
		},
	}, extra...)
	ctx, cwm := cwmapp.New(t, exts...)
	for _, m := range []string{"billing", "shipping"} {
		cwmapp.Module(t, ctx, cwm, m)
	}
	svc, err := application.UseService[api.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc, got
}

func put(t *testing.T, ctx context.Context, svc api.TodoService, td api.Todo) api.Todo {
	t.Helper()
	if td.Owner == "" {
		td.Owner = "test"
	}
	if td.For == "" {
		td.For = api.ForAgent
	}
	if td.Start == "" {
		td.Start = api.StartAuto
	}
	if td.Priority == "" {
		td.Priority = "P2"
	}
	if td.State == "" {
		td.State = api.StateReady
	}
	if td.Target == "" {
		td.Target = td.Module
	}
	out, err := svc.Put(ctx, api.PutInput{Todo: td})
	if err != nil {
		t.Fatal(err)
	}
	return out.Todo
}

// The queue hands out the ready to-dos by priority, a module written held
// against its readers; a pulled to-do is the worker's for its lease; a person's
// to-do is a person's; the result reported is collected by its owner, which
// archives it to done — the files say so.
func TestTheSchedulerHandsOutAndCollects(t *testing.T) {
	ctx, cwm, svc, got := world(t)
	write := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement", Writes: []string{"billing"}})
	read := put(t, ctx, svc, api.Todo{Module: "billing", Title: "analyse", Priority: "P1", Reads: []string{"billing"}})
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "later", State: api.StateWaiting})
	ask := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", For: api.ForPerson, Start: api.StartManual})
	ship := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "pack", Priority: "P3"})

	first, err := svc.Pull(ctx, api.PullInput{By: alice})
	if err != nil {
		t.Fatal(err)
	}
	if first.Todo == nil || first.Todo.ID != read.ID || first.Todo.State != api.StateClaimed || first.Todo.Claim.By.ID != alice.ID {
		t.Fatalf("first pull: %+v; want the P1 analyse, held by session-a", first)
	}
	second, _ := svc.Pull(ctx, api.PullInput{By: bob})
	if second.Todo == nil || second.Todo.ID != ship.ID || len(second.Waiting) != 1 || !strings.HasPrefix(second.Waiting[0], write.ID) {
		t.Fatalf("second pull: %+v; want shipping's pack, the implement waiting (billing is read)", second)
	}
	if none, _ := svc.Pull(ctx, api.PullInput{By: bob, Filter: api.Filter{Module: "billing"}}); none.Todo != nil || none.Nothing == "" {
		t.Errorf("billing for session-b: %+v; want nothing, saying why", none)
	}
	if _, err := svc.Renew(ctx, api.RenewInput{ID: read.ID, By: bob}); err == nil {
		t.Error("session-b renewed session-a's to-do")
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: read.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "done", By: bob}}); err == nil {
		t.Error("session-b reported session-a's to-do")
	}
	rep, err := svc.Report(ctx, api.ReportInput{ID: read.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "analysed", By: alice}})
	if err != nil {
		t.Fatal(err)
	}
	if ids := got.seen(); len(ids) != 1 || ids[0] != read.ID || !strings.Contains(rep.Note, "collected by test") {
		t.Fatalf("collected %v (%q); want the analyse", ids, rep.Note)
	}
	done, err := svc.Get(ctx, api.GetInput{ID: read.ID})
	if err != nil || done.Todo.Archived == "" || done.Todo.Result == nil || done.Todo.Result.Summary != "analysed" {
		t.Fatalf("the analyse after its owner collected it: %+v, %v", done.Todo, err)
	}
	space := planning(t, ctx, cwm, "billing")
	if _, err := os.Stat(filepath.Join(space, "done", strings.TrimPrefix(read.ID, "billing.")+".yaml")); err != nil {
		t.Errorf("the done file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(space, "todo", strings.TrimPrefix(read.ID, "billing.")+".yaml")); !os.IsNotExist(err) {
		t.Errorf("still in todo: %v", err)
	}

	// The reader done, the writer goes; a person's to-do is a person's.
	if next, _ := svc.Pull(ctx, api.PullInput{By: alice}); next.Todo == nil || next.Todo.ID != write.ID {
		t.Fatalf("after the analyse: %+v; want the implement", next)
	}
	if _, err := svc.Pull(ctx, api.PullInput{ID: ask.ID, By: alice}); err != nil {
		t.Fatal(err)
	} else if p, _ := svc.Get(ctx, api.GetInput{ID: ask.ID}); p.Todo.State != api.StateReady {
		t.Errorf("an agent took a person's to-do: %+v", p.Todo)
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: ask.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "fine", Verdict: "approved", By: person}}); err != nil {
		t.Errorf("the person marks their review done: %v", err)
	}
}

// A lease that passes makes the to-do anyone's; a module's mode — manual by
// default, auto, off — says who may take its to-dos.
func TestLeasesAndModes(t *testing.T) {
	ctx, _, svc, _ := world(t)
	td := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement"})
	if _, err := svc.Pull(ctx, api.PullInput{By: alice, LeaseSeconds: 1}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	again, err := svc.Pull(ctx, api.PullInput{By: bob})
	if err != nil || again.Todo == nil || again.Todo.ID != td.ID || again.Todo.Claim.By.ID != bob.ID {
		t.Fatalf("after session-a's lease passed: %+v, %v", again, err)
	}
	if _, err := svc.Release(ctx, api.ReleaseInput{ID: td.ID, By: bob}); err != nil {
		t.Fatal(err)
	}

	modes, _ := svc.Modes(ctx, api.ModesInput{})
	for _, m := range modes.Modules {
		if m.Mode != api.ModeManual {
			t.Errorf("%s at first: %s, want manual", m.Module, m.Mode)
		}
	}
	if auto, _ := svc.Pull(ctx, api.PullInput{By: alice, Auto: true}); auto.Todo != nil {
		t.Errorf("the internal scheduler took a manual module's to-do: %+v", auto.Todo)
	}
	if _, err := svc.Mode(ctx, api.ModeInput{Module: "billing", Mode: api.ModeAuto, By: person}); err != nil {
		t.Fatal(err)
	}
	manual := put(t, ctx, svc, api.Todo{Module: "billing", Title: "by hand only", Start: api.StartManual, Priority: "P0"})
	if auto, _ := svc.Pull(ctx, api.PullInput{By: alice, Auto: true}); auto.Todo == nil || auto.Todo.ID != td.ID {
		t.Errorf("the internal scheduler in an auto module: %+v; want the auto to-do, not the manual one", auto.Todo)
	}
	if _, err := svc.Mode(ctx, api.ModeInput{Module: "billing", Mode: api.ModeOff, By: person}); err != nil {
		t.Fatal(err)
	}
	if off, _ := svc.Pull(ctx, api.PullInput{By: bob, ID: manual.ID}); off.Todo != nil {
		t.Errorf("a module off handed out %+v", off.Todo)
	}
	if _, err := svc.Mode(ctx, api.ModeInput{Module: "billing", Mode: "sometimes", By: person}); err == nil {
		t.Error("a mode of no kind was set")
	}
}

// Done to-dos are kept for the module's done_days, then pruned.
func TestDoneToDosArePruned(t *testing.T) {
	ctx, cwm, svc, _ := world(t)
	td := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement"})
	if _, err := svc.Archive(ctx, api.ArchiveInput{ID: td.ID}); err != nil {
		t.Fatal(err)
	}
	if out, _ := svc.Prune(ctx, api.PruneInput{}); out.Removed != 0 {
		t.Fatalf("pruned a fresh one: %+v", out)
	}
	path := filepath.Join(planning(t, ctx, cwm, "billing"), "done", strings.TrimPrefix(td.ID, "billing.")+".yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -31).Format(time.RFC3339Nano)
	lines := strings.Split(string(raw), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "archived:") {
			lines[i] = "archived: \"" + old + "\""
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _ := svc.Prune(ctx, api.PruneInput{}); out.Removed != 1 {
		t.Errorf("pruned %+v; want the one past 30 days", out)
	}
}

// planning is a module's .ktw folder.
func planning(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, module string) string {
	t.Helper()
	got, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: module})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(got.Path)
	if filepath.Base(dir) == "objects" {
		dir = filepath.Dir(dir)
	}
	return filepath.Join(dir, ".ktw")
}

// The store holds under concurrent use: a to-do is written over and over while
// others read it and list the folder, and to-dos are archived meanwhile — no
// write is refused, no read or listing fails.
func TestTheStoreHoldsUnderConcurrentUse(t *testing.T) {
	ctx, _, svc, _ := world(t)
	hot := put(t, ctx, svc, api.Todo{Module: "billing", Title: "written over and over"})
	var gone []string
	for i := range 20 {
		gone = append(gone, put(t, ctx, svc, api.Todo{Module: "billing", Title: fmt.Sprintf("archived %d", i), State: api.StateWaiting}).ID)
	}
	done := make(chan struct{})
	errs := make(chan error, 64)
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if _, err := svc.Get(ctx, api.GetInput{ID: hot.ID}); err != nil {
					errs <- fmt.Errorf("get: %w", err)
					return
				}
				if _, err := svc.List(ctx, api.ListInput{Module: "billing"}); err != nil {
					errs <- fmt.Errorf("list: %w", err)
					return
				}
			}
		}()
	}
	for i := range 60 {
		cur, err := svc.Get(ctx, api.GetInput{ID: hot.ID})
		if err != nil {
			errs <- fmt.Errorf("read %d: %w", i, err)
			break
		}
		td := cur.Todo
		td.Title = fmt.Sprintf("written %d", i)
		if _, err := svc.Put(ctx, api.PutInput{Todo: td}); err != nil {
			errs <- fmt.Errorf("write %d: %w", i, err)
			break
		}
		if i < len(gone) {
			if _, err := svc.Archive(ctx, api.ArchiveInput{ID: gone[i], Note: "gone"}); err != nil {
				errs <- fmt.Errorf("archive: %w", err)
				break
			}
		}
	}
	close(done)
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// A system module holds no to-dos: a to-do put there is refused, told apart by
// the system-module capability; and the scheduler's module list leaves it out
// - except the requests module, where unrouted requests are evaluated.
func TestASystemModuleHoldsNoToDos(t *testing.T) {
	ctx, cwm, svc, _ := world(t, cwmapp.SystemModules(t))
	if _, err := cwmapi.EnsureSystemModule(ctx, cwm, "sys-ops", "operational state"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Put(ctx, api.PutInput{Todo: api.Todo{Module: "sys-ops", Target: "sys-ops", Title: "x", Owner: "test",
		For: api.ForAgent, Start: api.StartAuto, Priority: "P2", State: api.StateReady}})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "system module") {
		t.Fatalf("a to-do in a system module: %v; want refused as a system module", err)
	}
	modes, err := svc.Modes(ctx, api.ModesInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range modes.Modules {
		if m.Module == "sys-ops" {
			t.Errorf("the scheduler lists the system module: %+v", modes.Modules)
		}
	}
	if len(modes.Modules) != 2 {
		t.Errorf("modules = %+v; want billing and shipping", modes.Modules)
	}
	// The requests module is the one exception: unrouted requests and demands
	// from outside are evaluated there.
	if _, err := cwmapi.EnsureSystemModule(ctx, cwm, api.RequestsModule, "requests"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Put(ctx, api.PutInput{Todo: api.Todo{Module: api.RequestsModule, Target: api.RequestsModule, Title: "intake", Owner: "test",
		For: api.ForAgent, Start: api.StartAuto, Priority: "P2", State: api.StateReady}}); err != nil {
		t.Fatalf("a to-do in the requests module: %v; want it queued", err)
	}
	if modes, _ = svc.Modes(ctx, api.ModesInput{}); len(modes.Modules) != 3 {
		t.Errorf("modules = %+v; want billing, shipping and the requests module", modes.Modules)
	}
}

// A module's lock is that module's only: a reader or a
// writer of billing and a writer of invoices, billing's sub-module, are pulled
// together; a step that changes invoices too, listing it in its writes, makes
// invoices' steps wait.
func TestAParentsStepDoesNotBlockItsSubModulesSteps(t *testing.T) {
	sub := func(t *testing.T) (context.Context, api.TodoService) {
		ctx, cwm, svc, _ := world(t)
		child := cwmapi.NewContainerObject("invoices", "invoices", "module", "billing", "invoices", "")
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "invoices", Object: child}); err != nil {
			t.Fatal(err)
		}
		return ctx, svc
	}

	t.Run("a reader of the parent, a writer of the child", func(t *testing.T) {
		ctx, svc := sub(t)
		read := put(t, ctx, svc, api.Todo{Module: "billing", Title: "analyse", Priority: "P1", Reads: []string{"billing"}})
		write := put(t, ctx, svc, api.Todo{Module: "invoices", Title: "implement", Writes: []string{"invoices"}})
		first, err := svc.Pull(ctx, api.PullInput{By: alice})
		if err != nil || first.Todo == nil || first.Todo.ID != read.ID {
			t.Fatalf("first pull: %+v, %v; want billing's analyse", first, err)
		}
		second, err := svc.Pull(ctx, api.PullInput{By: bob})
		if err != nil || second.Todo == nil || second.Todo.ID != write.ID {
			t.Fatalf("second pull: %+v, %v; want invoices' implement beside billing's reader", second, err)
		}
	})

	t.Run("a writer of the parent beside a writer of the child", func(t *testing.T) {
		ctx, svc := sub(t)
		parent := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement", Priority: "P1", Writes: []string{"billing"}})
		child := put(t, ctx, svc, api.Todo{Module: "invoices", Title: "implement", Writes: []string{"invoices"}})
		first, err := svc.Pull(ctx, api.PullInput{By: alice})
		if err != nil || first.Todo == nil || first.Todo.ID != parent.ID {
			t.Fatalf("first pull: %+v, %v; want billing's implement", first, err)
		}
		second, err := svc.Pull(ctx, api.PullInput{By: bob})
		if err != nil || second.Todo == nil || second.Todo.ID != child.ID {
			t.Fatalf("second pull: %+v, %v; want invoices' implement beside billing's writer", second, err)
		}
	})

	t.Run("a step changing the child too, listing it", func(t *testing.T) {
		ctx, svc := sub(t)
		parent := put(t, ctx, svc, api.Todo{Module: "billing", Title: "restructure", Priority: "P1", Writes: []string{"billing", "invoices"}})
		read := put(t, ctx, svc, api.Todo{Module: "invoices", Title: "analyse", Reads: []string{"invoices"}})
		write := put(t, ctx, svc, api.Todo{Module: "invoices", Title: "implement", Writes: []string{"invoices"}})
		first, err := svc.Pull(ctx, api.PullInput{By: alice})
		if err != nil || first.Todo == nil || first.Todo.ID != parent.ID {
			t.Fatalf("first pull: %+v, %v; want billing's restructure", first, err)
		}
		second, err := svc.Pull(ctx, api.PullInput{By: bob})
		if err != nil {
			t.Fatal(err)
		}
		if second.Todo != nil {
			t.Fatalf("second pull: %+v; want nothing while billing is written", second.Todo)
		}
		waits := strings.Join(second.Waiting, "\n")
		if !strings.Contains(waits, read.ID) || !strings.Contains(waits, write.ID) {
			t.Errorf("waiting %v; want invoices' analyse and implement", second.Waiting)
		}
	})
}
