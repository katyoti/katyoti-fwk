// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/usecases/api"
	usecaseslocal "github.com/katyoti/katyoti-fwk/internal/extensions/usecases/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// useCasePack is what a use-case needs from the catalog: its type, naming the
// `manual` workflow, and that workflow AS SHIPPED by ktw-methods.
func useCasePack(t testing.TB) application.Extension {
	return cwmapp.Pack(fstest.MapFS{
		"object-types/use-case.yaml": {Data: []byte("name: use-case\nfolder: use-cases\ncapabilities: [container, content, want, checkable, lifecycle]\nworkflow: manual\n")},
		"workflows/manual.yaml":      {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/manual.yaml")},
		// A split writes its trail as a `decision`; declared types are enforced.
		"object-types/decision.yaml": {Data: []byte("name: decision\nfolder: decisions\ncapabilities: [container, content]\n")},
	})
}

// hostExts is what the process holding CWM runs here: the use-case pack, and
// the journal — the lifecycle history, where a retire's or a split's note is.
func hostExts(tb testing.TB) []application.Extension {
	return []application.Extension{
		useCasePack(tb),
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""), journallocal.Extension(), cwmapp.SystemModules(tb),
	}
}

// who retires or splits, as recorded in the lifecycle history.
var who = cwmapi.Actor{ID: "jdoe", Kind: cwmapi.ActorPerson}

func setup(t *testing.T) (context.Context, cwmapi.CWMService, api.UseCasesService) {
	t.Helper()
	return setupIn(t, func(tb testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
		ctx, cwm := cwmapp.New(tb, append(exts, hostExts(tb)...)...)
		// The journal records on its own goroutine: drain it before the
		// workspace is removed.
		tb.Cleanup(func() { appcore.CloseApplicationScope(ctx) })
		return ctx, cwm
	})
}

// setupRemote is the shape of the binary hosting this service: CWM is in
// another process, reached through the generated bus client.
func setupRemote(t *testing.T) (context.Context, cwmapi.CWMService, api.UseCasesService) {
	t.Helper()
	return setupIn(t, func(tb testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
		return cwmapp.RemoteHosting(tb, hostExts(tb), exts...)
	})
}

func setupIn(t *testing.T, boot func(testing.TB, ...application.Extension) (context.Context, cwmapi.CWMService)) (context.Context, cwmapi.CWMService, api.UseCasesService) {
	t.Helper()
	ctx, cwm := boot(t, usecaseslocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.UseCasesService](ctx)
	if err != nil {
		t.Fatalf("resolve UseCasesService: %v", err)
	}
	return ctx, cwm, svc
}

func goal(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, local string) string {
	t.Helper()
	g := cwmapi.NewContainerObject("m", local, "goal", "m", "Goal "+local, "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: g}); err != nil {
		t.Fatal(err)
	}
	return g.ID
}

func addUC(t *testing.T, ctx context.Context, svc api.UseCasesService, title string, serves ...string) api.UseCase {
	t.Helper()
	out, err := svc.Add(ctx, api.AddInput{Module: "m", Title: title, Serves: serves})
	if err != nil {
		t.Fatalf("Add %q: %v", title, err)
	}
	return out.UseCase
}

func read(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) cwmapi.Object {
	t.Helper()
	m, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: id})
	if err != nil {
		t.Fatalf("ObjectGet %s: %v", id, err)
	}
	return got.Object
}

// status is the use-case's lifecycle state; none is the workflow's initial.
func status(o cwmapi.Object) string {
	if l, ok := o.Lifecycle(); ok && l.State != "" {
		return l.State
	}
	return api.StateInitial
}

