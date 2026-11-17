// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : an information source's detail -
// sources/source-detail with the source's id, the region its « Contenu lu »
// sub-tab or its « Source » tab draws -
// names the connector by its declared label.
func TestAnInformationSourcesPaneHoldsItsDetail(t *testing.T) {
	bad, fetches, failFetch := false, 0, false
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/maps.yaml": {Data: []byte(maps)}}, ".")
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(),
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-maps", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return documentActions{maps: mapsActions{bad: &bad}, fetches: &fetches, failFetch: &failFetch}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	src := createSource(t, ctx, cwm, "atlas", map[string]any{"connector": "maps", "location": map[string]any{"map": "x"}})

	// What the box loads: the source's detail, its connector named by label.
	out, err := svc.SourceDetail(ctx, api.SourceDetailInput{ID: src})
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := out.Data["detail"].(map[string]any); d == nil || d["label"] != "Maps" {
		t.Errorf("the detail names the connector by its declared label: %+v", out.Data)
	}
	region, _ := json.Marshal(out.DSLTree)
	if !strings.Contains(string(region), `"dataField":"$.detail.label"`) {
		t.Errorf("the region draws the connector's label: %s", region)
	}
	if err := render.ValidatePage(out.DSLTree); err != nil {
		t.Errorf("the region is no valid page: %v", err)
	}
}

// : no view names a source by its connector's
// kebab name - the information source's header carries no badge over
// fields.connector (its pane's « Source » tab names the connector by its
// declared label); the header's badges are the type's `pane.badges` and its
// effective capabilities', as the object pane reads them.
func TestAnInformationSourcesHeaderNamesNoConnectorByItsName(t *testing.T) {
	bad := false
	fetches, failFetch := 0, false
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/maps.yaml": {Data: []byte(maps)}}, ".")
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(),
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-maps", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return documentActions{maps: mapsActions{bad: &bad}, fetches: &fetches, failFetch: &failFetch}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	src := createSource(t, ctx, cwm, "atlas", map[string]any{"connector": "maps", "location": map[string]any{"map": "x"}})

	got, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: api.InformationSourceType})
	if err != nil {
		t.Fatal(err)
	}
	ps, err := paneapi.PaneOf(got.ObjectType)
	if err != nil {
		t.Fatal(err)
	}
	surf, err := cwm.ObjectEffectiveSurface(ctx, cwmapi.ObjectEffectiveSurfaceInput{ID: src})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range append(append([]cwmapi.BadgeDef(nil), ps.Badges...), surf.Badges...) {
		if strings.TrimPrefix(d.Field, "fields.") == "connector" {
			t.Errorf("the header draws the connector by its kebab name (badge %+v); the « Source » tab names it by its declared label", d)
		}
	}
}
