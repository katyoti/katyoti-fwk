// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// kindTable is a kind's table on « Contenu lu »: its column ids in order,
// the first column's prepend (its open mark), its filters.
func kindTable(t *testing.T, tree map[string]any, kind string) (cols []string, firstPrepend any, filters []any) {
	t.Helper()
	raw, _ := json.Marshal(tree)
	var root any
	_ = json.Unmarshal(raw, &root)
	var found map[string]any
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["kind"] == "table" && v["id"] == "source-read-"+kind {
				found = v
			}
			for _, c := range v {
				walk(c)
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no table for %s: %s", kind, raw)
	}
	types := found["schema"].(map[string]any)["types"].(map[string]any)
	ext := types["ReadKindItem"].(map[string]any)["extensions"].([]any)[0].(map[string]any)
	for i, c := range ext["columns"].([]any) {
		col := c.(map[string]any)
		cols = append(cols, col["id"].(string))
		if i == 0 {
			firstPrepend = col["prepend"]
		}
	}
	filters, _ = ext["filters"].([]any)
	return cols, firstPrepend, filters
}

// : each kind « Contenu lu » draws as a table of its own has a
// « Colonne identifiant » row on the source's « Réglages » - an attribute of
// its data-type, or none -; the chosen attribute leads the kind's table, its
// open mark and its filter; no choice leads with the entity's uid, none
// draws no id column. Nothing names a connector.
func TestAKindsIdColumnIsChosenInTheSourcesSettings(t *testing.T) {
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
  application: {label: Application}
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
	read("if-1", "Alpha → Beta", "interface", `{"id":"7b4e-uuid","reference":"FL0001","displayName":"Commandes","source":"Alpha","target":"Beta"}`)
	read("ent-1", "Magasin", "entity", `{"id":"E-1","name":"Magasin","zone":"Nord"}`)
	readTab := func() (map[string]any, map[string]any) {
		t.Helper()
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: src, Tab: api.SourceTabRead})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("the read tab is no valid page: %v", err)
		}
		detail, _ := out.Data["detail"].(map[string]any)
		return out.DSLTree, detail
	}

	// No choice: the entity's uid leads, its open mark and filter.
	tree, _ := readTab()
	cols, open, filters := kindTable(t, tree, "interface")
	if len(cols) < 2 || cols[0] != "x_id" || cols[1] != "x_displayName" || open == nil || len(filters) != 1 || filters[0] != "x_id" {
		t.Errorf("no choice: led by the uid, its open mark and filter: %v %v %v", cols, open, filters)
	}

	// « Réglages »: a « Colonne identifiant » row per kind drawn as its own
	// table, its choices the data-type's fields, none and no choice.
	rows, order, value := settingsRows(t, ctx, svc, src)
	if !slices.Contains(order, "id_columns.interface") || !slices.Contains(order, "id_columns.entity") || !slices.Contains(order, "id_columns.application") {
		t.Fatalf("an id column row per typed kind, the unit included: %v", order)
	}
	row := rows["id_columns.interface"]
	if row["label"] != "Colonne identifiant" {
		t.Errorf("the row's label: %+v", row)
	}
	var values []string
	for _, o := range row["options"].([]any) {
		values = append(values, o.(map[string]any)["value"].(string))
	}
	if !slices.Contains(values, "reference") || !slices.Contains(values, api.IDColumnNone) || !slices.Contains(values, api.IDColumnUID) || slices.Contains(values, "") {
		t.Errorf("the choices are the fields, none and the uid: %v", values)
	}
	if value.IDColumns["interface"] != api.IDColumnUID {
		t.Errorf("no choice reads as the uid: %+v", value.IDColumns)
	}

	value.IDColumns = map[string]string{"interface": "reference", "entity": api.IDColumnNone, "unknown": "x"}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: src, Settings: value}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	_, _, value = settingsRows(t, ctx, svc, src)
	if value.IDColumns["interface"] != "reference" || value.IDColumns["entity"] != api.IDColumnNone || value.IDColumns["unknown"] != "" {
		t.Errorf("the choices kept for the typed kinds only: %+v", value.IDColumns)
	}
	tree, detail := readTab()
	cols, open, filters = kindTable(t, tree, "interface")
	if len(cols) == 0 || cols[0] != "x_reference" || open == nil || len(filters) != 1 || filters[0] != "x_reference" {
		t.Errorf("the chosen attribute leads, opens and filters: %v %v %v", cols, open, filters)
	}
	found := false
	for k, v := range detail {
		if list, ok := v.([]any); ok && len(k) > 5 && k[:5] == "read_" {
			for _, r := range list {
				if r.(map[string]any)["x_reference"] == "FL0001" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("the interface's row carries its reference: %v", detail)
	}
	ecols, _, _ := kindTable(t, tree, "entity")
	if len(ecols) == 0 || ecols[0] != "x_name" || slices.Contains(ecols, "x_id") {
		t.Errorf("none: no id column: %v", ecols)
	}

	// A choice cleared is back to the uid.
	value.IDColumns = map[string]string{"entity": "zone"}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: src, Settings: value}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	tree, _ = readTab()
	if ecols, _, _ = kindTable(t, tree, "entity"); len(ecols) == 0 || ecols[0] != "x_zone" {
		t.Errorf("a chosen attribute leads: %v", ecols)
	}
	if cols, _, _ = kindTable(t, tree, "interface"); len(cols) == 0 || cols[0] != "x_id" {
		t.Errorf("a choice cleared: back to the uid: %v", cols)
	}
}