// lastNote is the note of the use-case's last transition, as the journal
// recorded it (the object stores only its state) — read where the journal
// writes, through the CWM that holds it, once it starts with want.
func lastNote(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		note := ""
		read, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: journalapi.DayID(time.Now()), Name: journalapi.EntriesFile})
		if err == nil {
			sc := bufio.NewScanner(bytes.NewReader(read.Content))
			sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for sc.Scan() {
				var r journalapi.Record
				if json.Unmarshal(sc.Bytes(), &r) == nil && r.ID == id && r.Action == journalapi.ActionTransitioned {
					note = r.Note
				}
			}
		}
		if strings.HasPrefix(note, want) || time.Now().After(deadline) {
			return note
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestUseCasesService drives every verb against a real CWM service and reads
// each effect back through the Object API.
func TestUseCasesService(t *testing.T) {
	ctx, cwm, svc := setup(t)
	useCasesScenario(t, ctx, cwm, svc)
}

// TestUseCasesServiceOverTheBus runs the same scenario with CWM in another
// process, as in the binary that hosts this service.
func TestUseCasesServiceOverTheBus(t *testing.T) {
	ctx, cwm, svc := setupRemote(t)
	useCasesScenario(t, ctx, cwm, svc)
}

func useCasesScenario(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, svc api.UseCasesService) {
	t.Helper()
	g1, g2 := goal(t, ctx, cwm, "G-001"), goal(t, ctx, cwm, "G-002")

	// Add — a pending use-case with a want, serving g1 by an edge.
	uc := addUC(t, ctx, svc, "Export a report", g1)
	o := read(t, ctx, cwm, uc.ID)
	if o.Type != "use-case" || status(o) != api.StateInitial || o.Capabilities["want"]["reachability"] != "closable" ||
		len(o.Edges) != 1 || o.Edges[0].Kind != "serves" || o.Edges[0].To != g1 {
		t.Fatalf("stored use-case: %+v", o)
	}

	// Goals — what it serves, with the goal's title.
	gs, err := svc.Goals(ctx, api.GoalsInput{ID: uc.ID})
	if err != nil || len(gs.Goals) != 1 || gs.Goals[0].ID != g1 || gs.Goals[0].Type != "goal" || gs.Goals[0].Title != "Goal G-001" {
		t.Fatalf("Goals: %v %+v", err, gs.Goals)
	}

	// Uncovered — no criterion yet.
	unc, err := svc.Uncovered(ctx, api.UncoveredInput{Module: "m"})
	if err != nil || len(unc.UseCases) != 1 || unc.UseCases[0].ID != uc.ID {
		t.Fatalf("Uncovered before a criterion: %v %+v", err, unc.UseCases)
	}

	// AttachCriterion — an acceptance-criterion under it, with a command probe.
	ac, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{UseCase: uc.ID, Title: "The report opens", Probe: "true"})
	if err != nil {
		t.Fatal(err)
	}
	co := read(t, ctx, cwm, ac.Criterion.ID)
	if co.Type != "acceptance-criterion" || co.ParentID() != uc.ID || co.Capabilities["checkable"]["kind"] != "command" || co.Capabilities["checkable"]["probe"] != "true" {
		t.Fatalf("stored criterion: %+v", co)
	}
	ac2, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{UseCase: uc.ID, Title: "Not measurable yet"})
	if err != nil {
		t.Fatal(err)
	}
	if k := read(t, ctx, cwm, ac2.Criterion.ID).Capabilities["checkable"]["kind"]; k != "none" {
		t.Fatalf("a criterion with no probe: checkable.kind = %v, want none", k)
	}
	unc, _ = svc.Uncovered(ctx, api.UncoveredInput{Module: "m"})
	if len(unc.UseCases) != 0 {
		t.Fatalf("Uncovered after a criterion: %+v", unc.UseCases)
	}

	// Split — two successors, each serving g1, one adopting the criterion; the
	// source stays, marked split (the journal records the split itself).
	sp, err := svc.Split(ctx, api.SplitInput{ID: uc.ID, Actor: who, Reason: "Two needs in one", Successors: []api.SplitSuccessor{
		{Title: "Export as PDF", Criteria: []string{ac.Criterion.ID}},
		{Title: "Export as CSV", Criteria: []string{ac2.Criterion.ID}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.Successors) != 2 {
		t.Fatalf("Split: %+v", sp)
	}
	for _, s := range sp.Successors {
		so := read(t, ctx, cwm, s.ID)
		if so.Type != "use-case" || status(so) != api.StateInitial || so.Name == "" ||
			len(so.Edges) != 1 || so.Edges[0].To != g1 {
			t.Fatalf("successor %s: %+v", s.ID, so)
		}
	}
	if got := read(t, ctx, cwm, ac.Criterion.ID).ParentID(); got != sp.Successors[0].ID {
		t.Fatalf("the adopted criterion hangs under %s, want %s", got, sp.Successors[0].ID)
	}
	if src := read(t, ctx, cwm, uc.ID); status(src) != api.StateDropped || !strings.HasPrefix(lastNote(t, ctx, cwm, uc.ID, "split"), "split") {
		t.Fatal("the split source is not marked split")
	}

	// Retire — a second use-case, retired with a reason; it leaves Uncovered.
	other := addUC(t, ctx, svc, "Print it", g2)
	if _, err := svc.Retire(ctx, api.RetireInput{ID: other.ID, Actor: who, Reason: "Nobody prints."}); err != nil {
		t.Fatal(err)
	}
	ro := read(t, ctx, cwm, other.ID)
	prose := ro.Body
	if status(ro) != api.StateDropped || !strings.HasPrefix(lastNote(t, ctx, cwm, other.ID, "retired"), "retired") || !strings.Contains(prose, "## Retired\n\nNobody prints.") {
		t.Fatalf("retired use-case: status %q prose %q", status(ro), prose)
	}
	unc, _ = svc.Uncovered(ctx, api.UncoveredInput{Module: "m"})
	for _, u := range unc.UseCases {
		if u.ID == other.ID || u.ID == uc.ID {
			t.Fatalf("a retired or split use-case is listed uncovered: %s", u.ID)
		}
	}
}

func TestUseCasesAddRefusesServingANonGoal(t *testing.T) {
	ctx, _, svc := setup(t)
	uc := addUC(t, ctx, svc, "A")
	if _, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "B", Serves: []string{uc.ID}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("serving a use-case: err = %v", err)
	}
	if _, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "C", Serves: []string{"m.G-404"}}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("serving a missing goal: err = %v", err)
	}
}

