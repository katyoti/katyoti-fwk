// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"strings"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestCompileOutputSchema_DeclaredFieldsReachComplete pins the root-cause fix:
// a TASK.md `output-schema:` must reach the `complete` tool as a JSON Schema
// carrying the task's declared fields. When it does not, the model has nothing
// to fill in, dumps its answer into `summary`, and `task.output` comes back
// empty — with nothing anywhere reporting why.
//
// The input here is the DSL, because that is the one language TASK.md schemas
// are authored in. This test used to feed JSON Schema and assert it passed
// THROUGH untouched; that branch existed only because the language was split,
// and it is gone with the split.
func TestCompileOutputSchema_DeclaredFieldsReachComplete(t *testing.T) {
	dsl := `{"name":"test-scenario-output","root":"TestScenarioOutput","types":{"TestScenarioOutput":{` +
		`"type":"object","fields":{` +
		`"verdict":{"type":"string","enum":["pass","fail","stall"]},` +
		`"child_session_id":{"type":"string","optional":true}}}}}`
	ctx := agentapi.WithPluginContext(context.Background(), map[string]any{"_output_schema": dsl})
	got := compileOutputSchema(ctx)
	if len(got) == 0 {
		t.Fatal("a declared output-schema must compile to a non-empty schema")
	}
	if !strings.Contains(string(got), "verdict") || !strings.Contains(string(got), "child_session_id") {
		t.Fatalf("compiled schema must carry the declared fields; got %s", got)
	}
	// Compiled, not forwarded: `fields` is the DSL's word and must not survive
	// into what the model is shown. Without this the test would pass on a
	// passthrough that never compiled anything.
	if strings.Contains(string(got), `"fields"`) {
		t.Fatalf("schema reached the model in the DSL rather than compiled; got %s", got)
	}
	if !strings.Contains(string(got), `"properties"`) {
		t.Fatalf("compiled schema must be JSON Schema; got %s", got)
	}
}

// TestCompileOutputSchema_EmptyContext: no _output_schema → nil (generic complete).
func TestCompileOutputSchema_EmptyContext(t *testing.T) {
	if compileOutputSchema(context.Background()) != nil {
		t.Fatal("no plugin context must yield nil schema")
	}
	ctx := agentapi.WithPluginContext(context.Background(), map[string]any{})
	if compileOutputSchema(ctx) != nil {
		t.Fatal("absent _output_schema must yield nil schema")
	}
}

// TestCompileOutputSchema_JSONSchemaNoLongerPasses pins the other half of the
// cut: a JSON Schema in `output-schema:` is not a second accepted dialect. It
// compiles to nothing here and the `complete` tool falls back to its generic
// shape — which is the SAFE failure, and the loud one is the census, where the
// author can still act on it.
func TestCompileOutputSchema_JSONSchemaNoLongerPasses(t *testing.T) {
	js := `{"type":"object","properties":{"verdict":{"type":"string"}}}`
	ctx := agentapi.WithPluginContext(context.Background(), map[string]any{"_output_schema": js})
	if got := compileOutputSchema(ctx); got != nil {
		t.Fatalf("JSON Schema must not be accepted as an authored output-schema; got %s", got)
	}
}
