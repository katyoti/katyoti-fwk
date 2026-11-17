// SPDX-License-Identifier: Apache-2.0

package reflect_test

import (
	"encoding/json"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

type sampleInput struct {
	Title       string   `json:"title"                jsonschema:"description=The title"  ui-form:"label=Title,widget=text"`
	Count       int      `json:"count,omitempty"      jsonschema:"description=How many"`
	Tags        []string `json:"tags,omitempty"`
	IsActive    bool     `json:"is_active,omitempty"`
	Description string   `json:"description,omitempty" ui-cli:"primary"`
	Requires    []string `json:"requires,omitempty"   dsl:"link=many2many,relation=requires,target=Contribution"`
}

func TestTypeAST_struct_basic(t *testing.T) {
	ast, err := corereflect.TypeAST[sampleInput]()
	if err != nil {
		t.Fatalf("TypeAST: %v", err)
	}
	if ast.Kind != schemaapi.KindObject {
		t.Errorf("expected KindObject, got %q", ast.Kind)
	}
	if len(ast.Fields) != 6 {
		t.Errorf("expected 6 fields, got %d", len(ast.Fields))
	}
	if ast.Fields["title"] == nil || ast.Fields["title"].Kind != schemaapi.KindString {
		t.Errorf("title field missing or wrong kind")
	}
	if ast.Fields["title"].Description != "The title" {
		t.Errorf("title description not extracted: %q", ast.Fields["title"].Description)
	}
	if ast.Fields["title"].Optional {
		t.Errorf("title should not be optional (no omitempty)")
	}
	if !ast.Fields["count"].Optional {
		t.Errorf("count should be optional (has omitempty)")
	}
}

func TestTypeAST_uiExtension(t *testing.T) {
	ast, _ := corereflect.TypeAST[sampleInput]()
	titleExts := ast.Fields["title"].Extensions
	foundUIForm := false
	for _, ext := range titleExts {
		if ext.Kind == "ui-form" {
			foundUIForm = true
			if ext.Payload["label"] != "Title" {
				t.Errorf("ui-form label not parsed: %v", ext.Payload)
			}
			if ext.Payload["widget"] != "text" {
				t.Errorf("ui-form widget not parsed: %v", ext.Payload)
			}
		}
	}
	if !foundUIForm {
		t.Errorf("ui-form extension not extracted on title field")
	}
}

func TestTypeAST_dslLinkAnnotation(t *testing.T) {
	ast, _ := corereflect.TypeAST[sampleInput]()
	requires := ast.Fields["requires"]
	if requires == nil {
		t.Fatalf("requires field missing")
	}
	if requires.LinkMode != schemaapi.LinkMany2Many {
		t.Errorf("expected LinkMany2Many, got %q", requires.LinkMode)
	}
	if requires.Target != "Contribution" {
		t.Errorf("expected target Contribution, got %q", requires.Target)
	}
}

func TestTypeAST_arrayElement(t *testing.T) {
	ast, _ := corereflect.TypeAST[sampleInput]()
	tags := ast.Fields["tags"]
	if tags.Kind != schemaapi.KindArray {
		t.Errorf("tags should be array, got %q", tags.Kind)
	}
	if tags.Items == nil || tags.Items.Inline == nil {
		t.Fatalf("tags items missing")
	}
	if tags.Items.Inline.Kind != schemaapi.KindString {
		t.Errorf("tags items should be string, got %q", tags.Items.Inline.Kind)
	}
}

func TestTypeAST_toJSONSchema_roundtrip(t *testing.T) {
	ast, err := corereflect.TypeAST[sampleInput]()
	if err != nil {
		t.Fatalf("TypeAST: %v", err)
	}
	js, err := ast.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	// Verify it's valid JSON and structurally sound.
	var decoded map[string]any
	if err := json.Unmarshal(js, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("expected draft-2020-12 schema URI, got %v", decoded["$schema"])
	}
	if decoded["type"] != "object" {
		t.Errorf("expected type=object, got %v", decoded["type"])
	}
	props, ok := decoded["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing or wrong type")
	}
	if _, ok := props["title"]; !ok {
		t.Errorf("title property missing in JSON Schema")
	}
	required, _ := decoded["required"].([]any)
	if len(required) != 1 || required[0] != "title" {
		t.Errorf("expected required=[title], got %v", required)
	}
}

func TestMustTypeAST_returnsValue(t *testing.T) {
	ast := corereflect.MustTypeAST[sampleInput]()
	if ast == nil {
		t.Fatalf("MustTypeAST returned nil")
	}
	if ast.Kind != schemaapi.KindObject {
		t.Errorf("expected KindObject, got %q", ast.Kind)
	}
}

type enumInput struct {
	Aspect string `json:"aspect,omitempty" jsonschema:"description=Slide ratio,enum=16:9|4:3"`
}

func TestTypeAST_enumTag(t *testing.T) {
	ast, err := corereflect.TypeAST[enumInput]()
	if err != nil {
		t.Fatal(err)
	}
	f := ast.Fields["aspect"]
	if f == nil || len(f.Enum) != 2 || f.Enum[0] != "16:9" || f.Enum[1] != "4:3" {
		t.Fatalf("enum not read from the jsonschema tag: %+v", f)
	}
	if f.Description != "Slide ratio" || !f.Optional {
		t.Fatalf("enum must not disturb description/optional: %+v", f)
	}
}

type contentInput struct {
	Details string `json:"details,omitempty" jsonschema:"description=What the ask leaves out,content=markdown"`
}

// The content tag gives a string field its content type, as the DSL's
// string@markdown — so a form edits it with the right editor.
func TestTypeAST_contentTag(t *testing.T) {
	ast, err := corereflect.TypeAST[contentInput]()
	if err != nil {
		t.Fatal(err)
	}
	f := ast.Fields["details"]
	if f == nil || f.Content != "markdown" || f.Description != "What the ask leaves out" {
		t.Fatalf("content not read from the jsonschema tag: %+v", f)
	}
}

type openUnion struct{ A, B string }

func (openUnion) OpenSchema() {}

// TestTypeAST_OpenSchema: a type declaring OpenSchema renders as any, not
// as its fields.
func TestTypeAST_OpenSchema(t *testing.T) {
	ty, err := corereflect.TypeAST[struct {
		U openUnion `json:"u"`
	}]()
	if err != nil {
		t.Fatal(err)
	}
	if got := ty.Fields["u"]; got == nil || got.Kind != schemaapi.KindAny {
		t.Errorf("an OpenSchema field must be any, got %+v", got)
	}
}
