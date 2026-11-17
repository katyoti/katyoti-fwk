// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A source's « Informations » and « Actions » tabs are
// technical - below the Expert level the strip shows only what the source is
// about (its take tab, « Contenu lu », « Réglages »), as mock-up v6's source
// view; both wait behind « Plus… », which the GUI opens at Expert
// (objectTabMore.splitStrip: Mode expert reveals the More tabs). The pane
// flags them More from the information-source type's `pane.more`; the other
// generic tabs stay behind « Plus… » as before, none hidden.
func TestASourcesInformationAndActionsTabsWaitBehindMore(t *testing.T) {
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(),
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":         {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml":      {Data: []byte("name: lifecycle\n")},
			"object-types/study-v2.yaml":       {Data: []byte("name: study-v2\ntree: node\ncapabilities: [one-source-per-connector, source-sub-tabs]\n")},
			"object-panels/information.yaml":   {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-panels/information.yaml")},
			"object-panels/actions.yaml":       {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-panels/actions.yaml")},
			"object-panels/lifecycle.yaml":     {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-panels/lifecycle.yaml")},
			"object-panels/files.yaml":         {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-panels/files.yaml")},
			"object-panels/challenged-by.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-panels/challenged-by.yaml")},
			"object-types/folder.yaml":         {Data: []byte("name: folder\ntree: node\n")},
			"edge-types/attaches.yaml":         {Data: []byte("name: attaches\n" + anyEdge)},
			"edge-types/challenges.yaml":       {Data: []byte("name: challenges\n" + anyEdge)},
		}),
		sourceslocal.Extension(),
		panelocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(ctx context.Context) (actionsapi.ActionService, error) {
			return functionRouting{actionsRouting{ctx}}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	holder := cwmapi.NewContainerObject("m", "ventes", "study-v2", "m", "ventes", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: holder}); err != nil {
		t.Fatal(err)
	}
	src := cwmapi.NewContainerObject("m", "ventes-folder", api.InformationSourceType, holder.ID, "ventes-folder", "")
	src.Fields = map[string]any{"connector": api.FolderConnector}
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: src})
	if err != nil {
		t.Fatal(err)
	}
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	det, err := pane.Detail(ctx, paneapi.DetailInput{ID: made.Object.ID})
	if err != nil {
		t.Fatal(err)
	}
	var main, more []string
	for _, p := range det.Panels {
		switch {
		case p.Section || p.Header:
		case p.More:
			more = append(more, p.ID)
		default:
			main = append(main, p.ID)
		}
	}
	// Below Expert: the strip's main tabs alone.
	wantMain := []string{"tab:information-source-" + api.SourceTabTake, "tab:information-source-" + api.SourceTabRead, "tab:information-source-" + api.SourceTabSettings}
	if !slices.Equal(main, wantMain) {
		t.Errorf("below Expert a source shows %v, want %v (never Informations or Actions)", main, wantMain)
	}
	// At Expert the More tabs come back: Informations and Actions first, the
	// other generic tabs still there.
	for _, id := range []string{"tab:information", "tab:actions", "tab:lifecycle"} {
		if !slices.Contains(more, id) {
			t.Errorf("%s waits behind « Plus… », shown at Expert: more = %v", id, more)
		}
	}
	if len(more) < 2 || more[0] != "tab:information" || more[1] != "tab:actions" {
		t.Errorf("Informations then Actions lead the tabs shown at Expert: %v", more)
	}

	// A tab neither list names is hidden: under a holder without the
	// sub-tabs, a source attaching a file and challenged by an object still
	// shows every tab it can have - « Source » then « Choisir les éléments »
	// (where the generic Sources list opens it) in the strip, its files and
	// its challengers behind « Plus… ».
	plain := cwmapi.NewContainerObject("m", "dossier", "folder", "m", "dossier", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: plain}); err != nil {
		t.Fatal(err)
	}
	other := cwmapi.NewContainerObject("m", "dossier-folder", api.InformationSourceType, plain.ID, "dossier-folder", "")
	other.Fields = map[string]any{"connector": api.FolderConnector}
	other.Edges = []cwmapi.ObjectEdge{{Kind: "attaches", To: plain.ID}}
	madeOther, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: other})
	if err != nil {
		t.Fatal(err)
	}
	challenger := cwmapi.NewContainerObject("m", "doute", "folder", "m", "doute", "")
	challenger.Edges = []cwmapi.ObjectEdge{{Kind: "challenges", To: madeOther.Object.ID}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: challenger}); err != nil {
		t.Fatal(err)
	}
	det, err = pane.Detail(ctx, paneapi.DetailInput{ID: madeOther.Object.ID})
	if err != nil {
		t.Fatal(err)
	}
	main, more = nil, nil
	for _, p := range det.Panels {
		switch {
		case p.Section || p.Header:
		case p.More:
			more = append(more, p.ID)
		default:
			main = append(main, p.ID)
		}
	}
	if want := []string{"tab:information-source", "tab:source-pick"}; !slices.Equal(main, want) {
		t.Errorf("under a plain holder a source shows %v, want %v (never a hidden « Choisir les éléments »)", main, want)
	}
	for _, id := range []string{"tab:information", "tab:actions", "tab:lifecycle", "tab:files", "tab:challenged-by"} {
		if !slices.Contains(more, id) {
			t.Errorf("%s is a tab this source has, hidden by its pane lists: main = %v, more = %v", id, main, more)
		}
	}
}

// anyEdge is a test edge kind between any two objects.
const anyEdge = "from: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n"

// functionRouting also answers the knowledge capability's functions a
// pane's filters and names evaluate, as the bus does in a binary.
type functionRouting struct{ actionsRouting }

func (f functionRouting) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	var call cwmapi.FunctionCall
	var got any
	switch {
	case strings.HasSuffix(in.URI, "sources/source-sub-tabs-function"):
		if err := json.Unmarshal(in.Input, &call); err != nil {
			return actionsapi.RunOutput{}, err
		}
		got, err = svc.SourceSubTabsFunction(ctx, call)
	case strings.HasSuffix(in.URI, "sources/source-take-function"):
		if err := json.Unmarshal(in.Input, &call); err != nil {
			return actionsapi.RunOutput{}, err
		}
		got, err = svc.SourceTakeFunction(ctx, call)
	default:
		return f.actionsRouting.Run(ctx, in)
	}
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(got)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// Describe says the functions only ask, as their methods declare.
func (f functionRouting) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: strings.HasSuffix(in.URI, "-function")}, nil
}
