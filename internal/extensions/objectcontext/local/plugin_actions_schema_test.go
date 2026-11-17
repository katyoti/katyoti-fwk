// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	schemacore "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	schemasvc "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// parsingSvc is a SchemaService that answers Parse with one object of one
// string field, `topic`, and records the DSL it was handed.
type parsingSvc struct {
	schemasvc.SchemaService
	got []byte
}

func (s *parsingSvc) Parse(_ context.Context, in schemacore.ParseInput) (schemacore.ParseOutput, error) {
	s.got = in.Body
	return schemacore.ParseOutput{Schema: &schemacore.Schema{Name: "in", Root: "In", Types: map[string]*schemacore.Type{
		"In": {Kind: schemacore.KindObject, Fields: map[string]*schemacore.Type{"topic": {Kind: schemacore.KindString}}},
	}}}, nil
}

// inputCatalog knows one task, whose input is declared in the DSL.
type inputCatalog struct{ taskapi.TaskService }

func (inputCatalog) Get(_ context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	return taskapi.TaskInfo{Name: in.Name, Description: "generate", InputSchema: []byte("name: in\nroot: In\n")}, nil
}

// TestANodeActionsInputIsCompiledByTheSchemaService: a task's input DSL
// reaches the model as JSON Schema, compiled through the SchemaService - this
// package never parses for itself (schema-parser-boundary).
func TestANodeActionsInputIsCompiledByTheSchemaService(t *testing.T) {
	svc := &parsingSvc{}
	app := application.New(application.Config())
	application.PutService[schemasvc.SchemaService](app, "default", svc)
	ctx := application.WithApplication(context.Background(), app)

	p := newNodeActions(ctx, inputCatalog{}, []string{"ktw-methods:tasks/generate"}, func() time.Time { return time.Unix(0, 0) })
	defs := p.ToolDefs()
	if len(defs) != 1 {
		t.Fatalf("tools = %d, want 1", len(defs))
	}
	if string(svc.got) != "name: in\nroot: In\n" {
		t.Fatalf("the SchemaService was handed %q, not the task's DSL", svc.got)
	}
	if params := string(defs[0].InputSchema); !strings.Contains(params, `"topic"`) {
		t.Fatalf("the tool's parameters are not the compiled schema: %s", params)
	}
}
