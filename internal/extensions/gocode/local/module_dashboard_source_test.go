// SPDX-License-Identifier: Apache-2.0

package local

import (
	"encoding/json"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	stdreflect "reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/reflect"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	gccore "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/core"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE MODULE DASHBOARD'S SOURCE is gocode's module dashboard, addressed where
// it is published (ktw-gocode): it answers a page the generic renderer draws,
// so the blocks a person reads are held here, on a module written through CWM —
// the health, the tier, the use-cases as a stacked bar with its legend, the
// audit counters, the promote gates and the facets.
func TestModuleDashboardAnswersAPageWithItsBlocks(t *testing.T) {
	ctx, cwm := newObjectCtx(t)
	svc := &gccore.GocodeServiceImpl{}
	seedProject(t, ctx, cwm, "proj")
	mod, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{Name: "dash", ProjectID: "proj", Kind: gcapi.ModuleKindLibrary})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UCCreate(ctx, gcapi.UCCreateInput{NodeID: mod.Module.ID, UCID: "UC-001", Title: "it shows", Severity: gcapi.UCSeverityCore}); err != nil {
		t.Fatal(err)
	}
	def, err := svc.ModuleDashboard(ctx, gcapi.ModuleDashboardInput{ID: mod.Module.ID})
	if err != nil {
		t.Fatal(err)
	}
	if def.Kind != "page" {
		t.Fatalf("the dashboard is a page, got kind %q", def.Kind)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the page must hold to the DSL: %v", err)
	}
	raw, _ := json.Marshal(def)
	page := string(raw)
	for _, want := range []string{
		`"text":"Module status"`, `gocode — kind: library`, // the head
		`"title":"Tier"`, `"icon":"trending-up"`, `"text":"target `, // tier against its target
		`"title":"Use-cases"`, `"segments":[`, `"label":"no-test"`, `"legend":true`, `"1 total"`, // the stacked bar
		`"title":"Audit + open decisions"`, `"0 critical"`, // the counters
		`"title":"Promote gates"`, `"badgeColor":"orange"`, `UC with no test`, // what still blocks
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the dashboard page lacks %s:\n%s", want, page)
		}
	}
}

// The shipped panel (ktw-code's pack) is a source: object-pane/Detail returns
// it with gocode's module-dashboard verb, declared as a page — on a module
// with code packages (the panel's when).
func TestTheShippedModuleDashboardIsSourcedFromModuleStatus(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":            {Data: []byte("name: module\ntree: node\ncapabilities: [module, code, module-health]\n")},
		"capabilities/module-health.yaml":     {Data: []byte("name: module-health\n")},
		"capabilities/module.yaml":            {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/capabilities/module.yaml")},
		"capabilities/code.yaml":              {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/capabilities/code.yaml")},
		"data-types/cap-module.dsl.yaml":      {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/data-types/cap-module.dsl.yaml")},
		"object-panels/module-dashboard.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-panels/module-dashboard.yaml")},
	}), panelocal.Extension())
	m := cwmapi.NewContainerObject("m", "m", "module", "", "m", "")
	m.Capabilities["code"] = map[string]any{"code_packages": []any{"internal/extensions/m"}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: m}); err != nil {
		t.Fatalf("create module m: %v", err)
	}
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := pane.Detail(ctx, paneapi.DetailInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	// The verb as the gocode slot publishes it: its Go method, kebab-cased.
	var verb string
	for _, sm := range reflect.DiscoverMethods(stdreflect.TypeOf((*gcapi.GocodeService)(nil)).Elem()) {
		if sm.Name == "ModuleDashboard" {
			verb = reflect.ToKebab(sm.Name)
		}
	}
	want := "ktw-gocode:" + gcapi.GocodeServiceDef.Name + "/" + verb
	for _, p := range d.Panels {
		if p.ID == "tab:module-dashboard" {
			if verb == "" || p.Kind != paneapi.PanelKindSource || p.Source != want || p.Content == nil || p.Content.Kind != "page" {
				t.Fatalf("panel: kind %q source %q (want %q) content %+v", p.Kind, p.Source, want, p.Content)
			}
			return
		}
	}
	t.Fatalf("no module-dashboard panel on a module with code: %+v", d.Panels)
}

// A module with no code packages has no dashboard: there is nothing to measure.
func TestNoModuleDashboardOnAModuleWithoutCode(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":            {Data: []byte("name: module\ntree: node\ncapabilities: [module, code, module-health]\n")},
		"capabilities/module-health.yaml":     {Data: []byte("name: module-health\n")},
		"capabilities/module.yaml":            {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/capabilities/module.yaml")},
		"capabilities/code.yaml":              {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/capabilities/code.yaml")},
		"data-types/cap-module.dsl.yaml":      {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/data-types/cap-module.dsl.yaml")},
		"object-panels/module-dashboard.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-panels/module-dashboard.yaml")},
	}), panelocal.Extension())
	cwmapp.Module(t, ctx, cwm, "docs")
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := pane.Detail(ctx, paneapi.DetailInput{ID: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range d.Panels {
		if p.ID == "tab:module-dashboard" {
			t.Fatalf("a module without code has a dashboard: %+v", p)
		}
	}
}
