// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/local"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	gatescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/gates/contrib"
	gateslocal "github.com/katyoti/katyoti-fwk/internal/extensions/gates/local"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	inboxlocal "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/local"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	personasapi "github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/contrib"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	verificationcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/verification/contrib"
	verificationlocal "github.com/katyoti/katyoti-fwk/internal/extensions/verification/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// REQUESTS, end to end on a real CWM: the planification pack as shipped (type,
// field-group, workflow, depends-on edge, hooks), the service, and the hooks'
// URIs routed to the service in process — what the bus does in ktw-agents.

const (
	servicePrefix  = "ktw-agents:requests/"
	releasesPrefix = "ktw-agents:releases/"
	planningPrefix = "ktw-agents:planning/"
	triggersPrefix = "ktw-agents:triggers/"
)

// routeActions runs the pack's hook URIs on the request service.
// attached records the criteria attached to a feature (features/attach-criterion).
var attached []string

type routeActions struct{}

// The functions verify() and lint() a gate check calls: the verification
// service's read-only methods.
const (
	verifyFunctionURI = "ktw-test:verification/verify-function"
	lintFunctionURI   = "ktw-test:verification/lint-function"
	gateFunctionURI   = "ktw-test:gates/gate-function"
)

func (routeActions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: in.URI == verifyFunctionURI || in.URI == lintFunctionURI || in.URI == gateFunctionURI}, nil
}

