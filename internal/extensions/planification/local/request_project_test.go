// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// EVERY TO-DO IS PUT IN ITS REQUEST'S PROJECT: a
// universe serving several projects names a request's module <key>:<module>
// when it puts its to-dos, so two projects holding one module name and one
// request id keep their work apart.

const (
	alphaProject = "0193a0d3-a5ed-71e3-9f71-b810b0466a01"
	betaProject  = "0193a0d3-a5ed-71e3-9f71-c921c1577b02"
)

// memberRepository makes a member repository holding its project object of
// key and uuid.
func memberRepository(t *testing.T, root, key, uuid string) string {
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

// membersWorld is a world whose CWM serves the members given (the store set
// with several), each holding a billing module.
func membersWorld(t *testing.T, keys map[string]string) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	root := t.TempDir()
	// Registered before the world, so it runs after the world is closed and
	// before t.TempDir's own removal (which retries only 2 s): under load a
	// handle in the world's folder (the scheduler's SQLite files, a file the
	// planning loop is writing) can outlive the close on Windows.
	t.Cleanup(func() { removeEventually(t, root, 30*time.Second) })
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	// Its runtime data - the scheduler's SQLite store - in its own folder:
	// the package's other worlds share the test data root and keep the file
	// store (no project uuid).
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	var members []string
	for key, uuid := range keys {
		members = append(members, memberRepository(t, root, key, uuid))
	}
	ctx, cwm := cwmapp.Members(t, root, members, worldExtensions(t, nil)...)
	for key := range keys {
		ref := "billing"
		if len(keys) > 1 {
			ref = key + ":billing"
		}
		var err error
		for _, typ := range []string{"module", "documentation"} {
			if _, err = cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: ref, Object: cwmapi.NewContainerObject("billing", "billing", typ, "", "billing", "")}); err == nil {
				break
			}
		}
		if err != nil {
			t.Fatalf("create module %s: %v", ref, err)
		}
	}
	ctx, cwm, req := settledWorld(t, ctx, cwm)
	// The planning loop settles for longer than settledWorld's 5 s before the
	// world closes: a loaded machine runs it slower, and a loop still running
	// at the close keeps writing in the folder being removed. Registered last,
	// it runs first; an idle loop returns at once.
	t.Cleanup(func() { planlocal.Settle(ctx, time.Minute) })
	return ctx, cwm, req
}

