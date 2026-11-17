// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/reflect"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/fileoverview/api"
	folocal "github.com/katyoti/katyoti-fwk/internal/extensions/fileoverview/local"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE FILE-OVERVIEW SOURCE, on a real CWM with the capability AS SHIPPED: an
// object with no overview yet, then its markdown overview, then its page —
// which wins — each read back from the files CWM wrote.

func setup(t *testing.T) (context.Context, cwmapi.CWMService, api.FileOverviewService, paneapi.ObjectPaneService) {
	t.Helper()
	pack := cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":         {Data: []byte("name: module\ntree: node\n")},
		"object-types/dossier.yaml":        {Data: []byte("name: dossier\nfolder: dossiers\ncapabilities: [file-overview]\n")},
		"capabilities/file-overview.yaml":  {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/capabilities/file-overview.yaml")},
		"object-panels/file-overview.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-panels/file-overview.yaml")},
	})
	ctx, cwm := cwmapp.New(t, pack, folocal.Extension(), panelocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.FileOverviewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc, pane
}

func write(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id, name, content string) {
	t.Helper()
	if _, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{ID: id, Capability: api.Capability, Name: name, Content: []byte(content)}); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestReadShowsThePageElseTheOverviewElseNone(t *testing.T) {
	ctx, cwm, svc, _ := setup(t)
	d := cwmapi.NewContainerObject("m", "d1", "dossier", "m", "D1", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: d}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Read(ctx, api.ReadInput{ID: d.ID})
	if err != nil || got.Exists {
		t.Fatalf("no overview yet: %+v %v", got, err)
	}

	write(t, ctx, cwm, d.ID, "overview", "# Overview\n")
	got, err = svc.Read(ctx, api.ReadInput{ID: d.ID})
	if err != nil || !got.Exists || got.Name != "overview" || got.Content != "# Overview\n" || got.Path != "overview/m.d1.md" {
		t.Fatalf("the markdown overview: %+v %v", got, err)
	}

	write(t, ctx, cwm, d.ID, "page", `{"kind":"page"}`)
	got, err = svc.Read(ctx, api.ReadInput{ID: d.ID})
	if err != nil || got.Name != "page" || got.Content != `{"kind":"page"}` || got.Media != "application/json" {
		t.Fatalf("the page wins: %+v %v", got, err)
	}

	if _, err := svc.Read(ctx, api.ReadInput{}); err == nil {
		t.Error("an empty id must be refused")
	}
}

// The panel's source answers the display itself: a page saying there is none
// yet, the markdown overview as a markdown document, then the page file —
// bare tree or component — as a page; a page file that cannot be shown says
// why on the page instead of failing the panel.
func TestPageAnswersTheDisplayOfTheOverview(t *testing.T) {
	ctx, cwm, svc, _ := setup(t)
	d := cwmapi.NewContainerObject("m", "d1", "dossier", "m", "D1", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: d}); err != nil {
		t.Fatal(err)
	}
	page := func() map[string]any {
		t.Helper()
		got, err := svc.Page(ctx, api.ReadInput{ID: d.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := got.Validate(); err != nil {
			t.Fatalf("the answer must display: %v (%+v)", err, got)
		}
		b, _ := json.Marshal(got)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
	tree := func(m map[string]any) map[string]any { t.Helper(); n, _ := m["dslTree"].(map[string]any); return n }

	none := page()
	if none["kind"] != "page" || tree(none)["kind"] != "section" || !strings.Contains(fmt.Sprint(tree(none)["empty"]), "No overview yet") ||
		!strings.Contains(fmt.Sprint(tree(none)["empty"]), "overview/m.d1.md") || strings.Contains(fmt.Sprint(tree(none)["empty"]), "run the overview action") {
		t.Fatalf("no overview yet: %v", none)
	}

	write(t, ctx, cwm, d.ID, "overview", "# Overview\n")
	md := page()
	if md["kind"] != "markdown" || md["mediaType"] != "text/markdown" || md["data"] != "# Overview\n" || md["title"] != "overview/m.d1.md" {
		t.Fatalf("the markdown overview: %v", md)
	}

	write(t, ctx, cwm, d.ID, "page", `{"kind":"vbox","children":[{"kind":"heading","text":"Hi"}]}`)
	bare := page()
	if bare["kind"] != "page" || tree(bare)["kind"] != "vbox" {
		t.Fatalf("a bare tree is a page: %v", bare)
	}

	write(t, ctx, cwm, d.ID, "page", `{"kind":"page","dslTree":{"kind":"heading","text":"Hi"},"data":{"x":1}}`)
	comp := page()
	if comp["kind"] != "page" || tree(comp)["kind"] != "heading" || comp["data"] == nil {
		t.Fatalf("a component is shown as written: %v", comp)
	}

	write(t, ctx, cwm, d.ID, "page", `{"kind":"vbox","children":[{"kind":"nope"}]}`)
	bad := page()
	if tree(bad)["emptyColor"] != "red" || !strings.Contains(fmt.Sprint(tree(bad)["empty"]), "cannot be shown") {
		t.Fatalf("a page off the grammar says so: %v", bad)
	}
}

// The shipped panel is a source: object-pane/Detail returns it with this
// service's Page verb, no content (the source answers the whole display), and
// a refresh on the object itself.
func TestTheShippedPanelIsSourcedFromThisService(t *testing.T) {
	ctx, cwm, _, pane := setup(t)
	d := cwmapi.NewContainerObject("m", "d1", "dossier", "m", "D1", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: d}); err != nil {
		t.Fatal(err)
	}
	det, err := pane.Detail(ctx, paneapi.DetailInput{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := "ktw-agents:" + api.FileOverviewServiceDef.Name + "/" + reflect.ToKebab("Page")
	if _, ok := api.FileOverviewServiceDef.Methods["Page"]; !ok {
		t.Fatal("Page is no published verb")
	}
	for _, p := range det.Panels {
		if p.ID != "tab:file-overview" {
			continue
		}
		if p.Kind != paneapi.PanelKindSource || p.Source != want || p.Content != nil {
			t.Fatalf("panel: kind %q source %q (want %q) content %+v", p.Kind, p.Source, want, p.Content)
		}
		if p.Refresh == nil || p.Refresh.Scope != paneapi.RefreshSelf {
			t.Fatalf("panel refresh: %+v", p.Refresh)
		}
		return
	}
	t.Fatalf("no file-overview panel on a dossier: %+v", det.Panels)
}

// TestNamesAreTheDeclaredFiles keeps the order the service tries the named
// files in (api.Names, its one source) and the files the shipped capability
// declares in step: a file added, dropped or renamed on one side fails here.
func TestNamesAreTheDeclaredFiles(t *testing.T) {
	var decl struct {
		Files map[string]any `yaml:"files"`
	}
	if err := config.DecodeBytes(cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/capabilities/file-overview.yaml"), marshal.YAML, &decl); err != nil {
		t.Fatal(err)
	}
	if len(decl.Files) != len(api.Names) {
		t.Fatalf("the capability declares %d files, api.Names has %d: %v vs %v", len(decl.Files), len(api.Names), decl.Files, api.Names)
	}
	for _, n := range api.Names {
		if _, ok := decl.Files[n]; !ok {
			t.Errorf("api.Names has %q, which the capability does not declare", n)
		}
	}
}
