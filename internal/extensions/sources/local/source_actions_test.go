// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	actionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/actions/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A source's « Actions » are what can be done with the source itself: read it
// (again) and test its connection, each bound to that source - and a task
// that applies to another type (a deck for a study) is not among them.
func TestASourceOffersItsConnectorsVerbs(t *testing.T) {
	tasks := func(context.Context) []actionsapi.TaskAction {
		return []actionsapi.TaskAction{
			{Provider: "ktw-example", Name: "deck-new", Label: "Deck", Kind: cwmapi.ActionScopeObject, Types: []string{"study-v2", "module"}},
		}
	}
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(),
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		actionslocal.SurfaceExtensionWith(tasks),
		application.DeclareSimpleService[actionsapi.ActionService](func(ctx context.Context) (actionsapi.ActionService, error) {
			return actionsRouting{ctx}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	obj := cwmapi.NewContainerObject("m", "specs", api.InformationSourceType, "m", "specs", "")
	raw, _ := json.Marshal(map[string]any{"connector": api.FolderConnector, "location": map[string]any{"path": "ext"}})
	obj.SetFieldsJSON(raw)
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Object.ID

	surface, err := application.UseService[actionsapi.SurfaceService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := surface.List(ctx, actionsapi.ListInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]cwmapi.Action{}
	for _, a := range out.Actions {
		got[a.URI] = a
	}
	for uri, label := range map[string]string{
		"ktw-agents:sources/source-sync": "Read / update",
		"ktw-agents:sources/source-test": "Test the connection",
	} {
		a, ok := got[uri]
		if !ok || a.Label != label || a.Disabled {
			t.Errorf("the source does not offer %q (%s): %+v", label, uri, a)
			continue
		}
		if a.BoundInput["id"] != id {
			t.Errorf("%s is not bound to the source: %+v", uri, a.BoundInput)
		}
	}
	if _, ok := got["ktw-example:tasks/deck-new"]; ok {
		t.Errorf("a task typed for other types is offered on a source: %+v", out.Actions)
	}

	// « Tester la connexion » checks the source's own connector, from its id.
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.SourceTest(ctx, api.SourceTestInput{ID: id})
	if err != nil || !strings.Contains(res.Message, api.FolderConnector) {
		t.Errorf("SourceTest by id does not ask the source's connector: %v %+v", err, res)
	}
	if _, err := svc.SourceTest(ctx, api.SourceTestInput{ID: "m"}); err == nil {
		t.Errorf("SourceTest on an object that is no source must be refused")
	}
}
