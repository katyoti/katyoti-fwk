// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// AN on_* HOOK IS NEVER SILENTLY LOST. These tests measure what becomes of a
// hook that runs after the write when the process stops before it ran, and when
// its action fails.

// durableActions answers hook calls: a uri listed in failFirst fails that many
// times then succeeds (-1 = always fails); block holds every call until closed.
type durableActions struct {
	mu        sync.Mutex
	failFirst map[string]int
	block     chan struct{}
	calls     map[string]int
	causes    map[string]cause.Cause // uri → the cause the last call ran under
	ran       chan string
}

func newDurableActions() *durableActions {
	return &durableActions{failFirst: map[string]int{}, calls: map[string]int{}, causes: map[string]cause.Cause{}, ran: make(chan string, 64)}
}

func (f *durableActions) Describe(stdctx.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (f *durableActions) Run(ctx stdctx.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	f.calls[in.URI]++
	f.causes[in.URI], _ = cause.CauseFrom(ctx)
	n, left := f.calls[in.URI], f.failFirst[in.URI]
	f.mu.Unlock()
	f.ran <- in.URI
	if left < 0 || n <= left {
		return actionsapi.RunOutput{}, errors.New("the hook's action failed")
	}
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{}`)}, nil
}

func (f *durableActions) callsTo(uri string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[uri]
}

func (f *durableActions) causeOf(uri string) cause.Cause {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.causes[uri]
}

// durableConfig is one project whose `hooked` objects fire x:after/create; a
// second service opened on the same roots is the process started again.
func durableConfig(t *testing.T) *CWMServiceConfig {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "hooked", Capabilities: []string{"container"}, Hooks: api.Hooks{
			api.HookOnCreate: {{URI: "x:after/create"}},
		}},
	}
	return cfg
}

func openDurable(t *testing.T, cfg *CWMServiceConfig, actions actionsapi.ActionService) (*Service, stdctx.Context) {
	t.Helper()
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc.hookRetry = hookRetryPolicy{Attempts: 3, Backoff: time.Millisecond, MaxBackoff: 4 * time.Millisecond}
	app := application.New(application.Config())
	if actions != nil {
		application.PutService[actionsapi.ActionService](app, "default", actions)
	}
	return svc, application.WithApplication(stdctx.Background(), app)
}

func hookedObject(id string) api.Object {
	o := api.Object{ID: id, Type: "hooked"}
	o.Name = id
	return o
}

// waitFor polls cond for up to 3s.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: not within 3s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func hookFailures(t *testing.T, svc *Service, ctx stdctx.Context, id string) []api.ObjectDiagnostic {
	t.Helper()
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: id})
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	var out []api.ObjectDiagnostic
	for _, d := range got.Errors {
		if d.Code == api.DiagnosticHookFailed {
			out = append(out, d)
		}
	}
	return out
}

// TestAnOnHookTheProcessDidNotRunRunsAtTheNextStart: the write lands, the
// process stops before the hook's action ran; the next start runs it.
func TestAnOnHookTheProcessDidNotRunRunsAtTheNextStart(t *testing.T) {
	cfg := durableConfig(t)
	stuck := newDurableActions()
	stuck.block = make(chan struct{}) // the action never gets to run in this process
	defer close(stuck.block)
	svc1, ctx1 := openDurable(t, cfg, stuck)
	if _, err := svc1.ObjectCreate(ctx1, api.ObjectCreateInput{Module: "billing", Object: hookedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = svc1.Close() // the process stops

	after := newDurableActions()
	svc2, ctx2 := openDurable(t, cfg, after)
	defer svc2.Close()
	svc2.RunBootstrapHooks(ctx2, BootstrapAll) // the start
	waitFor(t, "on_create replayed at the next start", func() bool { return after.callsTo("x:after/create") == 1 })
	waitFor(t, "the journal emptied once it ran", func() bool { return len(svc2.hooks().pending()) == 0 })
}

// TestAnOnHookWithNoActionServiceRunsAtTheNextStart: a write in a process that
// cannot run actions keeps its hook for a process that can.
func TestAnOnHookWithNoActionServiceRunsAtTheNextStart(t *testing.T) {
	cfg := durableConfig(t)
	svc1, ctx1 := openDurable(t, cfg, nil)
	if _, err := svc1.ObjectCreate(ctx1, api.ObjectCreateInput{Module: "billing", Object: hookedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = svc1.Close()

	after := newDurableActions()
	svc2, ctx2 := openDurable(t, cfg, after)
	defer svc2.Close()
	svc2.RunBootstrapHooks(ctx2, BootstrapAll)
	waitFor(t, "on_create replayed", func() bool { return after.callsTo("x:after/create") == 1 })
}

// TestAFailingOnHookIsRetriedThenShownOnTheObject: an action that keeps failing
// is retried up to the policy, then the object carries the failure (its pane,
// the tree's red, the Errors list) and the journal keeps it.
func TestAFailingOnHookIsRetriedThenShownOnTheObject(t *testing.T) {
	cfg := durableConfig(t)
	f := newDurableActions()
	f.failFirst["x:after/create"] = -1
	svc, ctx := openDurable(t, cfg, f)
	defer svc.Close()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: hookedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	waitFor(t, "the failure on the object", func() bool { return len(hookFailures(t, svc, ctx, "billing.a")) == 1 })
	if n := f.callsTo("x:after/create"); n != 3 {
		t.Errorf("attempts = %d, want 3 (the policy)", n)
	}
	list, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "billing"})
	if err != nil || len(list.InError) != 1 || list.InError[0] != "billing.a" {
		t.Errorf("in error = %v (%v), want billing.a", list.InError, err)
	}
	errs, _ := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	if errs.Total != 1 {
		t.Errorf("error list total = %d, want 1", errs.Total)
	}

	// The failure survives a restart, and is not retried on its own there.
	_ = svc.Close()
	again := newDurableActions()
	svc2, ctx2 := openDurable(t, cfg, again)
	defer svc2.Close()
	svc2.RunBootstrapHooks(ctx2, BootstrapAll)
	if len(hookFailures(t, svc2, ctx2, "billing.a")) != 1 {
		t.Fatal("the failure was lost by a restart")
	}
	time.Sleep(50 * time.Millisecond)
	if n := again.callsTo("x:after/create"); n != 0 {
		t.Errorf("a failed hook re-ran on its own at start (%d calls)", n)
	}
}

// TestAFailedHookClearsWhenItNextSucceeds: the same hook on the same object
// succeeding later removes the failure.
func TestAFailedHookClearsWhenItNextSucceeds(t *testing.T) {
	cfg := durableConfig(t)
	cfg.CatalogTypes.Objects[0].Hooks[api.HookOnUpdate] = []api.HookEntry{{URI: "x:after/create"}}
	f := newDurableActions()
	f.failFirst["x:after/create"] = 3 // the create's three attempts fail
	svc, ctx := openDurable(t, cfg, f)
	defer svc.Close()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: hookedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	waitFor(t, "the failure on the object", func() bool { return len(hookFailures(t, svc, ctx, "billing.a")) == 1 })
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: hookedObject("billing.a")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	waitFor(t, "the failure cleared", func() bool { return len(hookFailures(t, svc, ctx, "billing.a")) == 0 })
}

// TestAFlakyOnHookLeavesNoTrace: an action that fails then succeeds within the
// policy leaves nothing on the object and nothing in the journal.
func TestAFlakyOnHookLeavesNoTrace(t *testing.T) {
	cfg := durableConfig(t)
	f := newDurableActions()
	f.failFirst["x:after/create"] = 2
	svc, ctx := openDurable(t, cfg, f)
	defer svc.Close()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: hookedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	waitFor(t, "the third attempt", func() bool { return f.callsTo("x:after/create") == 3 })
	waitFor(t, "the journal emptied", func() bool { return len(svc.hooks().pending()) == 0 })
	if d := hookFailures(t, svc, ctx, "billing.a"); len(d) != 0 {
		t.Fatalf("a hook that succeeded on retry left %+v", d)
	}
}

// TestAnOnHookRunsUnderItsOwnCauseAtTheWriteAndAtAReplay: the action of an
// on_* hook runs under {hook, the journal entry's id, the write's depth + 1}
// — when the write dispatches it, and when the next start replays it from
// the journal, where the cause was kept.
func TestAnOnHookRunsUnderItsOwnCauseAtTheWriteAndAtAReplay(t *testing.T) {
	cfg := durableConfig(t)
	// At the write: the action sees the hook's cause, one deeper than the write's.
	direct := newDurableActions()
	svc0, ctx0 := openDurable(t, cfg, direct)
	ctx0 = cause.WithCause(ctx0, cause.Cause{Kind: cause.KindAgent, ID: "run-7", Depth: 2})
	if _, err := svc0.ObjectCreate(ctx0, api.ObjectCreateInput{Module: "billing", Object: hookedObject("billing.direct")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	waitFor(t, "on_create ran", func() bool { return direct.callsTo("x:after/create") == 1 })
	if c := direct.causeOf("x:after/create"); c.Kind != cause.KindHook || c.ID == "" || c.Depth != 3 {
		t.Fatalf("on_create ran under %+v, want {hook, <entry id>, 3}", c)
	}
	waitFor(t, "the journal emptied", func() bool { return len(svc0.hooks().pending()) == 0 })
	_ = svc0.Close()

	// At a replay: the entry kept its cause; the next start runs under it.
	stuck := newDurableActions()
	stuck.block = make(chan struct{}) // the action never gets to run in this process
	defer close(stuck.block)
	svc1, ctx1 := openDurable(t, cfg, stuck)
	ctx1 = cause.WithCause(ctx1, cause.Cause{Kind: cause.KindPerson, ID: "ada", Depth: 0})
	if _, err := svc1.ObjectCreate(ctx1, api.ObjectCreateInput{Module: "billing", Object: hookedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	pending := svc1.hooks().pending()
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want the one call owed", len(pending))
	}
	want := cause.Cause{Kind: cause.KindHook, ID: pending[0].ID, Depth: 1}
	if pending[0].Cause != want {
		t.Fatalf("the journaled entry carries %+v, want %+v", pending[0].Cause, want)
	}
	_ = svc1.Close() // the process stops

	after := newDurableActions()
	svc2, ctx2 := openDurable(t, cfg, after)
	defer svc2.Close()
	svc2.RunBootstrapHooks(ctx2, BootstrapAll) // the start: a bare context, no cause of its own
	waitFor(t, "on_create replayed", func() bool { return after.callsTo("x:after/create") == 1 })
	if got := after.causeOf("x:after/create"); got != want {
		t.Fatalf("the replayed call ran under %+v, want %+v", got, want)
	}
}
