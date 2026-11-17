// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// documentActions serves the maps connector (mapsActions), counting its
// fetches and failing them when told.
type documentActions struct {
	maps      mapsActions
	fetches   *int
	failFetch *bool
}

func (a documentActions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a documentActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	switch {
	case in.URI == "ktw-maps:maps/fetch":
		*a.fetches++
		if *a.failFetch {
			return actionsapi.RunOutput{}, errors.New("maps unreachable")
		}
	}
	return a.maps.Run(ctx, in)
}

// : sources/document-detail opens the item a
// source-document records - its pane's page (sources/document-page) is
// built on it. A document a sync brought under its source is
// opened through the source's fetch - its roles, its data and data-type, its
// text -, with the objects derived from it; an anchor with no item key
// answers what it stores, never fetching; a fetch that fails answers what is
// stored, saying so. The tab shows for every source-document.
func TestASourceDocumentOpensItsItemAndWhatDerivesFromIt(t *testing.T) {
	root := t.TempDir()
	bad, fetches, failFetch := false, 0, false
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/maps.yaml": {Data: []byte(maps)}}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-maps", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"object-types/decision.yaml":  {Data: []byte("name: decision\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		schemalocal.Extension(),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return documentActions{maps: mapsActions{bad: &bad}, fetches: &fetches, failFetch: &failFetch}, nil
		}),
	)
	schemas, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas.RegisterDataType(ctx, schemaapi.RegisterDataTypeInput{URI: "ktw-maps:/data-type/maps-app", Body: []byte(mapsApp)}); err != nil {
		t.Fatal(err)
	}
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	src := createSource(t, ctx, cwm, "atlas", map[string]any{"connector": "maps", "location": map[string]any{"map": "x"}})
	res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: src})
	if err != nil || !res.OK {
		t.Fatalf("sync: %+v %v", res, err)
	}
	var erp string
	for _, id := range res.Anchors {
		if strings.HasSuffix(id, "-erp") {
			erp = id
		}
	}
	if erp == "" {
		t.Fatalf("no ERP anchor in %v", res.Anchors)
	}
	create := func(local, name string, edges ...cwmapi.ObjectEdge) string {
		t.Helper()
		o := cwmapi.NewContainerObject("m", local, "decision", "m", name, "")
		o.Edges = edges
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o})
		if err != nil {
			t.Fatalf("create %s: %v", local, err)
		}
		return out.Object.ID
	}
	use1 := create("use-erp", "Use the ERP", cwmapi.ObjectEdge{Kind: api.DerivedFromEdge, To: erp})
	use2 := create("keep-erp", "Keep the ERP", cwmapi.ObjectEdge{Kind: api.DerivedFromEdge, To: erp})
	create("unrelated", "Unrelated")

	// A synced document: opened through its source's fetch, with what
	// derives from it.
	got, err := svc.DocumentDetail(ctx, api.DocumentDetailInput{ID: erp})
	if err != nil {
		t.Fatal(err)
	}
	if fetches != 1 || got.Item.Key != "erp" || got.Data["code"] != "ERP" || got.DataType != "ktw-maps:/data-type/maps-app" || !strings.Contains(got.Text, "The ERP.") || got.Note != "" {
		t.Errorf("a synced document opens through its source's fetch (fetches %d): %+v", fetches, got)
	}
	var used []string
	for _, u := range got.Used {
		if u.Type != "decision" || u.Name == "" {
			t.Errorf("a derived object carries its name and type: %+v", u)
		}
		used = append(used, u.ID)
	}
	sort.Strings(used)
	want := []string{use1, use2}
	sort.Strings(want)
	if strings.Join(used, ",") != strings.Join(want, ",") {
		t.Errorf("used lists the objects derived from it, and only them: %v, want %v", used, want)
	}

	// Its source cannot open it: what is stored, said.
	failFetch = true
	got, err = svc.DocumentDetail(ctx, api.DocumentDetailInput{ID: erp})
	failFetch = false
	if err != nil {
		t.Fatalf("a failing fetch is no error page: %v", err)
	}
	if got.Note == "" || !strings.Contains(got.Text, "The ERP.") || got.Data["code"] != "ERP" || got.DataType != "ktw-maps:/data-type/maps-app" || got.Item.Title != "ERP" || len(got.Used) != 2 {
		t.Errorf("a failing fetch answers what is stored, saying so: %+v", got)
	}

	// An anchor with no item key (sources/Ingest): its snapshot, no fetch.
	before := fetches
	if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m.spec", SourceURI: "https://example.com/spec", Name: "Spec", NewContent: "# Spec\n\nLimit 100 rps.\n"}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.DocumentDetail(ctx, api.DocumentDetailInput{ID: "m.spec"})
	if err != nil {
		t.Fatal(err)
	}
	if fetches != before || !strings.Contains(got.Text, "Limit 100 rps.") || got.Item.URI != "https://example.com/spec" || got.Item.Title != "Spec" || got.Used == nil || len(got.Used) != 0 {
		t.Errorf("an anchor with no key answers its snapshot, never fetching (fetches %d→%d): %+v", before, fetches, got)
	}

	// Only a source-document opens.
	if _, err := svc.DocumentDetail(ctx, api.DocumentDetailInput{ID: src}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("an information source is no source-document: %v", err)
	}
	if _, err := svc.DocumentDetail(ctx, api.DocumentDetailInput{}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("an id is required: %v", err)
	}

	// The tab: a source-document's, with no filter beyond its type, drawn by
	// the page document-page answers, built on this answer.
	raw, err := os.ReadFile("../contrib/embed/cwm/object-panels/source-document.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var panel struct {
		Filter struct {
			Types []string `json:"types"`
			When  string   `json:"when"`
		} `json:"filter"`
		Source string `json:"source"`
	}
	if err := config.DecodeBytes(raw, marshal.YAML, &panel); err != nil {
		t.Fatal(err)
	}
	if strings.Join(panel.Filter.Types, ",") != api.SourceDocumentType || panel.Filter.When != "" || panel.Source != api.DocumentPageURI {
		t.Errorf("the tab is every source-document's, from document-page: %+v", panel)
	}
	page, err := svc.DocumentPage(ctx, api.DocumentDetailInput{ID: erp})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(page.DSLTree); err != nil {
		t.Errorf("the tab's page: %v", err)
	}
}

