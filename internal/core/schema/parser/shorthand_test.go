// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// TestShorthand_Primitive covers the basic `string@email?` form.
func TestShorthand_Primitive(t *testing.T) {
	t1 := mustParseShorthand(t, "string@email?")
	if t1.Kind != schemaapi.KindString {
		t.Errorf("kind: got %q want string", t1.Kind)
	}
	if t1.Content != "email" {
		t.Errorf("content: got %q want email", t1.Content)
	}
	if !t1.Optional {
		t.Errorf("optional: got false want true")
	}
}

// TestShorthand_PrimitiveBare verifies a bare type name produces a typed Type.
func TestShorthand_PrimitiveBare(t *testing.T) {
	cases := []struct {
		input string
		kind  schemaapi.TypeKind
	}{
		{"string", schemaapi.KindString},
		{"number", schemaapi.KindNumber},
		{"boolean", schemaapi.KindBoolean},
		{"any", schemaapi.KindAny},
		{"null", schemaapi.KindNull},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			tt := mustParseShorthand(t, c.input)
			if tt.Kind != c.kind {
				t.Errorf("got kind %q want %q", tt.Kind, c.kind)
			}
			if tt.Optional {
				t.Errorf("unexpected optional flag")
			}
		})
	}
}

// TestShorthand_InlineParams verifies `string@code(language:typescript)` parses.
func TestShorthand_InlineParams(t *testing.T) {
	tt := mustParseShorthand(t, "string@code(language:typescript)")
	if tt.Content != "code" {
		t.Errorf("content: got %q want code", tt.Content)
	}
	if got, ok := tt.ContentParams["language"].(string); !ok || got != "typescript" {
		t.Errorf("language param: got %v want typescript", tt.ContentParams["language"])
	}
}

// TestShorthand_InlineParamsTypes verifies value coercion (int / float / bool).
func TestShorthand_InlineParamsTypes(t *testing.T) {
	tt := mustParseShorthand(t, "number@slider(min:0, max:100, step:5, marks:true)")
	p := tt.ContentParams
	if got, _ := p["min"].(int64); got != 0 {
		t.Errorf("min: got %v want 0", p["min"])
	}
	if got, _ := p["max"].(int64); got != 100 {
		t.Errorf("max: got %v want 100", p["max"])
	}
	if got, _ := p["step"].(int64); got != 5 {
		t.Errorf("step: got %v want 5", p["step"])
	}
	if got, _ := p["marks"].(bool); !got {
		t.Errorf("marks: got %v want true", p["marks"])
	}
}

// TestBlock_ContentRichOptionsPassThrough pins the contract that the parser
// carries a content-type's RICH `options` list (value + label + description +
// icon) through to ContentParams UNINTERPRETED. Go never models options; the
// GUI reads them off the canonical AST. This guards the rich-enum-options flow
// (schema-rich-enum-options): a regression that dropped or reshaped these would
// silently break the rich document-form pickers.
func TestBlock_ContentRichOptionsPassThrough(t *testing.T) {
	m := map[string]any{
		"type": "string",
		"content": map[string]any{
			"type": "text",
			"options": []any{
				map[string]any{
					"value":       "etude-architecture",
					"label":       "Étude d'architecture",
					"description": "Dossier d'architecture complet",
					"icon":        "IconBuildingSkyscraper",
				},
				map[string]any{"value": "matrice-flux", "label": "Matrice de flux"},
			},
		},
	}
	tt, diags := buildBlock(m, "types.X")
	for _, d := range diags {
		if d.Severity == schemaapi.SeverityError {
			t.Fatalf("unexpected error diagnostic: %+v", d)
		}
	}
	if tt.Content != "text" {
		t.Errorf("content: got %q want text", tt.Content)
	}
	rawOpts, ok := tt.ContentParams["options"].([]any)
	if !ok {
		t.Fatalf("options: not preserved as []any in ContentParams; got %T", tt.ContentParams["options"])
	}
	if len(rawOpts) != 2 {
		t.Fatalf("options: got %d want 2", len(rawOpts))
	}
	first, ok := rawOpts[0].(map[string]any)
	if !ok {
		t.Fatalf("options[0]: got %T want map", rawOpts[0])
	}
	if first["description"] != "Dossier d'architecture complet" {
		t.Errorf("options[0].description not preserved: got %v", first["description"])
	}
	if first["icon"] != "IconBuildingSkyscraper" {
		t.Errorf("options[0].icon not preserved: got %v", first["icon"])
	}
}

// TestShorthand_ArrayPrimitive covers `string[]`.
func TestShorthand_ArrayPrimitive(t *testing.T) {
	tt := mustParseShorthand(t, "string[]")
	if tt.Kind != schemaapi.KindArray {
		t.Errorf("kind: got %q want array", tt.Kind)
	}
	if tt.Items == nil || tt.Items.Inline == nil {
		t.Fatalf("Items.Inline missing: %+v", tt.Items)
	}
	if tt.Items.Inline.Kind != schemaapi.KindString {
		t.Errorf("Items.Inline.Kind: got %q want string", tt.Items.Inline.Kind)
	}
}

