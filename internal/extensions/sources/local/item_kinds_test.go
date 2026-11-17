// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// maps is a connector whose applications bring data typed by its maps-app
// data-type and a text, and whose notes bring a text only.
const maps = `name: maps
label: Maps
description: The applications of the map and their notes.
sync: ktw-maps:maps/source-sync
fetch: ktw-maps:maps/fetch
items:
  app: ktw-maps:/data-type/maps-app
`

// mapsApp is the data-type of an application's data: its code required.
const mapsApp = `$schema: ktw-schema/v1
name: maps-app
version: "1.0.0"
root: MapsApp
types:
  MapsApp:
    type: object
    fields:
      code:
        type: string
      kind:
        type: string
        optional: true
`

// mapsActions serves the maps connector: a sync bringing ERP as data and
// text and a note as text only - the application's data wrong when bad is
// set, its data file named dataName (erp.data.json when empty) -, a fetch
// opening ERP, and sources' own save gate.
type mapsActions struct {
	bad      *bool
	dataName *string
}

func (mapsActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a mapsActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	var value any
	switch in.URI {
	case "ktw-maps:maps/source-sync":
		var sin api.ConnectorSyncInput
		_ = json.Unmarshal(in.Input, &sin)
		data := `{"code":"ERP","kind":"progiciel"}`
		if *a.bad {
			data = `{"kind":"progiciel"}`
		}
		dataName := "erp.data.json"
		if a.dataName != nil && *a.dataName != "" {
			dataName = *a.dataName
		}
		for name, body := range map[string]string{dataName: data, "erp.md": "# ERP\n\nThe ERP.\n", "note.md": "# Note\n\nA note.\n"} {
			if err := os.WriteFile(filepath.Join(sin.Dest, name), []byte(body), 0o644); err != nil {
				return actionsapi.RunOutput{}, err
			}
		}
		value = api.ConnectorSyncOutput{Items: []api.ConnectorItem{
			{Kind: "app", Data: dataName, Text: "erp.md", URI: "maps://erp", Title: "ERP", Key: "erp"},
			{Kind: "note", Text: "note.md", URI: "maps://note", Title: "Note", Key: "note"},
		}}
	case "ktw-maps:maps/fetch":
		var fin api.ConnectorFetchInput
		_ = json.Unmarshal(in.Input, &fin)
		value = api.ConnectorFetchOutput{Item: api.PickItem{Key: fin.Key, Title: "ERP", Kind: "app"},
			Data: map[string]any{"code": "ERP"}, Text: "# ERP\n\nThe ERP.\n"}
	case "ktw-agents:sources/source-check":
		svc, err := application.UseService[api.SourcesService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		var cin api.SourceCheckInput
		if err := json.Unmarshal(in.Input, &cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if value, err = svc.SourceCheck(ctx, cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// : an item a source brings is data typed by the data-type its
// connector declares for its kind, rich markdown text, or both. A sync
// anchors each with its kind, its data file and data-type, its text file;
// data that does not fit its data-type is not read, the source failing with
// why; fetch opens one item with its data, the data-type naming its shape,
// and its text.
func TestAnItemIsDataTextOrBoth(t *testing.T) {
	root := t.TempDir()
	bad, dataName := false, ""
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/maps.yaml": {Data: []byte(maps)}}, ".")
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
			return mapsActions{bad: &bad, dataName: &dataName}, nil
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
	if err != nil || !res.OK || res.Items != 2 {
		t.Fatalf("both items are read: %+v %v", res, err)
	}
	anchor := func(suffix string) map[string]any {
		t.Helper()
		for _, id := range res.Anchors {
			if strings.HasSuffix(id, suffix) {
				g, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
				if err != nil {
					t.Fatal(err)
				}
				var f map[string]any
				_ = json.Unmarshal(g.Object.FieldsJSON(), &f)
				return f
			}
		}
		t.Fatalf("no anchor %s in %v", suffix, res.Anchors)
		return nil
	}
	erp, note := anchor("-erp"), anchor("-note")
	folder := "modules/m/input/atlas"
	if erp["item_kind"] != "app" || erp["data_file"] != folder+"/erp.data.json" || erp["data_type"] != "ktw-maps:/data-type/maps-app" || erp["text_file"] != folder+"/erp.md" {
		t.Errorf("an item bringing data and text names both and the data's type: %v", erp)
	}
	if note["item_kind"] != "note" || note["text_file"] != folder+"/note.md" || note["data_file"] != nil || note["data_type"] != nil {
		t.Errorf("an item bringing text only names its text: %v", note)
	}
	for _, f := range []string{"erp.data.json", "erp.md", "note.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(folder), f)); err != nil {
			t.Errorf("a sync read whole keeps every file an item names: %s %v", f, err)
		}
	}

	// Data that does not fit its kind's data-type is not read.
	bad = true
	res, err = svc.SourceSync(ctx, api.SourceSyncInput{ID: src})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || len(res.Errors) == 0 || !strings.Contains(strings.Join(res.Errors, " "), "maps-app") {
		t.Errorf("data not fitting its data-type fails the sync, saying which: %+v", res)
	}
	bad = false

	// : an item's files follow the names the knowledge base
	// reads - its data <stem>.data.json beside its text <stem>.md. A data
	// file named otherwise, or not beside its text, is refused: the sync
	// fails, saying why.
	for name, want := range map[string]string{"erp.json": "<stem>.data.json", "erp-app.data.json": "not beside"} {
		dataName = name
		res, err = svc.SourceSync(ctx, api.SourceSyncInput{ID: src})
		if err != nil {
			t.Fatal(err)
		}
		if res.OK || !strings.Contains(strings.Join(res.Errors, " "), want) {
			t.Errorf("a data file named %s fails the sync, saying %q: %+v", name, want, res)
		}
	}
	dataName = ""
	if res, err = svc.SourceSync(ctx, api.SourceSyncInput{ID: src}); err != nil || !res.OK {
		t.Errorf("an item named as the knowledge base reads it syncs: %+v %v", res, err)
	}

	// Fetch opens one item: its roles, its data and the data-type shaping it, its text.
	got, err := svc.SourceFetch(ctx, api.SourceFetchInput{ID: src, Key: "erp"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Item.Key != "erp" || got.Data["code"] != "ERP" || got.DataType != "ktw-maps:/data-type/maps-app" || !strings.Contains(got.Text, "The ERP.") {
		t.Errorf("fetch opens the item with its data, its data-type and its text: %+v", got)
	}
	if _, err := svc.SourceFetch(ctx, api.SourceFetchInput{ID: src}); err == nil {
		t.Error("a fetch with no key is refused")
	}

	// : a source opens only an item it keeps - one a sync
	// brought under it, one picked or held -, any other key refused saying
	// why; the connector and location form a picker uses opens any key.
	_, err = svc.SourceFetch(ctx, api.SourceFetchInput{ID: src, Key: "ghost"})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), `"ghost"`) || !strings.Contains(err.Error(), "ne garde pas") {
		t.Errorf("a key the source does not keep is refused, saying which: %v", err)
	}
	if got, err := svc.SourceFetch(ctx, api.SourceFetchInput{Connector: "maps", Location: map[string]any{"map": "x"}, Key: "ghost"}); err != nil || got.Item.Key != "ghost" {
		t.Errorf("a picker's connector and location form opens any key: %+v %v", got, err)
	}
	// Under another module: a holder reads a connector through one source.
	cwmapp.Module(t, ctx, cwm, "m2")
	chosenObj := cwmapi.NewContainerObject("m2", "chosen", api.InformationSourceType, "m2", "chosen", "")
	chosenObj.Fields = map[string]any{"connector": "maps", "location": map[string]any{"map": "x"},
		"selection": map[string]any{api.SelectionItems: []any{map[string]any{"key": "crm"}}}, "held": []any{map[string]any{"key": "hr"}}}
	chosenOut, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m2", Object: chosenObj})
	if err != nil {
		t.Fatal(err)
	}
	chosen := chosenOut.Object.ID
	for _, key := range []string{"crm", "hr"} {
		if _, err := svc.SourceFetch(ctx, api.SourceFetchInput{ID: chosen, Key: key}); err != nil {
			t.Errorf("an item picked or held opens before any sync (%s): %v", key, err)
		}
	}
	if _, err := svc.SourceFetch(ctx, api.SourceFetchInput{ID: chosen, Key: "erp"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("another source's item is not this one's: %v", err)
	}
}
