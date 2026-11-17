// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
)

// A pull's waiting line and a queue entry carry their structured wait - the
// cause (lock, reserved, backoff, assigned, members, person) and what holds
// it - in the scheduler's own words; an owner's wait namer prefixes the
// waiting line of its to-dos with its label, outside the store's hold, and a
// to-do of another owner keeps the scheduler's words.
func TestAPullsWaitingLineCarriesItsCauseAndHolder(t *testing.T) {
	pr := &holdProbe{}
	ctx, _, svc, _ := world(t, reviewers(), pr.namer())

	// In its back-off: failed once by carol, ready again in a minute.
	retried := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-B", Title: "retried", MaxAttempts: 2, BackoffSeconds: 60})
	if p, err := svc.Pull(ctx, api.PullInput{ID: retried.ID, By: carol}); err != nil || p.Todo == nil {
		t.Fatalf("carol pulls %s: %+v %v", retried.ID, p, err)
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: retried.ID, Result: api.Result{Status: api.StatusFail, Summary: "failed", By: carol}}); err != nil {
		t.Fatal(err)
	}
	// Held by alice: ledger is in use.
	holder := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-1", Title: "implement", Writes: []string{"ledger"}})
	if p, err := svc.Pull(ctx, api.PullInput{ID: holder.ID, By: alice}); err != nil || p.Todo == nil {
		t.Fatalf("alice pulls %s: %+v %v", holder.ID, p, err)
	}
	locked := put(t, ctx, svc, api.Todo{Owner: "named", Module: "billing", Request: "billing.RQ-2", Title: "behind ledger", Writes: []string{"ledger"}})
	// A holding list's step ahead in the queue reserves stock.
	reserver := put(t, ctx, svc, api.Todo{Module: "shipping", Request: "shipping.RQ-2", Title: "review", Priority: "P1", Group: "hl-reviewers", Writes: []string{"stock"}})
	// A person's step, asked for by an agent.
	personal := put(t, ctx, svc, api.Todo{Module: "shipping", Request: "shipping.RQ-6", Title: "approve", For: api.ForPerson})
	reserved := put(t, ctx, svc, api.Todo{Module: "shipping", Request: "shipping.RQ-3", Title: "behind stock", Writes: []string{"stock"}})
	// Given to alice, and held for hl-reviewers' members.
	given := put(t, ctx, svc, api.Todo{Owner: "named", Module: "billing", Request: "billing.RQ-4", Title: "given", Group: "hl-reviewers"})
	if _, err := svc.Assign(ctx, api.AssignInput{ID: given.ID, To: alice, By: person}); err != nil {
		t.Fatal(err)
	}
	listed := put(t, ctx, svc, api.Todo{Module: "shipping", Request: "shipping.RQ-5", Title: "listed", Group: "hl-reviewers"})

	pr.arm(ctx, svc, "shipping")
	out, err := svc.Pull(ctx, api.PullInput{By: bob})
	if err != nil {
		t.Fatal(err)
	}
	if out.Todo != nil {
		t.Fatalf("bob took %s (%s); every ready to-do should wait for him", out.Todo.ID, out.Todo.Title)
	}
	if len(out.Waits) != len(out.Waiting) {
		t.Fatalf("%d structured waits for %d waiting lines: %+v", len(out.Waits), len(out.Waiting), out)
	}
	type want struct {
		cause, module, holderTodo, holderRequest, holder, group string
		until, named                                            bool
	}
	wants := map[string]want{
		retried.ID:  {cause: api.WaitBackoff, until: true},
		locked.ID:   {cause: api.WaitLock, module: "ledger", holderTodo: holder.ID, holderRequest: "billing.RQ-1", holder: alice.ID, named: true},
		reserver.ID: {cause: api.WaitMembers, group: "hl-reviewers"},
		reserved.ID: {cause: api.WaitReserved, module: "stock", holderTodo: reserver.ID, holderRequest: "shipping.RQ-2"},
		given.ID:    {cause: api.WaitAssigned, holder: alice.ID, named: true},
		listed.ID:   {cause: api.WaitMembers, group: "hl-reviewers"},
	}
	seen := map[string]bool{}
	for i, w := range out.Waits {
		line := out.Waiting[i]
		wt, ok := wants[w.Todo]
		if !ok {
			t.Errorf("an unexpected waiting to-do %s: %q", w.Todo, line)
			continue
		}
		seen[w.Todo] = true
		holderID := ""
		if w.Holder != nil {
			holderID = w.Holder.ID
		}
		if w.Cause != wt.cause || w.Module != wt.module || w.HolderTodo != wt.holderTodo || w.HolderRequest != wt.holderRequest ||
			holderID != wt.holder || w.Group != wt.group || (w.Until != "") != wt.until {
			t.Errorf("%s waits %+v (holder %q); want %+v", w.Todo, w, holderID, wt)
		}
		if !strings.HasPrefix(line, w.Todo+": ") {
			t.Errorf("waiting line %q does not start with its to-do %s", line, w.Todo)
		}
		label := "N:" + w.Cause + " - "
		if got := strings.Contains(line, label); got != wt.named {
			t.Errorf("waiting line %q: named %v; want %v (only the namer's owner is labelled)", line, got, wt.named)
		}
	}
	for id := range wants {
		if !seen[id] {
			t.Errorf("%s has no waiting line: %+v", id, out.Waiting)
		}
	}
	if !strings.Contains(strings.Join(out.Waiting, "\n"), "module ledger is in use (held by "+holder.ID+")") {
		t.Errorf("the scheduler's words are gone from the lines: %v", out.Waiting)
	}
	pr.check(t)

	// A person's step asked for by an agent waits for a person.
	p, err := svc.Pull(ctx, api.PullInput{ID: personal.ID, By: bob})
	if err != nil {
		t.Fatal(err)
	}
	if p.Todo != nil || len(p.Waits) != 1 || p.Waits[0].Cause != api.WaitPerson || p.Waits[0].Todo != personal.ID || !strings.HasPrefix(p.Waiting[0], personal.ID+": a person's to-do") {
		t.Errorf("bob asks for %s: %+v; want it waiting, cause person, in the scheduler's words", personal.ID, p)
	}

	// The queue's entries carry the same structured wait.
	q, err := svc.Queue(ctx, api.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range q.Queue {
		wt, ok := wants[e.Todo.ID]
		if !ok || e.Takeable {
			if e.Wait != nil {
				t.Errorf("takeable %s carries a wait %+v", e.Todo.ID, e.Wait)
			}
			continue
		}
		if e.Wait == nil || e.Wait.Cause != wt.cause || e.Wait.Todo != e.Todo.ID || e.Wait.HolderTodo != wt.holderTodo {
			t.Errorf("queue entry %s waits %q with %+v; want cause %s held by %q", e.Todo.ID, e.Waits, e.Wait, wt.cause, wt.holderTodo)
		}
	}
}

