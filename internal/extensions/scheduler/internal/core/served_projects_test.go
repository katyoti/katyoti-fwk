// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : the queue, the modes, the prune and the waits read every
// served project's modules - two projects each holding a module of one name
// are two module references <key>:<module>, never one.

// servedRepository makes a member repository: its project object of key and
// uuid, and the modules given - created through the CWM once it is served.
func servedRepository(t *testing.T, root, key, uuid string) string {
	t.Helper()
	dir := filepath.Join(root, "repo-"+key)
	if err := os.MkdirAll(filepath.Join(dir, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "id: " + key + "\ntype: project\nname: " + key + "\ncapabilities:\n  project:\n    uuid: " + uuid + "\n"
	if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// servedUniverse runs an application whose real CWM serves the universe at
// root attaching the members given (the store set when there are several).
func servedUniverse(t *testing.T, root string, members ...string) (context.Context, cwmapi.CWMService) {
	t.Helper()
	return cwmapp.Members(t, root, members,
		cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")}}),
		cwmapp.SystemModules(t))
}

// moduleIn creates a module object in the module ref names (qualified to
// name its project): a user module, or the requests module.
func moduleIn(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, ref string) {
	t.Helper()
	name := bareModule(ref)
	typ := "module"
	if name == api.RequestsModule {
		typ = cwmapi.SystemModuleType
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: ref, Object: cwmapi.NewContainerObject(name, name, typ, "", name, "")}); err != nil {
		t.Fatalf("create module %s: %v", ref, err)
	}
}

// TestTwoServedProjectsWithOneModuleNameKeepTheirQueueModesPruneAndWaits: a
// universe serves alpha and beta, each holding scheduler and the requests
// module, alpha alone crm, over one SQLite store and the real CWM store set.
// The unfiltered queue lists both projects' to-dos, each project's requests
// module's included; the modes answer alpha:scheduler and beta:scheduler,
// each its own; a prune of beta:scheduler removes beta's past done to-do
// only, an unfiltered one alpha's; the waits list both projects' waits, each
// qualified; a bare scheduler filter is refused, naming both references, a
// bare crm names alpha's. One project served alone keeps its modules bare.
func TestTwoServedProjectsWithOneModuleNameKeepTheirQueueModesPruneAndWaits(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	alpha := servedRepository(t, root, "alpha", alphaUUID)
	beta := servedRepository(t, root, "beta", betaUUID)
	ctx, cwm := servedUniverse(t, root, alpha, beta)
	for _, ref := range []string{"alpha:scheduler", "beta:scheduler", "alpha:crm", "alpha:" + api.RequestsModule, "beta:" + api.RequestsModule} {
		moduleIn(t, ctx, cwm, ref)
	}
	mods, err := modules(ctx)
	if want := []string{"alpha:crm", "alpha:scheduler", "alpha:" + api.RequestsModule, "beta:scheduler", "beta:" + api.RequestsModule}; err != nil || !slices.Equal(mods, want) {
		t.Fatalf("modules = %v (%v), want %v", mods, err, want)
	}

	st, err := OpenSQLiteStore(schedulerEnv{dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewOver(st)
	by := cwmapi.Actor{ID: "test", Kind: "agent"}
	put := func(module, title string) api.Todo {
		t.Helper()
		out, err := svc.Put(ctx, api.PutInput{Todo: api.Todo{Module: module, Target: bareModule(module), Title: title, Owner: "test",
			For: api.ForAgent, Start: api.StartAuto, Priority: "P1", State: api.StateReady}})
		if err != nil {
			t.Fatalf("put %s: %v", title, err)
		}
		return out.Todo
	}
	steps := map[string]string{}
	for _, ref := range mods {
		steps[put(ref, ref+"'s step").ID] = ref
	}

	// The unfiltered queue: every project's to-dos.
	q, err := svc.Queue(ctx, api.Filter{})
	if err != nil {
		t.Fatalf("the unfiltered queue: %v", err)
	}
	queued := map[string]bool{}
	for _, e := range q.Queue {
		queued[e.Todo.ID] = true
	}
	for id, ref := range steps {
		if !queued[id] {
			t.Errorf("the unfiltered queue misses %s's to-do %s", ref, id)
		}
	}

	// The modes: each project's module its own.
	if _, err := svc.Mode(ctx, api.ModeInput{Module: "beta:scheduler", Mode: api.ModeOff, By: by}); err != nil {
		t.Fatal(err)
	}
	modes, err := svc.Modes(ctx, api.ModesInput{})
	if err != nil {
		t.Fatalf("modes: %v", err)
	}
	got := map[string]string{}
	for _, m := range modes.Modules {
		got[m.Module] = m.Mode
	}
	if got["alpha:scheduler"] != api.ModeManual || got["beta:scheduler"] != api.ModeOff || len(got) != len(mods) {
		t.Errorf("modes = %v, want alpha:scheduler manual and beta:scheduler off among %d", got, len(mods))
	}

	// A bare name both hold is refused, naming both references; one only
	// alpha holds names alpha's.
	for _, try := range []func() error{
		func() error { _, err := svc.List(ctx, api.ListInput{Module: "scheduler"}); return err },
		func() error { _, err := svc.Prune(ctx, api.PruneInput{Module: "scheduler"}); return err },
		func() error {
			_, err := (&Waits{store: st}).List(ctx, api.ListWaitsInput{Module: "scheduler"})
			return err
		},
	} {
		if err := try(); err == nil || !strings.Contains(err.Error(), "alpha:scheduler") || !strings.Contains(err.Error(), "beta:scheduler") {
			t.Errorf("a bare scheduler filter = %v, want refused naming alpha:scheduler and beta:scheduler", err)
		}
	}
	if l, err := svc.List(ctx, api.ListInput{Module: "crm"}); err != nil || len(l.Todos) != 1 || l.Todos[0].Repository != alphaUUID {
		t.Errorf("a bare crm filter = %+v (%v), want alpha's one to-do", l.Todos, err)
	}

	// The prune: each project's past done to-dos, and only those.
	done := map[string]string{}
	for _, ref := range []string{"alpha:scheduler", "beta:scheduler"} {
		td := put(ref, ref+"'s long done step")
		if _, err := st.Archive(ctx, td.ID, func(t *api.Todo) error {
			t.Archived = time.Now().UTC().AddDate(0, 0, -400).Format(time.RFC3339Nano)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		done[ref] = td.ID
	}
	if p, err := svc.Prune(ctx, api.PruneInput{Module: "beta:scheduler"}); err != nil || p.Removed != 1 {
		t.Fatalf("prune beta:scheduler = %+v (%v), want its one past to-do removed", p, err)
	}
	if _, err := st.Get(ctx, done["beta:scheduler"]); err == nil {
		t.Errorf("beta's past done to-do is still there")
	}
	if _, err := st.Get(ctx, done["alpha:scheduler"]); err != nil {
		t.Errorf("a prune of beta:scheduler removed alpha's: %v", err)
	}
	if p, err := svc.Prune(ctx, api.PruneInput{}); err != nil || p.Removed != 1 {
		t.Fatalf("an unfiltered prune = %+v (%v), want alpha's past to-do removed", p, err)
	}

	// The waits: both projects', each named by its qualified module.
	w := &Waits{store: st}
	for _, ref := range []string{"alpha:scheduler", "beta:scheduler"} {
		if _, err := w.Add(ctx, api.AddWaitInput{Module: ref, On: "queue.RQ-1", From: "crm", What: ref + " awaits", By: by}); err != nil {
			t.Fatalf("add a wait of %s: %v", ref, err)
		}
	}
	waits, err := w.List(ctx, api.ListWaitsInput{})
	if err != nil {
		t.Fatalf("the waits: %v", err)
	}
	var named []string
	for _, x := range waits.Waits {
		named = append(named, x.Module)
	}
	slices.Sort(named)
	if !slices.Equal(named, []string{"alpha:scheduler", "beta:scheduler"}) {
		t.Errorf("the waits name %v, want alpha:scheduler and beta:scheduler", named)
	}
	if one, err := w.List(ctx, api.ListWaitsInput{Module: "beta:scheduler"}); err != nil || len(one.Waits) != 1 || one.Waits[0].Module != "beta:scheduler" {
		t.Errorf("beta:scheduler's waits = %+v (%v), want its one", one.Waits, err)
	}

	// One project served alone: its modules stay bare.
	solo := servedRepository(t, filepath.Join(root, "solo"), "gamma", "0193a0d3-a5ed-71e3-9f71-d032d268845f")
	soloCtx, soloCWM := servedUniverse(t, filepath.Join(root, "solo"), solo)
	moduleIn(t, soloCtx, soloCWM, "scheduler")
	if mods, err := modules(soloCtx); err != nil || !slices.Equal(mods, []string{"scheduler"}) {
		t.Errorf("one project's modules = %v (%v), want scheduler, bare", mods, err)
	}
}

// TestAWaitStoredBareIsNamedAndRepointedInItsOwnProject:
// a wait written while alpha alone was served keeps its module bare in its
// row; once beta, holding a scheduler too, is attached, the waits name it
// alpha:scheduler (Waits.List), and a repoint of what it waits on writes it
// back in alpha's rows - a bare module there would be refused as ambiguous.
// On the one-project universe a qualified module reaches its planning space
// through the bare id its single store knows (effectiveCapabilities).
func TestAWaitStoredBareIsNamedAndRepointedInItsOwnProject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	alpha := servedRepository(t, root, "alpha", alphaUUID)
	beta := servedRepository(t, root, "beta", betaUUID)
	st, err := OpenSQLiteStore(schedulerEnv{dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	by := cwmapi.Actor{ID: "test", Kind: "agent"}

	// alpha served alone: a wait written with its module bare.
	soloCtx, soloCWM := servedUniverse(t, filepath.Join(root, "solo"), alpha)
	moduleIn(t, soloCtx, soloCWM, "scheduler")
	space, err := planningSpace(soloCtx, "alpha:scheduler")
	if want := filepath.Join(alpha, "modules", "scheduler", ".ktw"); err != nil || !strings.EqualFold(filepath.Clean(space), filepath.Clean(want)) {
		t.Errorf("alpha:scheduler's planning space on alpha's own universe = %q (%v), want %q", space, err, want)
	}
	if _, err := (&Waits{store: st}).Add(soloCtx, api.AddWaitInput{Module: "scheduler", On: "queue.RQ-1", From: "crm", What: "alpha awaits", By: by}); err != nil {
		t.Fatalf("add a wait on alpha alone: %v", err)
	}
	stored, err := st.ListWaits(soloCtx, "scheduler")
	if err != nil || len(stored) != 1 || stored[0].Module != "scheduler" {
		t.Fatalf("the stored wait = %+v (%v), want one whose module is bare", stored, err)
	}

	// beta attached: the bare-stored wait is named by its qualified module.
	ctx, cwm := servedUniverse(t, root, alpha, beta)
	moduleIn(t, ctx, cwm, "beta:scheduler")
	w := &Waits{store: st}
	got, err := w.List(ctx, api.ListWaitsInput{})
	if err != nil || len(got.Waits) != 1 || got.Waits[0].Module != "alpha:scheduler" {
		t.Fatalf("the waits = %+v (%v), want alpha's one named alpha:scheduler", got.Waits, err)
	}

	// A repoint writes it back in alpha's rows, on what it now waits on.
	if out, err := NewOver(st).repoint(ctx, "queue.RQ-1", "queue.RQ-2"); err != nil || len(out) != 1 {
		t.Fatalf("repoint = %v (%v), want alpha's wait re-pointed", out, err)
	}
	if a, err := w.List(ctx, api.ListWaitsInput{Module: "alpha:scheduler"}); err != nil || len(a.Waits) != 1 || a.Waits[0].On != "queue.RQ-2" {
		t.Errorf("alpha:scheduler's waits after the repoint = %+v (%v), want its one on queue.RQ-2", a.Waits, err)
	}
	if b, err := w.List(ctx, api.ListWaitsInput{Module: "beta:scheduler"}); err != nil || len(b.Waits) != 0 {
		t.Errorf("beta:scheduler's waits after the repoint = %+v (%v), want none", b.Waits, err)
	}
}