// TestShorthand_ArrayUserType covers `Item[]` (user-type ref preserved).
func TestShorthand_ArrayUserType(t *testing.T) {
	tt := mustParseShorthand(t, "Item[]")
	if tt.Kind != schemaapi.KindArray {
		t.Errorf("kind: got %q want array", tt.Kind)
	}
	if tt.Items == nil || !tt.Items.IsName() || tt.Items.Name != "Item" {
		t.Errorf("Items: expected name ref to Item, got %+v", tt.Items)
	}
}

// TestShorthand_LinkMany2One covers `->User`.
func TestShorthand_LinkMany2One(t *testing.T) {
	tt := mustParseShorthand(t, "->User")
	if tt.Kind != schemaapi.KindLink {
		t.Errorf("kind: got %q want link", tt.Kind)
	}
	if tt.Target != "User" {
		t.Errorf("target: got %q want User", tt.Target)
	}
	if tt.LinkMode != schemaapi.LinkMany2One {
		t.Errorf("mode: got %q want many2one", tt.LinkMode)
	}
}

// TestShorthand_LinkMany2Many covers `->User[]`.
func TestShorthand_LinkMany2Many(t *testing.T) {
	tt := mustParseShorthand(t, "->User[]")
	if tt.Kind != schemaapi.KindLink {
		t.Errorf("kind: got %q want link", tt.Kind)
	}
	if tt.Target != "User" {
		t.Errorf("target: got %q", tt.Target)
	}
	if tt.LinkMode != schemaapi.LinkMany2Many {
		t.Errorf("mode: got %q want many2many", tt.LinkMode)
	}
}

// TestShorthand_Tree covers `~Category`.
func TestShorthand_Tree(t *testing.T) {
	tt := mustParseShorthand(t, "~Category")
	if tt.Kind != schemaapi.KindTree {
		t.Errorf("kind: got %q want tree", tt.Kind)
	}
	if tt.Target != "Category" {
		t.Errorf("target: got %q", tt.Target)
	}
	if tt.LinkMode != schemaapi.LinkMany2One {
		t.Errorf("mode: got %q want many2one", tt.LinkMode)
	}
	if tt.LinkType != "part_of" {
		t.Errorf("linkType: got %q want part_of", tt.LinkType)
	}
}

// TestShorthand_InlineChoice covers `string | number | null`.
func TestShorthand_InlineChoice(t *testing.T) {
	tt := mustParseShorthand(t, "string | number | null")
	if tt.Kind != schemaapi.KindChoice {
		t.Errorf("kind: got %q want choice", tt.Kind)
	}
	if len(tt.Cases) != 3 {
		t.Fatalf("cases: got %d want 3", len(tt.Cases))
	}
	wantKinds := []schemaapi.TypeKind{schemaapi.KindString, schemaapi.KindNumber, schemaapi.KindNull}
	for i, c := range tt.Cases {
		if c.Inline == nil || c.Inline.Kind != wantKinds[i] {
			t.Errorf("case[%d]: got %v want %s", i, c.Inline, wantKinds[i])
		}
	}
}

// TestShorthand_BlockOverridesShorthand verifies a long-form block can both
// use shorthand in the `type:` field AND override fields.
func TestShorthand_BlockOverridesShorthand(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: t
types:
  Email:
    type: string@email
    label: Email Address
    optional: true
    description: User email
`
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tt := out.Schema.Types["Email"]
	if tt == nil {
		t.Fatalf("missing Email type")
	}
	if tt.Kind != schemaapi.KindString || tt.Content != "email" {
		t.Errorf("type/content from shorthand: got %q/%q", tt.Kind, tt.Content)
	}
	if tt.Label != "Email Address" {
		t.Errorf("label override missing: got %q", tt.Label)
	}
	if !tt.Optional {
		t.Errorf("optional override missing")
	}
	if tt.Description != "User email" {
		t.Errorf("description override missing")
	}
}

// TestShorthand_FullSchemaIntegration verifies a multi-type schema with
// mixed shorthand variants parses end-to-end.
func TestShorthand_FullSchemaIntegration(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: blog
types:
  User:
    type: object
    id: id
    fields:
      id: string@uuid
      email: string@email?
      name: string@text(max:100)
      tags: string[]
      manager: ->User
      friends: ->User[]
      parent: ~User
      mixed: string | number | null
`
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	user := out.Schema.Types["User"]
	if user == nil || user.Kind != schemaapi.KindObject {
		t.Fatalf("User missing or wrong kind: %+v", user)
	}
	if got := user.Fields["email"]; got == nil || got.Content != "email" || !got.Optional {
		t.Errorf("email field: %+v", got)
	}
	if got := user.Fields["tags"]; got == nil || got.Kind != schemaapi.KindArray {
		t.Errorf("tags field: %+v", got)
	}
	if got := user.Fields["manager"]; got == nil || got.Kind != schemaapi.KindLink || got.LinkMode != schemaapi.LinkMany2One {
		t.Errorf("manager link: %+v", got)
	}
	if got := user.Fields["friends"]; got == nil || got.Kind != schemaapi.KindLink || got.LinkMode != schemaapi.LinkMany2Many {
		t.Errorf("friends link: %+v", got)
	}
	if got := user.Fields["parent"]; got == nil || got.Kind != schemaapi.KindTree {
		t.Errorf("parent tree: %+v", got)
	}
	if got := user.Fields["mixed"]; got == nil || got.Kind != schemaapi.KindChoice || len(got.Cases) != 3 {
		t.Errorf("mixed choice: %+v", got)
	}
}