func (routeActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if in.URI == "ktw-methods:features/attach-criterion" {
		attached = append(attached, string(in.Input))
		return actionsapi.RunOutput{URI: in.URI, Value: []byte("{}")}, nil
	}
	if in.URI == gateFunctionURI {
		var call cwmapi.FunctionCall
		if err := json.Unmarshal(in.Input, &call); err != nil {
			return actionsapi.RunOutput{}, err
		}
		g, err := application.UseService[gatesapi.GatesService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err := g.GateFunction(ctx, call)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		raw, _ := json.Marshal(out)
		return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
	}
	if in.URI == verifyFunctionURI || in.URI == lintFunctionURI {
		var call cwmapi.FunctionCall
		if err := json.Unmarshal(in.Input, &call); err != nil {
			return actionsapi.RunOutput{}, err
		}
		v, err := application.UseService[verificationapi.VerificationService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		var out any
		if in.URI == verifyFunctionURI {
			out, err = v.VerifyFunction(ctx, call)
		} else {
			out, err = v.LintFunction(ctx, call)
		}
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		raw, _ := json.Marshal(out)
		return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
	}
	if strings.HasPrefix(in.URI, decisionsPrefix) {
		return runDecisions(ctx, in)
	}
	svc, err := application.UseService[api.RequestService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	var out any
	switch in.URI {
	case servicePrefix + "ensure":
		out, err = svc.Ensure(ctx, api.EnsureInput{})
	case servicePrefix + "plan-modules":
		out, err = svc.PlanModules(ctx, api.EnsureInput{})
	case servicePrefix + "plan-module":
		var call cwmapi.HookCall
		if err := json.Unmarshal(in.Input, &call); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err = svc.PlanModule(ctx, call)
	case servicePrefix + "check-module-updated":
		// The module-updated gate's requirement on a plan's finish.
		var mu api.ModuleUpdatedInput
		if err := json.Unmarshal(in.Input, &mu); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err = svc.CheckModuleUpdated(ctx, mu)
	case "ktw-agents:increments/check-usable-delivered":
		// The usable-delivered gate's requirement on an increment's done.
		inc, ierr := application.UseService[api.IncrementService](ctx)
		if ierr != nil {
			return actionsapi.RunOutput{}, ierr
		}
		var ud api.UsableDeliveredInput
		if err := json.Unmarshal(in.Input, &ud); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err = inc.CheckUsableDelivered(ctx, ud)
	case releasesPrefix + "check-transition", releasesPrefix + "lint":
		rel, rerr := application.UseService[api.ReleaseService](ctx)
		if rerr != nil {
			return actionsapi.RunOutput{}, rerr
		}
		var hin api.HookInput
		if err := json.Unmarshal(in.Input, &hin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if in.URI == releasesPrefix+"lint" {
			out, err = rel.Lint(ctx, hin)
		} else {
			out, err = rel.CheckTransition(ctx, hin)
		}
	case triggersPrefix + "fire":
		trig, terr := application.UseService[api.TriggerService](ctx)
		if terr != nil {
			return actionsapi.RunOutput{}, terr
		}
		var call cwmapi.HookCall
		if err := json.Unmarshal(in.Input, &call); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err = trig.Fire(ctx, call)
	case planningPrefix + "check-transition", planningPrefix + "gui-tested":
		tr, terr := application.UseService[api.PlanningService](ctx)
		if terr != nil {
			return actionsapi.RunOutput{}, terr
		}
		var hin api.HookInput
		if err := json.Unmarshal(in.Input, &hin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if in.URI == planningPrefix+"gui-tested" {
			out, err = tr.GuiTested(ctx, hin)
		} else {
			out, err = tr.CheckTransition(ctx, hin)
		}
	default:
		if strings.HasPrefix(in.URI, fakeConverterPrefix) {
			return fakeConvert(in)
		}
		return actionsapi.RunOutput{}, fmt.Errorf("no route for %s", in.URI)
	}
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(out)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// host is what ktw-agents and ktw-methods declare that the planification pack
// builds on: modules, the system module, and the capabilities it composes.
var host = fstest.MapFS{
	"capabilities/container.yaml":     {Data: []byte("name: container\n")},
	"capabilities/content.yaml":       {Data: []byte("name: content\n")},
	"capabilities/lifecycle.yaml":     {Data: []byte("name: lifecycle\n")},
	"capabilities/system-module.yaml": {Data: []byte("name: system-module\n")},
	"capabilities/module.yaml":        {Data: []byte("name: module\n")},
	"object-types/module.yaml":        {Data: []byte("name: module\ncapabilities: [container, module]\n")},
	"object-types/system-module.yaml": {Data: []byte("name: system-module\ncapabilities: [container, module, system-module]\n")},
}

// withGoals is the host plus a use-case type on the shipped `manual` workflow —
// work a release can target, whose `done` state is the one a release waits on.
func withGoals(t *testing.T) fstest.MapFS {
	fs := fstest.MapFS{
		"capabilities/want.yaml":      {Data: []byte("name: want\n")},
		"capabilities/checkable.yaml": {Data: []byte("name: checkable\n")},
		"object-types/use-case.yaml":  {Data: []byte("name: use-case\nfolder: use-cases\nid_prefix: UC\ncapabilities: [container, want, lifecycle]\nworkflow: manual\n")},
		"workflows/manual.yaml":       {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/manual.yaml")},
		"object-types/idea.yaml":      {Data: []byte("name: idea\nfolder: ideas\ncapabilities: [container, want]\n")},
		// The ladders ktw-methods ships. Every world carries the same pack: a
		// second test application in the process does not see declarations
		// the first one lacked (filed for CWM).
		"object-types/goal.yaml":    {Data: []byte("name: goal\nfolder: goals\nid_prefix: G\ncapabilities: [container, want, lifecycle]\nworkflow: goal\n")},
		"object-types/feature.yaml": {Data: []byte("name: feature\nfolder: features\nid_prefix: F\ncapabilities: [container, want, lifecycle]\nworkflow: feature\n")},
		"workflows/goal.yaml":       {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/goal.yaml")},
		"workflows/feature.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/feature.yaml")},
		// The requirements ktw-methods gives its types on planification's gates.
		"gate-requirements/goal-decomposed.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/gate-requirements/goal-decomposed.yaml")},
		"gate-requirements/feature-specified.yaml":  {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/gate-requirements/feature-specified.yaml")},
		"gate-requirements/feature-challenged.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/gate-requirements/feature-challenged.yaml")},
		"edge-types/serves.yaml":                    {Data: []byte("name: serves\ncardinality: M:N\nto_capabilities: [want]\ntarget: object\nobject_storage: edges\ninverse_name: served-by\n")},
		"edge-types/wants.yaml":                     {Data: []byte("name: wants\ncardinality: M:N\nfrom_capabilities: [want]\nto_capabilities: [want]\ntarget: object\nobject_storage: edges\n")},
		// A decision, decided on its own panel: a decide request's person
		// step ends on its move to applies.
		"capabilities/decision.yaml": {Data: []byte("name: decision\n")},
		"object-types/decision.yaml": {Data: []byte("name: decision\nfolder: decisions\nid_prefix: OD\ncapabilities: [container, decision, lifecycle]\nworkflow: decision\n")},
		"workflows/decision.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/decision.yaml")},
	}
	for k, v := range host {
		fs[k] = v
	}
	return fs
}

var (
	person = cwmapi.Actor{ID: "jdoe", Kind: "person"}
	intake = cwmapi.Actor{ID: "intake", Kind: "agent"}
	// builder is an agent doing a request's work — who may say it is done.
	builder = cwmapi.Actor{ID: "builder", Kind: "agent"}
)

func world(t *testing.T) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	return worldWith(t, nil)
}

