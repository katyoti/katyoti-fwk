// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

type orderLine struct {
	Label string  `json:"label"`
	Qty   float64 `json:"qty,omitempty"`
}

type placeInput struct {
	Title string      `json:"title" jsonschema:"description=What is ordered"`
	Lines []orderLine `json:"lines,omitempty"`
	Ship  struct {
		City string `json:"city"`
	} `json:"ship"`
}

type orders struct{}

func (orders) PlaceOrder(_ context.Context, in placeInput) (struct{}, error) { return struct{}{}, nil }

// Describe gives a method's input as the canonical AST too — read back from
// its JSON Schema — so a form renders its nested object and its list of rows.
func TestActionService_Describe_MethodInputAST(t *testing.T) {
	app := application.New(application.Config())
	app.SetProvider("ktw-test")
	endpoint := ktsp.NewEndpoint("ktw-test", "", "0")
	endpoint.AddSlot(ktslocal.BuildKTSPSlot("orders", ktsp.DefaultInstance, "", orders{}))
	application.PutService[*ktsp.Endpoint](app, "default", endpoint)
	ctx := application.WithApplication(context.Background(), app)
	svc, _ := New(ctx, &Config{})

	d, err := svc.Describe(ctx, actionsapi.DescribeInput{URI: "ktw-test:orders/place-order"})
	if err != nil {
		t.Fatal(err)
	}
	var ast schemaapi.Schema
	if err := json.Unmarshal(d.InputSchemaAST, &ast); err != nil {
		t.Fatalf("input_schema_ast: %v (%s)", err, d.InputSchemaAST)
	}
	root := ast.Types[ast.Root]
	if ast.Root != "PlaceOrderInput" || root == nil || root.Kind != schemaapi.KindObject {
		t.Fatalf("root %q: %+v", ast.Root, root)
	}
	if f := root.Fields["title"]; f == nil || f.Optional || f.Description != "What is ordered" {
		t.Errorf("title = %+v", f)
	}
	if f := root.Fields["lines"]; f == nil || f.Kind != schemaapi.KindArray || !f.Optional {
		t.Errorf("lines = %+v", f)
	}
	if f := root.Fields["ship"]; f == nil || f.Kind != schemaapi.KindObject || f.Fields["city"] == nil {
		t.Errorf("ship = %+v", f)
	}
}

// A task's AST comes from its DSL, its options_from computed into the enum,
// the same as its JSON Schema.
func TestLaunchFormSchemaGivesTheASTWithOptions(t *testing.T) {
	app := application.New(application.Config())
	s := &Service{app: app}
	dsl := []byte("name: t\nroot: In\ntypes:\n  In:\n    type: object\n    fields:\n      who:\n        type: string\n        content: {type: text, options_from: {source: nowhere}}\n      note: string?\n")
	js, ast := s.launchFormSchema(context.Background(), "t", dsl)
	if js == nil || ast == nil {
		t.Fatalf("both projections: %s / %s", js, ast)
	}
	var got schemaapi.Schema
	if err := json.Unmarshal(ast, &got); err != nil || got.Types["In"].Fields["note"] == nil || !got.Types["In"].Fields["note"].Optional {
		t.Fatalf("ast: %v %s", err, ast)
	}
}
