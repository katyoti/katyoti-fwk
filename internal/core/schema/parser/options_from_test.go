// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// An `options_from` block is a content parameter: the parser carries it to
// the AST as written, strict mode included, and the canonical JSON keeps it —
// where ResolveOptions finds it when a form is served.
func TestOptionsFromSurvivesTheParser(t *testing.T) {
	src := []byte(`
name: of-probe
root: Form
types:
  Form:
    type: object
    fields:
      theme:
        type: string
        label: "Thème"
        default: katyoti
        content:
          type: text
          options_from:
            source: catalog
            kind: deck-theme
            where: {data.aspect: "16:9"}
            value: name
            label: description
            limit: 20
`)
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: src, Strict: true})
	if err != nil {
		t.Fatalf("parse: %v (diagnostics %v)", err, out.Diagnostics)
	}
	theme := out.Schema.Types["Form"].Fields["theme"]
	if theme.Content != "text" {
		t.Fatalf("content = %q", theme.Content)
	}
	spec, err := schemaapi.ParseOptionsFrom(theme.ContentParams[schemaapi.OptionsFromKey])
	if err != nil {
		t.Fatal(err)
	}
	if spec.Source != "catalog" || spec.Value != "name" || spec.Label != "description" || spec.Limit != 20 ||
		!reflect.DeepEqual(spec.Params, map[string]any{"kind": "deck-theme"}) ||
		!reflect.DeepEqual(spec.Where, map[string]any{"data.aspect": "16:9"}) {
		t.Fatalf("spec = %+v", spec)
	}

	// Through the canonical JSON the hub serves, and back.
	b, err := out.Schema.MarshalCanonicalJSONIndent("")
	if err != nil {
		t.Fatal(err)
	}
	var back schemaapi.Schema
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if _, err := schemaapi.ParseOptionsFrom(back.Types["Form"].Fields["theme"].ContentParams[schemaapi.OptionsFromKey]); err != nil {
		t.Fatalf("lost in canonical JSON: %v", err)
	}

	// Compiled for a JSON Schema reader (the Actions launch modal), the
	// choices arrive as `enum`.
	src2 := []byte(`
name: of-probe
root: Form
types:
  Form:
    type: object
    fields:
      theme:
        type: string
        content:
          type: text
          options_from: {source: catalog, kind: deck-theme, value: name}
`)
	catalog := schemaapi.OptionSourceFunc(func(context.Context, map[string]any) ([]map[string]any, error) {
		return []map[string]any{{"name": "katyoti"}, {"name": "demo"}}, nil
	})
	js, warnings, err := CompileJSONSchemaWithOptions(context.Background(), src2, map[string]schemaapi.OptionSource{"catalog": catalog})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("compile: %v %v", err, warnings)
	}
	var doc map[string]any
	_ = json.Unmarshal(js, &doc)
	enum := schemaapi.Lookup(doc, "$defs.Form.properties.theme.enum")
	if enum == nil {
		enum = schemaapi.Lookup(doc, "properties.theme.enum")
	}
	if !reflect.DeepEqual(enum, []any{"demo", "katyoti"}) {
		t.Fatalf("enum in the JSON Schema = %v\n%s", enum, js)
	}
}
