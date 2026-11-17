// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	schemaast "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// readTypes answers the data-types of the grouped « Contenu lu » test: an
// application whose id field is its uid, an interface declaring its own
// table, an entity declaring none.
type readTypes struct{ schemaapi.SchemaService }

func (readTypes) GetDataTypeWithExtensions(_ context.Context, in schemaapi.GetDataTypeWithExtensionsInput) (schemaapi.GetDataTypeOutput, error) {
	str := func(label string) *schemaast.Type { return &schemaast.Type{Kind: schemaast.KindString, Label: label} }
	switch in.URI {
	case "ktw-apps:/data-type/app":
		root := &schemaast.Type{Kind: schemaast.KindObject, IDField: []string{"uid"}, Fields: map[string]*schemaast.Type{
			"uid": str("Uid"), "name": str("Nom"), "code": str("Code applicatif"),
		}}
		return schemaapi.GetDataTypeOutput{Schema: &schemaast.Schema{Name: "app", Root: "App", Types: map[string]*schemaast.Type{"App": root}}}, nil
	case "ktw-apps:/data-type/flow":
		root := &schemaast.Type{Kind: schemaast.KindObject, Fields: map[string]*schemaast.Type{
			"id": str("Id"), "displayName": str("Name"), "source": str("Source"), "target": str("Target"),
			"dataObjects": str("Data"), "description": str("Description"), "reference": str("Reference"),
			"lifecycle": {Kind: schemaast.KindObject, Fields: map[string]*schemaast.Type{"asString": str("Phase")}},
		}, Extensions: []schemaast.Extension{{Kind: "ui-table", Payload: map[string]any{"name": "default", "columns": []any{
			map[string]any{"id": "id"}, map[string]any{"id": "displayName"}, map[string]any{"id": "source"},
			map[string]any{"id": "target"}, map[string]any{"id": "dataObjects"}, map[string]any{"id": "lifecycle.asString", "label": "État LeanIX"},
		}}}}}
		return schemaapi.GetDataTypeOutput{Schema: &schemaast.Schema{Name: "flow", Root: "Flow", Types: map[string]*schemaast.Type{"Flow": root}}}, nil
	case "ktw-apps:/data-type/entity":
		root := &schemaast.Type{Kind: schemaast.KindObject, Fields: map[string]*schemaast.Type{
			"zone": str("Zone"), "name": str("Nom"), "id": str("Id"), "description": str("Description"),
			"tags": {Kind: schemaast.KindArray},
		}}
		return schemaapi.GetDataTypeOutput{Schema: &schemaast.Schema{Name: "entity", Root: "Entity", Types: map[string]*schemaast.Type{"Entity": root}}}, nil
	}
	return schemaapi.GetDataTypeOutput{}, schemaapi.ErrDataTypeNotFound
}

