// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	eventslocal "github.com/katyoti/katyoti-fwk/internal/extensions/events/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// EVENTS FROM OUTSIDE THE OBJECTS, on the real CWM and
// scheduler: a git repository the code source reads, a clock the test moves,
// objects that declare their own reflexes at the events pack's points.

var sourcesPack = fstest.MapFS{
	"capabilities/module.yaml":  {Data: []byte("name: module\n")},
	"capabilities/code.yaml":    {Data: []byte("name: code\n")},
	"object-types/module.yaml":  {Data: []byte("name: module\ncapabilities: [module]\n")},
	"object-types/clocked.yaml": {Data: []byte("name: clocked\ncapabilities: [timed]\n")},
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

type srcWorld struct {
	ctx   context.Context
	cwm   cwmapi.CWMService
	src   eventsapi.EventSourceService
	todos schedapi.TodoService
	repo  string
	clk   *clock
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commit(t *testing.T, repo string, files map[string]string, msg string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(repo, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(t, repo, "add", "-A")
	run(t, repo, "commit", "-q", "-m", msg)
}

func newSrcWorld(t *testing.T) *srcWorld {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "init", "-q")
	commit(t, repo, map[string]string{"shop/go.mod": "module shop\n", "docs/readme.md": "hi\n"}, "first")
	clk := &clock{t: time.Date(2026, 10, 7, 1, 0, 0, 0, time.Local)}
	state := t.TempDir()
	acts := &actions{}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(sourcesPack),
		schedlocal.Extension(),
		eventslocal.ReactionsExtensionWith(eventslocal.ReactionsOptions{BackoffSeconds: 1, SourcesDir: state}),
		eventslocal.SourcesExtensionWith(eventslocal.SourcesOptions{StateDir: state, Repo: repo, Now: clk.now, Interval: -1}),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "farm")
	src, err := application.UseService[eventsapi.EventSourceService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return &srcWorld{ctx: ctx, cwm: cwm, src: src, todos: todos, repo: repo, clk: clk}
}

// object creates an object of module m declaring its own async reflex at point.
func (w *srcWorld) object(t *testing.T, id, typ string, caps map[string]any, point cwmapi.HookPoint, uri string) {
	t.Helper()
	o := cwmapi.Object{ID: id, Type: typ}
	o.Name = id
	o.Capabilities = map[string]map[string]any{}
	for k, v := range caps {
		o.Capabilities[k] = v.(map[string]any)
	}
	o.Hooks = cwmapi.Hooks{point: {{URI: uri, Async: &cwmapi.AsyncSpec{}}}}
	module, _, _ := strings.Cut(id, ".")
	if _, err := w.cwm.ObjectCreate(w.ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
		t.Fatal(err)
	}
}

func (w *srcWorld) poll(t *testing.T) eventsapi.PollOutput {
	t.Helper()
	out, err := w.src.Poll(w.ctx, eventsapi.PollInput{})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// reactionsOf waits until n open reactions of uri exist in module m.
func (w *srcWorld) reactionsOf(t *testing.T, m, uri string, n int) []cwmapi.Match {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		list, err := w.todos.List(w.ctx, schedapi.ListInput{Module: m})
		if err != nil {
			t.Fatal(err)
		}
		var out []cwmapi.Match
		for _, td := range list.Todos {
			if td.Owner == eventsapi.Owner && td.Work[eventsapi.WorkURI] == uri {
				b, _ := json.Marshal(td.Work[eventsapi.WorkMatch])
				var mt cwmapi.Match
				_ = json.Unmarshal(b, &mt)
				out = append(out, mt)
			}
		}
		if len(out) == n {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d reaction(s) of %s in %s, want %d", len(out), uri, m, n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A commit changing a module's code packages raises code.changed on that
// module — its paths only, cause code — and its subscription reacts; a poll
// with no new commit raises nothing; a path no module holds raises nothing.
func TestACodeChangeIsRaisedOnItsModule(t *testing.T) {
	w := newSrcWorld(t)
	w.object(t, "shop", "module", map[string]any{"code": map[string]any{"code_packages": []any{"shop/"}}}, eventsapi.PointCodeChange, "x:policy/check")
	if out := w.poll(t); len(out.Raised) != 0 {
		t.Fatalf("first sight raised %+v; the cursor is set, nothing raised", out.Raised)
	}
	commit(t, w.repo, map[string]string{"shop/go.mod": "module shop\nrequire evil v1\n", "docs/readme.md": "hello\n"}, "add a package")
	out := w.poll(t)
	if len(out.Raised) != 1 || out.Raised[0].Object != "shop" || out.Raised[0].Type != eventsapi.EventCodeChanged {
		t.Fatalf("raised %+v; want one code.changed on shop", out.Raised)
	}
	m := w.reactionsOf(t, "shop", "x:policy/check", 1)[0]
	ev := m.Events[0]
	if !strings.Contains(string(ev.Payload), "shop/go.mod") || strings.Contains(string(ev.Payload), "docs/readme.md") {
		t.Fatalf("the payload %s; want shop's paths only", ev.Payload)
	}
	if ev.Cause == nil || ev.Cause.Kind != eventsapi.CauseCode {
		t.Fatalf("the event's cause %+v; want code", ev.Cause)
	}
	if out := w.poll(t); len(out.Raised) != 0 {
		t.Fatalf("a poll with no new commit raised %+v", out.Raised)
	}
	commit(t, w.repo, map[string]string{"docs/readme.md": "bye\n"}, "docs only")
	if out := w.poll(t); len(out.Raised) != 0 {
		t.Fatalf("a commit no module holds raised %+v", out.Raised)
	}
}

// A recurring timer fires once per period, never on first sight; a daily one
// fires once when its time has passed.
func TestTimersFireWhenDue(t *testing.T) {
	w := newSrcWorld(t)
	w.object(t, "farm.hourly", "clocked", map[string]any{"timed": map[string]any{"timers": []any{map[string]any{"name": "tick", "every": "1h"}}}}, eventsapi.PointTimer, "x:farm/tick")
	w.object(t, "farm.nightly", "clocked", map[string]any{"timed": map[string]any{"timers": []any{map[string]any{"name": "recheck", "at": "02:00"}}}}, eventsapi.PointTimer, "x:farm/recheck")
	t0 := w.clk.now() // 01:00
	count := func(out eventsapi.PollOutput, obj string) int {
		n := 0
		for _, r := range out.Raised {
			if r.Object == obj {
				n++
			}
		}
		return n
	}
	if out := w.poll(t); len(out.Raised) != 0 {
		t.Fatalf("first sight raised %+v", out.Raised)
	}
	w.clk.set(t0.Add(30 * time.Minute)) // 01:30
	if out := w.poll(t); len(out.Raised) != 0 {
		t.Fatalf("before any timer is due: %+v", out.Raised)
	}
	w.clk.set(t0.Add(61 * time.Minute)) // 02:01: both due
	out := w.poll(t)
	if count(out, "farm.hourly") != 1 || count(out, "farm.nightly") != 1 {
		t.Fatalf("at 02:01 raised %+v; want the hourly and the nightly once each", out.Raised)
	}
	w.clk.set(t0.Add(70 * time.Minute)) // 02:10
	if out := w.poll(t); len(out.Raised) != 0 {
		t.Fatalf("10 minutes later raised %+v; each fires once per period", out.Raised)
	}
	w.reactionsOf(t, "farm", "x:farm/recheck", 1)
}

// A delayed event is a to-do raised at its time, once.
func TestADelayedEventIsRaisedAtItsTime(t *testing.T) {
	w := newSrcWorld(t)
	w.object(t, "farm.gate", "clocked", map[string]any{}, eventsapi.PointTimer, "x:farm/open")
	got, err := w.src.After(w.ctx, eventsapi.AfterInput{Type: eventsapi.EventTimerElapsed, Object: "farm.gate", After: "5m", Payload: json.RawMessage(`{"timer":"open"}`)})
	if err != nil || got.Todo == "" {
		t.Fatalf("after: %+v %v", got, err)
	}
	if out := w.poll(t); len(out.Raised) != 0 {
		t.Fatalf("before its time raised %+v", out.Raised)
	}
	w.clk.set(w.clk.now().Add(6 * time.Minute))
	out := w.poll(t)
	if len(out.Raised) != 1 || out.Raised[0].Object != "farm.gate" {
		t.Fatalf("at its time raised %+v; want one on farm.gate", out.Raised)
	}
	if out := w.poll(t); len(out.Raised) != 0 {
		t.Fatalf("raised again %+v; a delayed event is raised once", out.Raised)
	}
	w.reactionsOf(t, "farm", "x:farm/open", 1)
}

// A call from outside raises a declared event with cause external.
func TestAnOutsideCallRaisesWithCauseExternal(t *testing.T) {
	w := newSrcWorld(t)
	w.object(t, "farm.gate", "clocked", map[string]any{}, eventsapi.PointTimer, "x:farm/open")
	if _, err := w.src.Raise(w.ctx, eventsapi.RaiseInput{Type: eventsapi.EventTimerElapsed, Object: "farm.gate", By: cwmapi.Actor{ID: "ci", Kind: "agent"}}); err != nil {
		t.Fatal(err)
	}
	m := w.reactionsOf(t, "farm", "x:farm/open", 1)[0]
	if c := m.Events[0].Cause; c == nil || c.Kind != eventsapi.CauseExternal || c.ID != "ci" {
		t.Fatalf("its cause %+v; want external ci", c)
	}
}

// THE DEBUGGER'S EVENTS READ: every declared event type with
// its subscribers as CWM resolves them — an object's own, another module's
// (CWM refuses one on a private type at declaration) — whether each applies,
// its open reactions, and the source's state.
func TestTheEventsReadListsSubscribersOpenReactionsAndTheSource(t *testing.T) {
	w := newSrcWorld(t)
	w.object(t, "shop", "module", map[string]any{"code": map[string]any{"code_packages": []any{"shop/"}}}, eventsapi.PointCodeChange, "x:policy/check")
	sec := cwmapi.NewContainerObject("security", "security", "module", "", "Security", "")
	sec.Hooks = cwmapi.Hooks{
		eventsapi.PointCodeChange: {{URI: "x:security/review", Async: &cwmapi.AsyncSpec{}, On: &cwmapi.EventSource{Module: "shop", Event: eventsapi.EventCodeChanged}, Writes: []string{"security"}}},
	}
	if _, err := w.cwm.ObjectCreate(w.ctx, cwmapi.ObjectCreateInput{Module: "security", Object: sec}); err != nil {
		t.Fatal(err)
	}
	w.poll(t)
	commit(t, w.repo, map[string]string{"shop/go.mod": "module shop\nrequire x v1\n"}, "a package")
	w.poll(t)
	w.reactionsOf(t, "shop", "x:policy/check", 1)

	react, err := application.UseService[eventsapi.ReactionService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := react.Events(w.ctx, eventsapi.ReactionEventsInput{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]eventsapi.EventView{}
	for _, e := range out.Events {
		byName[e.Event] = e
	}
	code, ok := byName[eventsapi.EventCodeChanged]
	if !ok || !code.Published || code.Point != eventsapi.PointCodeChange {
		t.Fatalf("code.changed: %+v; want it listed, published, at on_code_change", code)
	}
	var own, cross *cwmapi.EventSubscription
	for i, s := range code.Subscriptions {
		switch s.Source {
		case "object:shop":
			own = &code.Subscriptions[i]
		case "module:security":
			cross = &code.Subscriptions[i]
		}
	}
	if own == nil || cross == nil || !own.Applies || !cross.Applies {
		t.Fatalf("code.changed's subscribers %+v; want shop's own and security's, both applying", code.Subscriptions)
	}
	if code.Open[own.Subscription] != 1 {
		t.Fatalf("open reactions %v; want shop's subscription's one", code.Open)
	}
	if code.Source == nil || code.Source.Head == "" {
		t.Fatalf("code.changed's source %+v; want the HEAD last seen", code.Source)
	}
	if timer, ok := byName[eventsapi.EventTimerElapsed]; !ok || timer.Published {
		t.Fatalf("timer.elapsed: %+v; want it listed, private", timer)
	}
	if _, ok := byName["object.created"]; !ok {
		t.Fatal("CWM's own event types are not listed")
	}
	// A type nobody subscribes to answers an empty list, never null (the
	// debugger's tab iterates it).
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), `"subscriptions":null`) {
		t.Fatal("an event type with no subscriber answers subscriptions null")
	}
}
