// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	schemasvc "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// PANEL RESOLUTION, on a real CWM and object pane: what a panel draws is made
// self-contained before any host renders it — a schema named by data-type URI
// read from the schema authority with the ui-<kind> variant it names, a form
// over an action's input read from ActionService.Describe, a ui block laid
// over it; one fetch per distinct schema; a schema that cannot be read names
// itself.

// describer answers the input of the actions it knows as an AST.
type describer struct{ inputs map[string]string }

func (d describer) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, InputSchemaAST: json.RawMessage(d.inputs[in.URI])}, nil
}

func (describer) Run(context.Context, actionsapi.RunInput) (actionsapi.RunOutput, error) {
	return actionsapi.RunOutput{}, nil
}

// schemas is the schema authority: one data-type, its fetches counted.
type schemas struct {
	schemasvc.SchemaService
	mu    sync.Mutex
	asked []schemasvc.GetDataTypeWithExtensionsInput
}

func (s *schemas) GetDataTypeWithExtensions(_ context.Context, in schemasvc.GetDataTypeWithExtensionsInput) (schemasvc.GetDataTypeOutput, error) {
	s.mu.Lock()
	s.asked = append(s.asked, in)
	s.mu.Unlock()
	if in.URI != "ktw-demo:/data-type/book" {
		return schemasvc.GetDataTypeOutput{}, schemasvc.ErrDataTypeNotFound
	}
	var book coreschemaapi.Schema
	_ = json.Unmarshal([]byte(`{"name":"book","root":"Book","types":{"Book":{"type":"object","fields":{"title":{"type":"string"}}}}}`), &book)
	return schemasvc.GetDataTypeOutput{Schema: &book}, nil
}

func TestResolvePanelMakesThePageSelfContained(t *testing.T) {
	sch := &schemas{}
	acts := describer{inputs: map[string]string{
		"ktw-agents:planification/file-request": `{"name":"in","root":"FileRequestInput","types":{"FileRequestInput":{"type":"object","fields":{"title":{"type":"string"},"lines":{"type":"array","items":{"type":"object","fields":{"label":{"type":"string"}}}}}}}}`,
	}}
	ctx, _, svc := paneWorld(t, nil,
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
		application.DeclareSimpleService[schemasvc.SchemaService](func(context.Context) (schemasvc.SchemaService, error) { return sch, nil }),
	)
	def := render.Def{Kind: "page", DSLTree: render.VBox(
		render.Node{"kind": "view", "schema": "ktw-demo:/data-type/book", "data": map[string]any{"title": "Dune"}},
		render.Node{"kind": "table", "schema": "ktw-demo:/data-type/book", "ui": map[string]any{"name": "compact", "columns": []any{"title"}}},
		render.List(nil, render.Node{"kind": "view", "schema": "ktw-demo:/data-type/book"}, false),
		render.Node{"kind": "form", "schema": map[string]any{"action": "ktw-agents:planification/file-request"}},
	)}
	out, err := svc.ResolvePanel(ctx, api.ResolvePanelInput{Def: def})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out.Def)
	s := string(b)
	if strings.Contains(s, `"schema":"ktw-demo`) || strings.Contains(s, `"action":"ktw-agents:planification`) {
		t.Errorf("a schema reference survived: %s", s)
	}
	if !strings.Contains(s, `"root":"FileRequestInput"`) || !strings.Contains(s, `"lines"`) {
		t.Errorf("the form carries the action's input AST, nested fields included: %s", s)
	}
	if !strings.Contains(s, `"variant":"compact"`) || strings.Contains(s, `"ui":{`) {
		t.Errorf("the ui block is laid over as the compact variant: %s", s)
	}
	if !strings.Contains(s, `"title":"Dune"`) {
		t.Errorf("data passes through: %s", s)
	}
	// view default (twice: one fetch) and table default: two distinct fetches.
	if len(sch.asked) != 2 || sch.asked[0].Extensions[0].Name != "*" {
		t.Errorf("fetches = %+v, want one per distinct (uri, ui-kind, variant)", sch.asked)
	}

	// A schema that cannot be read fails the display, naming it.
	_, err = svc.ResolvePanel(ctx, api.ResolvePanelInput{Def: render.Def{Kind: "view", Schema: "ktw-demo:/data-type/nope"}})
	if err == nil || !strings.Contains(err.Error(), "ktw-demo:/data-type/nope") {
		t.Errorf("an unknown data-type: %v", err)
	}
	// A display with no schema-bound component comes back as it went.
	plain, err := svc.ResolvePanel(ctx, api.ResolvePanelInput{Def: render.Def{Kind: "markdown", Data: "# hi"}})
	if err != nil || plain.Def.(map[string]any)["data"] != "# hi" {
		t.Errorf("a markdown def: %+v, %v", plain.Def, err)
	}
}

// A form's field drawing its choices from the project's objects (`options_from:
// {source: objects}`) gets them from the real CWM when the panel is resolved,
// filtered by the ObjectFilter it declares.
func TestResolvePanelComputesObjectOptions(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml": "name: thing\ntree: node\n",
		"object-types/other.yaml": "name: other\ntree: node\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.a", Type: "thing", Name: "Alpha"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.b", Type: "thing", Name: "Beta"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.c", Type: "other", Name: "Gamma"})
	form := render.Node{"kind": "form", "schema": map[string]any{
		"name": "pick", "root": "In", "types": map[string]any{"In": map[string]any{"type": "object", "fields": map[string]any{
			"thing": map[string]any{"type": "string", "content": "text", "contentParams": map[string]any{
				"options_from": map[string]any{"source": "objects", "types": []any{"thing"}, "label": "name"},
			}},
		}}},
	}}
	out, err := svc.ResolvePanel(ctx, api.ResolvePanelInput{Def: render.Def{Kind: "page", DSLTree: render.VBox(form)}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out.Def)
	s := string(b)
	if !strings.Contains(s, `"enum":["m.a","m.b"]`) || !strings.Contains(s, `"label":"Alpha"`) || strings.Contains(s, "m.c") {
		t.Errorf("the thing field's options are the things, labelled by name: %s", s)
	}
}