// unitTable is the unit's table on « Contenu lu » (the source's items): its
// column ids in order, the open mark's column, its filters.
func unitTable(t *testing.T, tree map[string]any) (cols []string, open string, filters []any) {
	t.Helper()
	raw, _ := json.Marshal(tree)
	var root any
	_ = json.Unmarshal(raw, &root)
	var found map[string]any
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["kind"] == "table" && v["id"] == "source-detail-items" {
				found = v
			}
			for _, c := range v {
				walk(c)
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no unit table: %s", raw)
	}
	ext := found["schema"].(map[string]any)["types"].(map[string]any)["ReadItem"].(map[string]any)["extensions"].([]any)[0].(map[string]any)
	for _, c := range ext["columns"].([]any) {
		col := c.(map[string]any)
		cols = append(cols, col["id"].(string))
		if col["prepend"] != nil {
			open = col["id"].(string)
		}
	}
	filters, _ = ext["filters"].([]any)
	return cols, open, filters
}

// , the unit the picker picks (an application) as any kind:
// its « Colonne identifiant » row on « Réglages »; with no choice its table
// leads with the entity's uid (the data-type's id field), alone or beside
// the other kinds' tables; « Aucune » draws none; a chosen attribute leads;
// a default its carrier gave the source is kept and a person's choice wins.
func TestReadIDColumns(t *testing.T) {
	ctx, cwm, svc, _ := pickWorldExts(t, map[string]string{"map.yaml": `name: map
label: Carte
description: The applications of the map and their flows.
sync: ktw-apps:map/source-sync
search: ktw-apps:map/search
picker: {unit: application, layout: list}
items:
  application: ktw-apps:/data-type/app
  interface: ktw-apps:/data-type/flow
kinds:
  application: {label: Application, slots: [{attr: code, label: Code applicatif}]}
  interface: {label: Interface}
`}, application.DeclareSimpleService[schemaapi.SchemaService](func(context.Context) (schemaapi.SchemaService, error) {
		return readTypes{}, nil
	}))
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.SubTabsCapability}); err != nil {
		t.Fatal(err)
	}
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "m"})
	if err != nil || res.ProjectRoot == "" {
		t.Fatalf("the project root: %v %+v", err, res)
	}
	read := func(src, local, kind, data string) {
		t.Helper()
		file := filepath.Join("input", local+".data.json")
		if err := os.MkdirAll(filepath.Join(res.ProjectRoot, "input"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(res.ProjectRoot, file), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m." + local, SourceURI: "map://" + local, NewContent: "x", Parent: src,
			Name: local, ItemKind: kind, DataFile: filepath.ToSlash(file)}); err != nil {
			t.Fatal(err)
		}
	}
	readTab := func(src string) (map[string]any, map[string]any) {
		t.Helper()
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: src, Tab: api.SourceTabRead})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("the read tab is no valid page: %v", err)
		}
		detail, _ := out.Data["detail"].(map[string]any)
		return out.DSLTree, detail
	}
	cell := func(detail map[string]any, key, col string) any {
		rows, _ := detail[key].([]any)
		if len(rows) == 0 {
			return nil
		}
		return rows[0].(map[string]any)[col]
	}

	// Beside the other kinds: the unit's table leads with its uid.
	src := createSource(t, ctx, cwm, "map", map[string]any{"connector": "map"})
	read(src, "alpha", "application", `{"uid":"a-7f3","name":"Alpha","code":"RDY"}`)
	read(src, "if-1", "interface", `{"id":"7b4e-uuid","reference":"FL0001","displayName":"Commandes"}`)
	tree, detail := readTab(src)
	cols, open, filters := unitTable(t, tree)
	if len(cols) < 2 || cols[0] != "x_uid" || cols[1] != "title" || open != "x_uid" || len(filters) == 0 || filters[0] != "x_uid" || !slices.Contains(cols, "x_code") {
		t.Errorf("no choice: the unit's uid leads, opens and filters, its slots kept: %v %q %v", cols, open, filters)
	}
	if cell(detail, "read_0", "x_uid") != "a-7f3" {
		t.Errorf("the unit's row carries its uid: %v", detail["read_0"])
	}
	if c, _, _ := kindTable(t, tree, "interface"); len(c) == 0 || c[0] != "x_id" {
		t.Errorf("no choice: the interface's uid leads: %v", c)
	}

	// « Réglages »: the unit's row, its choices the data-type's fields.
	rows, order, value := settingsRows(t, ctx, svc, src)
	if !slices.Contains(order, "id_columns.application") {
		t.Fatalf("the unit has its id column row: %v", order)
	}
	var values, labels []string
	for _, o := range rows["id_columns.application"]["options"].([]any) {
		values = append(values, o.(map[string]any)["value"].(string))
		labels = append(labels, o.(map[string]any)["label"].(string))
	}
	if !slices.Contains(values, "code") || !slices.Contains(values, api.IDColumnNone) || !slices.Contains(labels, "Par défaut (identifiant)") {
		t.Errorf("the unit's choices: %v %v", values, labels)
	}

	// « Aucune » draws none; a chosen attribute leads.
	value.IDColumns = map[string]string{"application": api.IDColumnNone}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: src, Settings: value}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	tree, _ = readTab(src)
	if cols, open, _ = unitTable(t, tree); cols[0] != "title" || open != "title" || slices.Contains(cols, "x_uid") {
		t.Errorf("none: no id column on the unit's table: %v %q", cols, open)
	}
	value.IDColumns = map[string]string{"application": "code"}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: src, Settings: value}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	_, _, value = settingsRows(t, ctx, svc, src)
	if value.IDColumns["application"] != "code" {
		t.Errorf("the unit's choice kept: %+v", value.IDColumns)
	}
	tree, _ = readTab(src)
	if cols, _, _ = unitTable(t, tree); cols[0] != "x_code" {
		t.Errorf("the chosen attribute leads the unit's table: %v", cols)
	}
	// The slot on the chosen attribute is not drawn again: no column id repeats.
	once := map[string]bool{}
	for _, c := range cols {
		if once[c] {
			t.Errorf("the unit's table draws %q twice: %v", c, cols)
		}
		once[c] = true
	}

	// The unit alone keeps its one table, led by its uid.
	solo := createSource(t, ctx, cwm, "solo", map[string]any{"connector": "map"})
	read(solo, "beta", "application", `{"uid":"b-19c","name":"Beta"}`)
	tree, detail = readTab(solo)
	if cols, open, _ = unitTable(t, tree); cols[0] != "x_uid" || open != "x_uid" {
		t.Errorf("the unit alone: led by its uid: %v %q", cols, open)
	}
	if cell(detail, "items", "x_uid") != "b-19c" {
		t.Errorf("the lone unit's row carries its uid: %v", detail["items"])
	}

	// A default its carrier gave the source is kept; a person's choice wins.
	carried := createSource(t, ctx, cwm, "carried", map[string]any{"connector": "map", "id_columns": map[string]any{"application": "name"}})
	read(carried, "gamma", "application", `{"uid":"g-2","name":"Gamma"}`)
	tree, _ = readTab(carried)
	if cols, _, _ = unitTable(t, tree); cols[0] != "x_name" {
		t.Errorf("the carrier's default leads: %v", cols)
	}
	_, _, value = settingsRows(t, ctx, svc, carried)
	if value.IDColumns["application"] != "name" {
		t.Errorf("the carrier's default shown in « Réglages »: %+v", value.IDColumns)
	}
	value.IDColumns = map[string]string{"application": "uid"}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: carried, Settings: value}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	tree, _ = readTab(carried)
	if cols, _, _ = unitTable(t, tree); cols[0] != "x_uid" {
		t.Errorf("a person's choice wins over the carrier's: %v", cols)
	}

	// « Par défaut (identifiant) » picked over a carrier's default is kept
	// as a choice of its own: a carrier filling only the kinds its sources
	// do not name leaves it, and the uid still leads.
	chosen := createSource(t, ctx, cwm, "chosen", map[string]any{"connector": "map", "id_columns": map[string]any{"application": "name"}})
	read(chosen, "delta", "application", `{"uid":"d-4","name":"Delta"}`)
	_, _, value = settingsRows(t, ctx, svc, chosen)
	value.IDColumns = map[string]string{"application": api.IDColumnUID}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: chosen, Settings: value}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: chosen})
	if err != nil {
		t.Fatal(err)
	}
	var stored api.InformationSourceFields
	if err := json.Unmarshal(got.Object.FieldsJSON(), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.IDColumns["application"] != api.IDColumnUID {
		t.Errorf("the uid picked is stored as a choice, not dropped: %+v", stored.IDColumns)
	}
	_, _, value = settingsRows(t, ctx, svc, chosen)
	if value.IDColumns["application"] != api.IDColumnUID {
		t.Errorf("the uid picked reads back as « Par défaut (identifiant) »: %+v", value.IDColumns)
	}
	tree, _ = readTab(chosen)
	if cols, _, _ = unitTable(t, tree); cols[0] != "x_uid" {
		t.Errorf("the uid picked leads the unit's table: %v", cols)
	}

	// A kind with no choice sent back as the uid (the form sends every row)
	// stays without a choice, for a carrier to fill.
	plain := createSource(t, ctx, cwm, "plain", map[string]any{"connector": "map"})
	_, _, value = settingsRows(t, ctx, svc, plain)
	if value.IDColumns["application"] != api.IDColumnUID {
		t.Errorf("no choice reads as the uid: %+v", value.IDColumns)
	}
	if got, err := svc.SettingsSave(ctx, api.SettingsSaveInput{ID: plain, Settings: value}); err != nil || !got.OK {
		t.Fatalf("save: %v %+v", err, got)
	}
	if got, err = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: plain}); err != nil {
		t.Fatal(err)
	}
	stored = api.InformationSourceFields{}
	if err := json.Unmarshal(got.Object.FieldsJSON(), &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.IDColumns) != 0 {
		t.Errorf("an untouched row writes no choice: %+v", stored.IDColumns)
	}
}