// TestShorthand_StringEnum covers `string@enum(admin, member, guest)`.
// @enum is structural sugar (populates Type.Enum, NOT Type.Content) — it
// declares the value set, not a content-type refinement.
func TestShorthand_StringEnum(t *testing.T) {
	tt := mustParseShorthand(t, "string@enum(admin, member, guest)")
	if tt.Kind != schemaapi.KindString {
		t.Errorf("kind: got %q want string", tt.Kind)
	}
	if tt.Content != "" {
		t.Errorf("content: got %q want empty (enum is structural, not content)", tt.Content)
	}
	want := []any{"admin", "member", "guest"}
	if len(tt.Enum) != len(want) {
		t.Fatalf("enum len: got %d want %d (%v)", len(tt.Enum), len(want), tt.Enum)
	}
	for i := range want {
		if tt.Enum[i] != want[i] {
			t.Errorf("enum[%d]: got %v want %v", i, tt.Enum[i], want[i])
		}
	}
}

// TestShorthand_StringEnumOptional verifies `?` still works.
func TestShorthand_StringEnumOptional(t *testing.T) {
	tt := mustParseShorthand(t, "string@enum(active, archived)?")
	if !tt.Optional {
		t.Errorf("optional: got false want true")
	}
	if len(tt.Enum) != 2 {
		t.Errorf("enum: got %v want 2 values", tt.Enum)
	}
}

// TestShorthand_NumberEnum covers integer + float enums.
func TestShorthand_NumberEnum(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		tt := mustParseShorthand(t, "number@enum(1, 2, 3)")
		if tt.Kind != schemaapi.KindNumber {
			t.Errorf("kind: got %q want number", tt.Kind)
		}
		for i, want := range []int64{1, 2, 3} {
			if got, _ := tt.Enum[i].(int64); got != want {
				t.Errorf("enum[%d]: got %v want %v", i, tt.Enum[i], want)
			}
		}
	})
	t.Run("float", func(t *testing.T) {
		tt := mustParseShorthand(t, "number@enum(0.1, 0.5, 1.0)")
		for i, want := range []float64{0.1, 0.5, 1.0} {
			if got, _ := tt.Enum[i].(float64); got != want {
				t.Errorf("enum[%d]: got %v want %v", i, tt.Enum[i], want)
			}
		}
	})
}

// TestShorthand_EnumQuoted verifies surrounding-quote stripping (single + double).
func TestShorthand_EnumQuoted(t *testing.T) {
	tt := mustParseShorthand(t, `string@enum("admin", 'member', guest)`)
	want := []any{"admin", "member", "guest"}
	for i := range want {
		if tt.Enum[i] != want[i] {
			t.Errorf("enum[%d]: got %v want %v", i, tt.Enum[i], want[i])
		}
	}
}

// TestShorthand_EmptyEnum_Errors enforces that `@enum()` is a parse error.
func TestShorthand_EmptyEnum_Errors(t *testing.T) {
	_, derr := parseShorthand("string@enum()", "test")
	if derr == nil {
		t.Fatal("expected parse error for empty @enum, got nil")
	}
	if derr.Code != "EMPTY_ENUM" {
		t.Errorf("code: got %q want EMPTY_ENUM", derr.Code)
	}
}

// TestShorthand_EnumStrayComma_Errors enforces that `@enum(a,,b)` is a parse error.
func TestShorthand_EnumStrayComma_Errors(t *testing.T) {
	_, derr := parseShorthand("string@enum(a,,b)", "test")
	if derr == nil {
		t.Fatal("expected parse error for stray comma, got nil")
	}
	if derr.Code != "EMPTY_ENUM_VALUE" {
		t.Errorf("code: got %q want EMPTY_ENUM_VALUE", derr.Code)
	}
}

// helpers

func mustParseShorthand(t *testing.T, s string) *schemaapi.Type {
	t.Helper()
	tt, derr := parseShorthand(s, "test")
	if derr != nil {
		t.Fatalf("parseShorthand(%q): %s", s, derr.Message)
	}
	return tt
}