// : a document under a source whose connector declares no
// fetch (a LeanIX application) opens from what its sync stored, with no note
// - nothing failed -, its source never asked to fetch.
func TestASourceDocumentWhoseConnectorDeclaresNoFetchOpensFromStored(t *testing.T) {
	root := t.TempDir()
	bad, fetches, failFetch := false, 0, false
	noFetch := strings.Replace(maps, "fetch: ktw-maps:maps/fetch\n", "", 1)
	if noFetch == maps {
		t.Fatal("the connector still declares a fetch")
	}
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/maps.yaml": {Data: []byte(noFetch)}}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-maps", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		schemalocal.Extension(),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return documentActions{maps: mapsActions{bad: &bad}, fetches: &fetches, failFetch: &failFetch}, nil
		}),
	)
	schemas, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas.RegisterDataType(ctx, schemaapi.RegisterDataTypeInput{URI: "ktw-maps:/data-type/maps-app", Body: []byte(mapsApp)}); err != nil {
		t.Fatal(err)
	}
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	src := createSource(t, ctx, cwm, "atlas", map[string]any{"connector": "maps", "location": map[string]any{"map": "x"}})
	res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: src})
	if err != nil || !res.OK {
		t.Fatalf("sync: %+v %v", res, err)
	}
	var erp string
	for _, id := range res.Anchors {
		if strings.HasSuffix(id, "-erp") {
			erp = id
		}
	}
	if erp == "" {
		t.Fatalf("no ERP anchor in %v", res.Anchors)
	}
	got, err := svc.DocumentDetail(ctx, api.DocumentDetailInput{ID: erp})
	if err != nil {
		t.Fatal(err)
	}
	if got.Note != "" {
		t.Errorf("a connector declaring no fetch is no failure: no note, got %q", got.Note)
	}
	if fetches != 0 || !strings.Contains(got.Text, "The ERP.") || got.Data["code"] != "ERP" || got.DataType != "ktw-maps:/data-type/maps-app" || got.Item.Title != "ERP" || got.Used == nil {
		t.Errorf("a document whose connector declares no fetch answers what is stored (fetches %d): %+v", fetches, got)
	}
}