// worldWith is world with more declarations in the host pack.
func worldWith(t *testing.T, extra fstest.MapFS) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	return worldConfigured(t, extra, nil)
}

// worldConfigured is worldWith with configuration files (name → content).
func worldConfigured(t *testing.T, extra fstest.MapFS, files map[string]string) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	return worldExtended(t, extra, files)
}

// worldExtended is worldConfigured with more extensions in the binary (a
// fake contributed source, ...).
func worldExtended(t *testing.T, extra fstest.MapFS, files map[string]string, exts ...application.Extension) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	ctx, cwm := cwmapp.NewWithConfig(t, files, worldExtensions(t, extra, exts...)...)
	cwmapp.Module(t, ctx, cwm, "billing")
	return settledWorld(t, ctx, cwm)
}

// worldExtensions is what a world's binary holds: the CWM pack with extra,
// the planification, scheduler, inbox, journal and gates services, the fake
// runs - and exts.
func worldExtensions(t *testing.T, extra fstest.MapFS, exts ...application.Extension) []application.Extension {
	t.Helper()
	pack := withGoals(t)
	for k, v := range extra {
		pack[k] = v
	}
	return []application.Extension{
		verificationlocal.Extension(),
		verificationcontrib.Pack(""),
		gateslocal.Extension(),
		gatescontrib.Pack(""),
		panelocal.Extension(),
		// The in-process bus CWM publishes on (a host binary registers it).
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		cwmapp.Pack(pack),
		contrib.Pack("ktw-agents"),
		schedlocal.Extension(),
		planlocal.Extension(),
		inboxcontrib.Pack("ktw-agents"),
		// The kinds of questions a person step is asked as.
		inboxcontrib.DeclareKinds("ktw-agents"),
		inboxcontrib.OwnAskKinds("ktw-agents"),
		inboxlocal.Extension(),
		// The human task's answers: the collector that
		// keeps its answer - a gate's confirmation is one.
		inboxlocal.AnswersExtension(),
		// « À faire »'s waiting list, which the person queue reads
		// with IN-039's switch on.
		inboxlocal.WaitingExtension(),
		// The journal: the lifecycle moves the views read (forced moves, done
		// dates).
		journalcontrib.Pack("ktw-agents"),
		journallocal.Extension(),
		// ktw-methods' decisions, which a choice a step reports opens.
		decisionslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return routeActions{}, nil
		}),
		// The runs the scheduler starts: a fake whose outcomes a test sets, and a
		// persona library whose spawns are runs of that fake.
		application.DeclareSimpleService[taskapi.TaskService](func(context.Context) (taskapi.TaskService, error) {
			return newFakeTasks(), nil
		}),
		application.DeclareSimpleService[personasapi.PersonaService](func(ctx context.Context) (personasapi.PersonaService, error) {
			tasks, err := application.UseService[taskapi.TaskService](ctx)
			if err != nil {
				return nil, err
			}
			return newFakePersonas(tasks.(*fakeTasks), "coder", "reviewer", "documenter"), nil
		}),
		application.DeclareExtensions(exts...),
	}
}

