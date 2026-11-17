// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Planning comes with a capability, never with every module:
// planification's panels - Requests, Planification, the
// person's queue - show only on a module carrying `planned`. A module created
// plain carries it; one created with a domain capability of its own (a study,
// a dossier) does not; the modules a project already plans are given it when
// the project opens.

// planningTabs are the tabs planification's panels put on a module.
var planningTabs = []string{"tab:requests", "tab:planning-deck", "tab:person-queue"}

// aDomain is a capability a domain gives the modules it creates (as a study
// domain gives asis-study to a study): planification knows nothing of it.
var aDomain = fstest.MapFS{"capabilities/a-domain.yaml": {Data: []byte("name: a-domain\n")}}

func tabsOf(t *testing.T, ctx context.Context, id string) []string {
	t.Helper()
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := pane.Detail(ctx, paneapi.DetailInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var tabs []string
	for _, p := range d.Panels {
		tabs = append(tabs, p.ID)
	}
	return tabs
}

func carries(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) bool {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	_, has := got.Object.Capabilities[api.CapPlanned]
	return has
}

func TestAPlainModuleIsPlannedAndShowsPlanning(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := worldWith(t, aDomain)
	// on_create hooks run after the write, in the background: wait for it.
	for i := 0; i < 100 && !carries(t, ctx, cwm, "billing"); i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if !carries(t, ctx, cwm, "billing") {
		t.Fatal("a module created plain does not carry planned")
	}
	tabs := tabsOf(t, ctx, "billing")
	for _, want := range planningTabs {
		if !slices.Contains(tabs, want) {
			t.Errorf("a planned module lacks %s: %v", want, tabs)
		}
	}
}

func TestAModuleOfADomainShowsNoPlanning(t *testing.T) {
	t.Parallel()
	ctx, cwm, svc := worldWith(t, aDomain)
	obj := cwmapi.NewContainerObject("study", "study", "module", "", "A study", "")
	obj.Capabilities = map[string]map[string]any{"a-domain": {}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "study", Object: obj}); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "study"})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := svc.PlanModule(ctx, cwmapi.HookCall{Point: cwmapi.HookOnCreate, Object: &got.Object}); err != nil || out.Planned {
		t.Fatalf("PlanModule on a module of a domain = %+v, %v; want not planned", out, err)
	}
	time.Sleep(500 * time.Millisecond) // and the hook the create fired left it so
	if carries(t, ctx, cwm, "study") {
		t.Fatal("a module created with a domain capability of its own was given planned")
	}
	tabs := tabsOf(t, ctx, "study")
	for _, tab := range planningTabs {
		if slices.Contains(tabs, tab) {
			t.Errorf("a module that is not planned shows %s: %v", tab, tabs)
		}
	}
}

func TestTheModulesAProjectPlansAreGivenPlannedWhenItOpens(t *testing.T) {
	t.Parallel()
	ctx, cwm, svc := worldWith(t, aDomain)
	for _, id := range []string{"worked", "idle"} {
		obj := cwmapi.NewContainerObject(id, id, "module", "", id, "")
		obj.Capabilities = map[string]map[string]any{"a-domain": {}}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: id, Object: obj}); err != nil {
			t.Fatal(err)
		}
	}
	// A module that held requests before the capability existed.
	if _, err := svc.File(ctx, api.FileInput{Title: "Fix the totals", Module: "worked", By: person}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.PlanModules(ctx, api.EnsureInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(out.Planned, "worked") || slices.Contains(out.Planned, "idle") {
		t.Errorf("planned %v; want worked, never idle", out.Planned)
	}
	if !carries(t, ctx, cwm, "worked") || carries(t, ctx, cwm, "idle") {
		t.Error("the module with requests is not planned, or the one without is")
	}
	again, err := svc.PlanModules(ctx, api.EnsureInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Planned) != 0 {
		t.Errorf("a second run planned %v; want nothing (idempotent)", again.Planned)
	}
}

// : a module of another type name - its type
// carries the module level capability - is planned when created plain, as a
// module of the type module is.
func TestAModuleOfAnotherTypeNameIsPlanned(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := worldWith(t, fstest.MapFS{
		"object-types/software-module.yaml": {Data: []byte("name: software-module\nlabel: Software module\ntree: node\ncapabilities: [module]\n")},
	})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "sw", Object: cwmapi.Object{ID: "sw", Type: "software-module", Name: "Software"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && !carries(t, ctx, cwm, "sw"); i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if !carries(t, ctx, cwm, "sw") {
		t.Fatal("a software module created plain does not carry planned")
	}
}