func TestUseCasesSplitNeedsTwoAndAReason(t *testing.T) {
	ctx, _, svc := setup(t)
	uc := addUC(t, ctx, svc, "A")
	two := []api.SplitSuccessor{{Title: "A1"}, {Title: "A2"}}
	if _, err := svc.Split(ctx, api.SplitInput{ID: uc.ID, Actor: who, Reason: "r", Successors: two[:1]}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a split into one: err = %v", err)
	}
	if _, err := svc.Split(ctx, api.SplitInput{ID: uc.ID, Actor: who, Successors: two}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a split with no reason: err = %v", err)
	}
}

func TestUseCasesRetiredIsFinal(t *testing.T) {
	ctx, _, svc := setup(t)
	uc := addUC(t, ctx, svc, "A")
	if _, err := svc.Retire(ctx, api.RetireInput{ID: uc.ID, Actor: who}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Retire(ctx, api.RetireInput{ID: uc.ID, Actor: who}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("retiring twice: err = %v", err)
	}
	if _, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{UseCase: uc.ID, Title: "x"}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("a criterion on a retired use-case: err = %v", err)
	}
	if _, err := svc.Split(ctx, api.SplitInput{ID: uc.ID, Actor: who, Reason: "r", Successors: []api.SplitSuccessor{{Title: "1"}, {Title: "2"}}}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("splitting a retired use-case: err = %v", err)
	}
}

func TestUseCasesCriterionOnlyUnderAUseCase(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := goal(t, ctx, cwm, "G-001")
	if _, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{UseCase: g, Title: "x"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a criterion under a goal: err = %v", err)
	}
}