// On the SQLite store, whose hold is one write transaction, the namer runs
// once the pull's hold is let go: a write from outside the pull lands while it
// names.
func TestAWaitNamerRunsOutsideTheStoresHold(t *testing.T) {
	pr := &holdProbe{}
	dir := t.TempDir()
	ctx, _, svc, _ := selectionWorldOf(t, dir, true, []string{"billing"}, nil, pr.namer())
	holder := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-1", Title: "implement", Writes: []string{"ledger"}})
	if p, err := svc.Pull(ctx, api.PullInput{ID: holder.ID, By: alice}); err != nil || p.Todo == nil {
		t.Fatalf("alice pulls %s: %+v %v", holder.ID, p, err)
	}
	locked := put(t, ctx, svc, api.Todo{Owner: "named", Module: "billing", Request: "billing.RQ-2", Title: "behind ledger", Writes: []string{"ledger"}})
	pr.arm(ctx, svc, "billing")
	out, err := svc.Pull(ctx, api.PullInput{By: bob})
	if err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, "scheduler.db")) {
		t.Fatal("no scheduler.db: the pull did not run on the SQLite store")
	}
	if out.Todo != nil || len(out.Waiting) != 1 || !strings.HasPrefix(out.Waiting[0], locked.ID+": N:lock - ") {
		t.Fatalf("bob's pull: %+v; want %s waiting, named", out, locked.ID)
	}
	pr.check(t)
}

// holdProbe is a wait namer for the owner "named" that labels a wait
// "N:<cause>" and, the first time it names once armed, writes a to-do from
// outside the pull - a write that would wait on the store's hold were the
// namer run inside it.
type holdProbe struct {
	mu     sync.Mutex
	outer  context.Context
	svc    api.TodoService
	module string
	probed bool
	err    error
}

func (p *holdProbe) arm(ctx context.Context, svc api.TodoService, module string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outer, p.svc, p.module = ctx, svc, module
}

func (p *holdProbe) check(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.probed || p.err != nil {
		t.Errorf("the namer's outside write: probed %v, %v; want it written - the namer runs outside the store's hold", p.probed, p.err)
	}
}

func (p *holdProbe) namer() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("scheduler-test", ".wait-namer", func(app *application.Application) error {
			return schedlocal.ContributeWaitNamer(app, api.WaitNamer{Owner: "named", Name: func(_ context.Context, _ api.Todo, w api.QueueWait) string {
				p.mu.Lock()
				defer p.mu.Unlock()
				if !p.probed && p.outer != nil {
					p.probed = true
					done := make(chan error, 1)
					go func() {
						_, err := p.svc.Put(p.outer, api.PutInput{Todo: api.Todo{Owner: "test", For: api.ForAgent, Start: api.StartAuto, Priority: "P2",
							Module: p.module, Target: p.module, Title: "written while naming", State: api.StateWaiting}})
						done <- err
					}()
					select {
					case p.err = <-done:
					case <-time.After(5 * time.Second):
						p.err = context.DeadlineExceeded
					}
				}
				return "N:" + w.Cause
			}})
		})
	}
}