// : « Contenu lu » draws what a source read grouped by kind -
// the applications it picks first, their code slot beside them, then one
// table per kind its connector types by a data-type, « Interfaces (n) », its
// columns the data-type's own table (else its plain fields: title, the
// others; a technical id never one), each row opening the
// item; the head counts each kind and says
// when they were read. Nothing names a connector.
func TestContenuLuIsGroupedByKind(t *testing.T) {
	ctx, cwm, svc, _ := pickWorldExts(t, map[string]string{"map.yaml": `name: map
label: Carte
description: The applications of the map and their flows.
sync: ktw-apps:map/source-sync
search: ktw-apps:map/search
picker: {unit: application, layout: list}
items:
  application: ktw-apps:/data-type/app
  interface: ktw-apps:/data-type/flow
  entity: ktw-apps:/data-type/entity
kinds:
  application: {label: Application, slots: [{attr: code, label: Code applicatif}]}
  interface: {label: Interface}
`}, application.DeclareSimpleService[schemaapi.SchemaService](func(context.Context) (schemaapi.SchemaService, error) {
		return readTypes{}, nil
	}))
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.SubTabsCapability}); err != nil {
		t.Fatal(err)
	}
	src := createSource(t, ctx, cwm, "map", map[string]any{"connector": "map"})
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "m"})
	if err != nil || res.ProjectRoot == "" {
		t.Fatalf("the project root: %v %+v", err, res)
	}
	read := func(local, name, kind, data string) {
		t.Helper()
		file := filepath.Join("input", local+".data.json")
		if err := os.MkdirAll(filepath.Join(res.ProjectRoot, "input"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(res.ProjectRoot, file), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m." + local, SourceURI: "map://" + local, NewContent: "x", Parent: src,
			Name: name, ItemKind: kind, DataFile: filepath.ToSlash(file)}); err != nil {
			t.Fatal(err)
		}
	}
	read("alpha", "Alpha", "application", `{"code":"RDY"}`)
	read("beta", "Beta", "application", `{"code":"WCT"}`)
	read("if-1", "Alpha → Beta", "interface", `{"id":"IF-1","displayName":"Commandes","source":"Alpha","target":"Beta","dataObjects":"Commande","lifecycle":{"asString":"active"},"description":"long"}`)
	read("if-2", "Beta → Alpha", "interface", `{"id":"IF-2","displayName":"Clients &amp; contacts","source":"Beta","target":"Alpha"}`)
	read("ent-1", "Magasin", "entity", `{"id":"E-1","name":"Magasin","zone":"Nord","tags":["a"]}`)

	out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: src, Tab: api.SourceTabRead})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(out.DSLTree); err != nil {
		t.Errorf("the grouped read tab is no valid page: %v", err)
	}
	raw, _ := json.Marshal(out.DSLTree)
	tree := string(raw)
	apps, ifs, ents := strings.Index(tree, `"Applications (2)"`), strings.Index(tree, `"Interfaces (2)"`), strings.Index(tree, `"Entitys (1)"`)
	if apps < 0 || ents < apps || ifs < ents {
		t.Errorf("the applications first, then a table per typed kind, by kind: %s", tree)
	}
	for _, want := range []string{`"id":"source-detail-items"`, `"dataField":"$.detail.read_0"`, `"id":"x_code"`, `"label":"Code applicatif"`,
		`"id":"source-read-interface"`, `"dataField":"$.detail.read_2"`, `"id":"x_displayName"`, `"id":"x_dataObjects"`,
		`"id":"x_lifecycle_asString"`, `"label":"État LeanIX"`, `"label":"Name"`, `"navigate":{"object":"${item.doc}"}`,
		`"id":"source-read-entity"`, `"dataField":"$.detail.read_1"`} {
		if !strings.Contains(tree, want) {
			t.Errorf("the grouped read tab lacks %s: %s", want, tree)
		}
	}
	if strings.Contains(tree, `"id":"x_description"`) {
		t.Errorf("a description is no column: %s", tree)
	}
	// A data-type with no table of its own: its uid leading
	// its title, then its other plain fields by name.
	ent := tree[strings.Index(tree, `"id":"source-read-entity"`):strings.Index(tree, `"Interfaces (2)"`)]
	if i, n, z := strings.Index(ent, `"id":"x_id"`), strings.Index(ent, `"id":"x_name"`), strings.Index(ent, `"id":"x_zone"`); i < 0 || n < i || z < n || strings.Contains(ent, `"id":"x_tags"`) {
		t.Errorf("the entity's columns are its uid, name, zone: %s", ent)
	}

	detail := out.Data["detail"].(map[string]any)
	if head, _ := detail["read_head"].(string); !strings.HasPrefix(head, "2 applications, 1 entitys, 2 interfaces, lues le ") {
		t.Errorf("the head counts each kind and says when: %q", head)
	}
	first := detail["read_0"].([]any)
	if len(first) != 2 || first[0].(map[string]any)["x_code"] != "RDY" {
		t.Errorf("the applications with their code: %v", first)
	}
	rows := detail["read_2"].([]any)
	if len(rows) != 2 {
		t.Fatalf("two interfaces: %v", rows)
	}
	var cmd, cli map[string]any
	for _, r := range rows {
		switch r.(map[string]any)["x_displayName"] {
		case "Commandes":
			cmd = r.(map[string]any)
		case "Clients & contacts":
			cli = r.(map[string]any)
		}
	}
	if cmd == nil || cmd["x_source"] != "Alpha" || cmd["x_target"] != "Beta" || cmd["x_dataObjects"] != "Commande" ||
		cmd["x_lifecycle_asString"] != "active" || cmd["doc"] != "m.if-1" || cmd["state_label"] != "synchronisé" {
		t.Errorf("an interface's row from its data, opening its document: %v", cmd)
	}
	if cli == nil || cli["x_displayName"] != "Clients & contacts" {
		t.Errorf("a cell decoded: %v", cli)
	}

	// A source bringing its unit alone keeps the one table.
	plain := createSource(t, ctx, cwm, "only-apps", map[string]any{"connector": "map"})
	if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m.solo", SourceURI: "map://solo", NewContent: "x", Parent: plain,
		Name: "Solo", ItemKind: "application"}); err != nil {
		t.Fatal(err)
	}
	solo, err := svc.SourceTab(ctx, api.SourceTabInput{ID: plain, Tab: api.SourceTabRead})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(solo.DSLTree)
	if strings.Contains(string(raw), "source-read-head") || !strings.Contains(string(raw), `"dataField":"$.detail.items"`) {
		t.Errorf("one kind: the one table: %s", raw)
	}
}