// settledWorld is a world's application, closed and settled at the test's
// end.
func settledWorld(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	// The journal records on its own goroutine: the close drains it before
	// the workspace is removed (run after the settle below).
	t.Cleanup(func() { appcore.CloseApplicationScope(ctx) })
	// The planning loop runs on the observer's goroutine: let it settle before
	// the workspace is torn down (registered after the app, so it runs first).
	t.Cleanup(func() { planlocal.Settle(ctx, 5*time.Second) })
	svc, err := application.UseService[api.RequestService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc
}

func file(t *testing.T, ctx context.Context, svc api.RequestService, in api.FileInput) api.Request {
	t.Helper()
	if in.By.ID == "" {
		in.By = person
	}
	if in.Module == "" {
		in.Module = "billing"
	}
	out, err := svc.File(ctx, in)
	if err != nil {
		t.Fatalf("File %q: %v", in.Title, err)
	}
	return out.Request
}

// walk moves a request's plan, as its steps' tasks would: ready plans its
// intake again, done and dropped finish it.
func walk(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string, states ...string) {
	t.Helper()
	plans := plansOf(t, ctx)
	for _, to := range states {
		var err error
		switch to {
		case api.PlanReady:
			if got, gerr := plans.Get(ctx, api.GetPlanInput{ID: id}); gerr == nil && got.Plan.State == api.PlanReady {
				continue
			}
			_, err = plans.Plan(ctx, api.PlanNextInput{ID: id, By: intake})
		case api.PlanDone, api.PlanDropped:
			_, err = plans.Finish(ctx, api.FinishInput{ID: id, Outcome: to, By: builder})
		default:
			t.Fatalf("walk %s: no move to %s", id, to)
		}
		if err != nil {
			t.Fatalf("%s → %s: %v", id, to, err)
		}
	}
}

func state(t *testing.T, ctx context.Context, svc api.RequestService, id string) string {
	t.Helper()
	got, err := svc.Get(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return got.Request.State
}

// waitState polls a request until it reaches a state (the wake is an observer,
// so it lands after the transition that caused it). A read that meets the
// observer's write can fail on Windows (a CWM store limit, filed there): retried.
func waitState(t *testing.T, ctx context.Context, svc api.RequestService, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := svc.Get(ctx, api.GetInput{ID: id})
		if err == nil && got.Request.State == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %+v, %v after 5s; want %s", id, got.Request.State, err, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A request lands in the planning space of the module it targets, in state
// new, with the fields it was filed with.
func TestFileLandsInTheModulePlanningSpace(t *testing.T) {
	t.Parallel()
	ctx, _, svc := world(t)
	out, err := svc.File(ctx, api.FileInput{
		Title: "Check the cross links in the docs of billing", Module: "billing", By: person,
		Deliverables: []api.Deliverable{{Title: "a list of broken links", Check: "make lint-links"}},
		BudgetUSD:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := out.Request
	if dir := filepath.ToSlash(filepath.Dir(out.Path)); !strings.HasSuffix(dir, "modules/billing/.ktw/requests") {
		t.Errorf("filed at %s, want under modules/billing/.ktw/requests", out.Path)
	}
	if r.Module != "billing" || r.State != api.PlanReady || !strings.Contains(r.ID, "RQ-") {
		t.Errorf("request = %+v, want an RQ- id in billing, state new", r)
	}
	if r.Requester != person || len(r.Deliverables) != 1 || r.BudgetUSD != 2 {
		t.Errorf("fields = %+v", r.RequestFields)
	}
}

// A request naming no module waits in sys-planification, created for it, and
// its intake is planned and taken there.
func TestAnUnroutedRequestWaitsInSysPlanification(t *testing.T) {
	t.Parallel()
	ctx, _, svc := world(t)
	out, err := svc.File(ctx, api.FileInput{Title: "Now build the final version of cwm", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if out.Request.Module != api.SystemModule {
		t.Fatalf("module = %s, want %s", out.Request.Module, api.SystemModule)
	}
	again, err := svc.Ensure(ctx, api.EnsureInput{})
	if err != nil || again.Created || again.Module != api.SystemModule {
		t.Fatalf("Ensure after File = %+v, %v; want the existing module", again, err)
	}
	// Its intake is planned there, the one system module that holds to-dos,
	// and taken: it routes the request on.
	in := stepOf(t, ctx, out.Request.ID, api.StepIntake)
	if in.Module != api.SystemModule || in.State != schedapi.StateReady {
		t.Fatalf("its intake: module %s, %s; want ready in %s", in.Module, in.State, api.SystemModule)
	}
	pullAndReport(t, ctx, in.ID, session, schedapi.Result{Summary: "routed"})
}

// A follow-up a step of the system module reports is filed there: the system
// module is a module requester by its capability, though its type is not
// named module.
func TestASystemModuleStepFilesItsFollowUp(t *testing.T) {
	t.Parallel()
	ctx, _, svc := world(t)
	out, err := svc.File(ctx, api.FileInput{Title: "Plan the next release", By: person})
	if err != nil {
		t.Fatal(err)
	}
	in := stepOf(t, ctx, out.Request.ID, api.StepIntake)
	got := pullAndReport(t, ctx, in.ID, session, schedapi.Result{Summary: "routed",
		More: map[string]any{"follow_ups": []map[string]any{{"own": true, "title": "Fix the release calendar", "kind": "fix"}}}})
	if !strings.Contains(got.Note, "collected by planification") {
		t.Fatalf("report note %q, want collected", got.Note)
	}
	fu, _ := svc.List(ctx, api.ListInput{Module: api.SystemModule, Requester: api.SystemModule})
	if len(fu.Requests) != 1 || fu.Requests[0].Title != "Fix the release calendar" {
		t.Errorf("the system module's follow-ups: %+v", fu.Requests)
	}
}

// Done is refused while a deliverable is unverified: Finish checks them.
func TestDoneWaitsOnEveryDeliverable(t *testing.T) {
	t.Parallel()
	ctx, cwm, svc := world(t)
	plans := plansOf(t, ctx)
	r := file(t, ctx, svc, api.FileInput{Title: "Fix the links", Deliverables: []api.Deliverable{
		{Title: "links fixed", Check: "make lint-links"}, {Title: "a note in the module"},
	}})
	finish := api.FinishInput{ID: r.ID, By: builder}
	if _, err := plans.Finish(ctx, finish); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("finish with unverified deliverables: %v, want refused", err)
	}
	verify := func(i int) {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: r.ID})
		if err != nil {
			t.Fatal(err)
		}
		o := got.Object
		f, _, _ := cwmapi.DecodeCapability[api.RequestFields](o, api.CapRequest)
		f.Deliverables[i].Verified = true
		_ = o.SetCapability(api.CapRequest, f)
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	verify(0)
	if _, err := plans.Finish(ctx, finish); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("finish with one of two verified: %v, want refused", err)
	}
	if _, err := plans.Finish(ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDropped, By: intake}); err != nil {
		t.Fatalf("dropping needs no verified deliverable: %v", err)
	}
	r2 := file(t, ctx, svc, api.FileInput{Title: "Fix the links again", Deliverables: []api.Deliverable{{Title: "links fixed"}}})
	r = r2
	finish.ID = r2.ID
	verify(0)
	if _, err := plans.Finish(ctx, finish); err != nil {
		t.Fatalf("finish with every deliverable verified: %v", err)
	}
	if st := state(t, ctx, svc, r2.ID); st != api.PlanDone {
		t.Fatalf("finished request = %s, want done", st)
	}
}

// A request with no deliverable has nothing to wait on to finish.
func TestDoneWithoutDeliverables(t *testing.T) {
	t.Parallel()
	ctx, cwm, svc := world(t)
	r := file(t, ctx, svc, api.FileInput{Title: "Look at the logs"})
	walk(t, ctx, cwm, r.ID, api.PlanReady, api.StateDone)
}

// Depend refuses an edge that would make a request wait on itself.
func TestDependRefusesACycle(t *testing.T) {
	t.Parallel()
	ctx, _, svc := world(t)
	a := file(t, ctx, svc, api.FileInput{Title: "a"})
	b := file(t, ctx, svc, api.FileInput{Title: "b", DependsOn: []string{a.ID}})
	c := file(t, ctx, svc, api.FileInput{Title: "c", DependsOn: []string{b.ID}})
	_, err := svc.Depend(ctx, api.DependInput{ID: a.ID, On: c.ID})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("a on c (c → b → a): %v, want a cycle refused", err)
	}
	if _, err := svc.Depend(ctx, api.DependInput{ID: a.ID, On: a.ID}); err == nil {
		t.Fatal("a request depending on itself was accepted")
	}
	got, err := svc.Depend(ctx, api.DependInput{ID: c.ID, On: a.ID})
	if err != nil || len(got.Request.DependsOn) != 2 {
		t.Fatalf("c on a (no cycle): %+v, %v", got.Request.DependsOn, err)
	}
}

// List filters by module and state.
func TestListByModuleAndState(t *testing.T) {
	t.Parallel()
	ctx, _, svc := world(t)
	a := file(t, ctx, svc, api.FileInput{Title: "a"})
	file(t, ctx, svc, api.FileInput{Title: "b"})
	if _, err := svc.File(ctx, api.FileInput{Title: "elsewhere", By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Block(ctx, api.BlockInput{ID: a.ID, Note: "waits on a person", By: intake}); err != nil {
		t.Fatal(err)
	}
	count := func(in api.ListInput) int {
		out, err := svc.List(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return len(out.Requests)
	}
	if n := count(api.ListInput{Module: "billing"}); n != 2 {
		t.Errorf("billing: %d, want 2", n)
	}
	if n := count(api.ListInput{State: api.PlanBlocked}); n != 1 {
		t.Errorf("blocked: %d, want 1", n)
	}
	if n := count(api.ListInput{State: api.PlanReady}); n != 2 {
		t.Errorf("ready: %d, want 2", n)
	}
	if n := count(api.ListInput{}); n != 3 {
		t.Errorf("all: %d, want 3", n)
	}
}

// A module's pane shows the Requests panel — and a request's does not — drawn
// by a view the GUI registers under the name the panel gives.
func TestAModuleShowsItsRequestsPanel(t *testing.T) {
	t.Parallel()
	ctx, _, svc := world(t)
	r := file(t, ctx, svc, api.FileInput{Title: "a"})
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	panel := func(id string) *paneapi.ObjectPanel {
		out, err := pane.Detail(ctx, paneapi.DetailInput{ID: id})
		if err != nil {
			t.Fatalf("Detail %s: %v", id, err)
		}
		for i, p := range out.Panels {
			if p.Name == "Requests" {
				return &out.Panels[i]
			}
		}
		return nil
	}
	p := panel("billing")
	if p == nil || p.Content == nil || p.Content.Kind != "extension" {
		t.Fatalf("billing's Requests panel = %+v, want an extension panel", p)
	}
	if panel(r.ID) != nil {
		t.Errorf("a request shows the Requests panel; only a module should")
	}
	tsx, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "app", "ktw-gui", "frontend", "src", "sections", "workspace", "RenderDefView.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	registry := string(tsx)
	if i := strings.Index(registry, "UI_EXTENSIONS"); i >= 0 {
		registry = registry[i:]
	}
	name := p.Content.Extension
	if !strings.Contains(registry, "\n  "+name+": ") && !strings.Contains(registry, "\n  '"+name+"': ") {
		t.Errorf("the panel names the view %q, which the GUI's UI_EXTENSIONS does not register", name)
	}
}

// Every hook the pack ships runs a method the service declares.
func TestTheShippedHooksNameServiceMethods(t *testing.T) {
	t.Parallel()
	entries, err := fs.ReadDir(contrib.FS(), "hooks")
	if err != nil {
		t.Fatal(err)
	}
	methods := map[string]bool{}
	for _, def := range []meta.ServiceDef{api.RequestServiceDef, api.ReleaseServiceDef, api.StreamServiceDef, api.PlanningServiceDef, api.TriggerServiceDef} {
		for m := range def.Methods {
			methods["ktw-agents:"+def.Name+"/"+ktwreflect.ToKebab(m)] = true
		}
	}
	for _, e := range entries {
		raw, err := fs.ReadFile(contrib.FS(), "hooks/"+e.Name())
		if err != nil {
			t.Fatal(err)
		}
		var h cwmapi.Hook
		if err := config.DecodeBytes(raw, marshal.YAML, &h); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if !methods[h.URI] {
			t.Errorf("%s runs %s, which is no method of the planification services", e.Name(), h.URI)
		}
	}
}

// verificationOf is the verification service of the test's application: it
// reads the lint and verify points CWM runs.
func verificationOf(t *testing.T, ctx context.Context) verificationapi.VerificationService {
	t.Helper()
	v, err := application.UseService[verificationapi.VerificationService](ctx)
	if err != nil {
		t.Fatalf("resolve VerificationService: %v", err)
	}
	return v
}

// `about` and `on` take an object's id: free text is refused before anything
// is written, saying what to give.
func TestFilingWithAnAboutThatIsNoObjectIsRefused(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	_, err := req.File(ctx, api.FileInput{Title: "Code review", Module: "billing", About: "check for code", By: person})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "not the id of an object") {
		t.Fatalf("File with about = free text: %v; want refused, saying it takes an object's id", err)
	}
	got, _ := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "billing", Type: api.RequestType})
	if len(got.Objects) != 0 {
		t.Errorf("a refused filing left %d requests", len(got.Objects))
	}
}

// What a request promised can be put right while it is open: a broken check is
// revised (the deliverables that did not change keep their verification), the
// checks run on demand, and one no command can check is verified by hand by a
// person, with what they saw. Done still needs every deliverable verified.
func TestARequestsDeliverablesAreRevisedCheckedAndVerifiedByHand(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	plans := plansOf(t, ctx)
	session := cwmapi.Actor{ID: "claude-test", Kind: "agent"}
	r := file(t, ctx, req, api.FileInput{Title: "Ship it", Module: "billing", Deliverables: []api.Deliverable{
		{Title: "broken check", Check: "exit 3"}, {Title: "passing check", Check: "go version"}, {Title: "seen in the GUI"},
	}})

	got, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 3 || got.Results[0].Passed || !got.Results[1].Passed || got.Results[2].Passed || got.Results[2].Error == "" {
		t.Fatalf("results %+v; want the broken one failed, the passing one passed, the one without a check not run", got.Results)
	}
	if _, err := plans.Finish(ctx, api.FinishInput{ID: r.ID, By: person}); err == nil || !strings.Contains(err.Error(), "requests revise") {
		t.Fatalf("finishing with unverified deliverables = %v; want refused, naming how to put it right", err)
	}

	revised, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: session, Note: "the flag was renamed", Deliverables: []api.Deliverable{
		{Title: "broken check", Check: "go env GOOS"}, {Title: "passing check", Check: "go version"}, {Title: "seen in the GUI"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	d := revised.Request.Deliverables
	if d[0].Verified || !d[1].Verified || !strings.Contains(revised.Request.Details, "the flag was renamed") {
		t.Fatalf("after revising: %+v, details %q; want the changed one unverified, the unchanged one kept, why recorded", d, revised.Request.Details)
	}
	if got, err = req.Check(ctx, api.CheckInput{ID: r.ID, By: session}); err != nil || !got.Results[0].Passed {
		t.Fatalf("checking again: %+v, %v; want the revised check passed", got.Results, err)
	}

	if _, err := req.Verify(ctx, api.VerifyInput{ID: r.ID, Deliverable: "seen in the GUI", Evidence: "looked", By: session}); err == nil {
		t.Error("an agent verified a deliverable by hand")
	}
	if _, err := req.Verify(ctx, api.VerifyInput{ID: r.ID, Deliverable: "seen in the GUI", By: person}); err == nil {
		t.Error("a deliverable was verified by hand without evidence")
	}
	v, err := req.Verify(ctx, api.VerifyInput{ID: r.ID, Deliverable: "seen in the GUI", Evidence: "the panel lists the release", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if hand := v.Request.Deliverables[2]; !hand.Verified || hand.VerifiedBy == nil || hand.VerifiedBy.ID != person.ID || hand.Evidence == "" {
		t.Fatalf("verified by hand: %+v; want verified, by the person, with the evidence", hand)
	}
	if _, err := plans.Finish(ctx, api.FinishInput{ID: r.ID, By: person}); err != nil {
		t.Fatalf("finishing with every deliverable verified: %v", err)
	}
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: session, Note: "late", Deliverables: d}); err == nil {
		t.Error("an ended request was revised")
	}
}