// removeEventually removes dir, retrying while Windows refuses a file a
// closing handle still holds, up to max; what still stands then fails the
// test, naming the error.
func removeEventually(t *testing.T, dir string, max time.Duration) {
	t.Helper()
	deadline := time.Now().Add(max)
	for {
		err := os.RemoveAll(dir)
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("remove the world's folder %s: %v", dir, err)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestEveryToDoIsPutInItsRequestsProject: alpha and beta both hold billing
// and a request of one id. Planning beta's puts every step in beta,
// Request and Target bare; the intake's report adds a step and releases the
// next: both put in beta; the request's to-dos are beta's only, never
// alpha's. One project served keeps its module bare.
func TestEveryToDoIsPutInItsRequestsProject(t *testing.T) {
	ctx, _, req := membersWorld(t, map[string]string{"alpha": alphaProject, "beta": betaProject})
	ids := map[string]string{}
	for _, key := range []string{"alpha", "beta"} {
		ids[key] = bareRef(file(t, ctx, req, api.FileInput{Title: key + "'s invoices", Module: key + ":billing", Kind: api.KindBuild}).ID)
	}
	if ids["alpha"] != ids["beta"] {
		t.Fatalf("the two projects' requests are %s and %s, want one id", ids["alpha"], ids["beta"])
	}
	id := ids["beta"]
	beta := "beta:" + id
	work := workOf(t, ctx)
	listed := func() []api.WorkTodo {
		t.Helper()
		got, err := work.List(ctx, api.ListWorkInput{Request: beta})
		if err != nil {
			t.Fatalf("beta's to-dos: %v", err)
		}
		return got.Todos
	}
	if len(listed()) == 0 {
		if _, err := work.Plan(ctx, api.PlanWorkInput{Request: beta, By: person}); err != nil {
			t.Fatalf("plan %s: %v", beta, err)
		}
	}
	inBeta := func(what string, todos []api.WorkTodo) {
		t.Helper()
		if len(todos) == 0 {
			t.Fatalf("%s: no to-do", what)
		}
		for _, td := range todos {
			if td.Repository != betaProject || td.Module != "billing" || td.Request != id || td.Target != id {
				t.Errorf("%s: %s %s is in repository %q module %q, request %q target %q - want beta's billing, both bare",
					what, td.ID, td.Packet.Step, td.Repository, td.Module, td.Request, td.Target)
			}
		}
	}
	planned := listed()
	inBeta("the plan", planned)
	// Alpha's request of the same id keeps its own to-dos, in alpha.
	alpha := "alpha:" + id
	ofAlpha, err := work.List(ctx, api.ListWorkInput{Request: alpha})
	if err == nil && len(ofAlpha.Todos) == 0 {
		if _, err = work.Plan(ctx, api.PlanWorkInput{Request: alpha, By: person}); err == nil {
			ofAlpha, err = work.List(ctx, api.ListWorkInput{Request: alpha})
		}
	}
	if err != nil || len(ofAlpha.Todos) == 0 {
		t.Fatalf("alpha's to-dos: %v (%v)", ofAlpha.Todos, err)
	}
	for _, td := range ofAlpha.Todos {
		if td.Repository != alphaProject || td.Request != id {
			t.Errorf("alpha's to-do %s is in repository %q request %q, want alpha's", td.ID, td.Repository, td.Request)
		}
	}
	if titles := titlesOf(planned); !titled(titles, "beta's invoices") || titled(titles, "alpha's invoices") {
		t.Errorf("beta's to-dos are titled %v: want beta's request only", titles)
	}

	// The intake's report adds a step: it and the next step are put in beta.
	var intake api.WorkTodo
	for _, td := range planned {
		if td.Packet.Step == api.StepIntake {
			intake = td
		}
	}
	if intake.ID == "" {
		t.Fatalf("no intake in %v", planned)
	}
	pullAndReport(t, ctx, intake.ID, session, schedapi.Result{Summary: "assessed",
		More: map[string]any{"add": []map[string]any{{"step": "measure", "task": "coder", "instructions": "Measure the invoices."}}}})
	after := listed()
	inBeta("after the intake", after)
	var added bool
	for _, td := range after {
		added = added || td.Packet.Step == "measure"
	}
	if !added {
		t.Errorf("the added step is not among beta's to-dos: %v", stepsOf(after))
	}

	// One project served: its module bare, as before.
	soloCtx, _, soloReq := membersWorld(t, map[string]string{"gamma": "0193a0d3-a5ed-71e3-9f71-d032d2688c03"})
	r := file(t, soloCtx, soloReq, api.FileInput{Title: "Gamma's invoices", Module: "billing", Kind: api.KindBuild})
	soloWork := workOf(t, soloCtx)
	got, err := soloWork.List(soloCtx, api.ListWorkInput{Request: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Todos) == 0 {
		if _, err := soloWork.Plan(soloCtx, api.PlanWorkInput{Request: r.ID, By: person}); err != nil {
			t.Fatalf("plan %s: %v", r.ID, err)
		}
		if got, err = soloWork.List(soloCtx, api.ListWorkInput{Request: r.ID}); err != nil {
			t.Fatal(err)
		}
	}
	for _, td := range got.Todos {
		if td.Module != "billing" || td.Request != r.ID {
			t.Errorf("one project's to-do %s: module %q request %q, want billing and %s", td.ID, td.Module, td.Request, r.ID)
		}
	}
}

// bareRef is a reference's bare id.
func bareRef(ref string) string {
	if i := strings.Index(ref, ":"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

func titlesOf(todos []api.WorkTodo) []string {
	var out []string
	for _, td := range todos {
		out = append(out, td.Title)
	}
	return out
}

func stepsOf(todos []api.WorkTodo) []string {
	var out []string
	for _, td := range todos {
		out = append(out, td.Packet.Step)
	}
	return out
}

// titled reports a title holding what.
func titled(titles []string, what string) bool {
	return slices.ContainsFunc(titles, func(s string) bool { return strings.Contains(s, what) })
}
