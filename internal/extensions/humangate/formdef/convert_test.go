// SPDX-License-Identifier: Apache-2.0

package formdef

import (
	"encoding/json"
	"testing"
)

func TestConvert_FlatForm(t *testing.T) {
	def := &FormDefinition{
		Screen: Screen{Title: "Test"},
		Fields: []Field{
			{Key: "name", Type: "string", Label: "Name", Required: true},
			{Key: "age", Type: "integer", Label: "Age", Validation: &Validation{Minimum: ptr(0.0), Maximum: ptr(150.0)}},
			{Key: "active", Type: "boolean", Label: "Active"},
		},
	}
	res, err := Convert(def)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	// Check JSON Schema
	js := res.JSONSchema
	if js["type"] != "object" {
		t.Errorf("schema type = %v", js["type"])
	}
	props := js["properties"].(map[string]any)
	if props["name"].(map[string]any)["type"] != "string" {
		t.Error("name should be string")
	}
	if props["age"].(map[string]any)["type"] != "integer" {
		t.Error("age should be integer")
	}
	if props["age"].(map[string]any)["maximum"] != 150.0 {
		t.Error("age maximum should be 150")
	}
	req := js["required"].([]any)
	if len(req) != 1 || req[0] != "name" {
		t.Errorf("required = %v", req)
	}

	// Check UI Schema
	ui := res.UISchema
	if ui["type"] != "VerticalLayout" {
		t.Errorf("ui type = %v", ui["type"])
	}
	elems := ui["elements"].([]any)
	if len(elems) != 3 {
		t.Fatalf("ui elements = %d, want 3", len(elems))
	}
	first := elems[0].(map[string]any)
	if first["scope"] != "#/properties/name" {
		t.Errorf("first scope = %v", first["scope"])
	}
}

func TestConvert_TabbedForm(t *testing.T) {
	def := &FormDefinition{
		Screen: Screen{Title: "Tabs"},
		Groups: []Group{
			{Key: "tab1", Label: "Tab 1", Fields: []Field{
				{Key: "f1", Type: "string"},
			}},
			{Key: "tab2", Label: "Tab 2", Fields: []Field{
				{Key: "f2", Type: "integer"},
			}},
		},
	}
	res, err := Convert(def)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	// JSON Schema should have both fields at top level
	props := res.JSONSchema["properties"].(map[string]any)
	if len(props) != 2 {
		t.Errorf("props = %d, want 2", len(props))
	}

	// UI Schema should be Categorization
	if res.UISchema["type"] != "Categorization" {
		t.Errorf("ui type = %v, want Categorization", res.UISchema["type"])
	}
	cats := res.UISchema["elements"].([]any)
	if len(cats) != 2 {
		t.Fatalf("categories = %d, want 2", len(cats))
	}
	cat1 := cats[0].(map[string]any)
	if cat1["type"] != "Category" {
		t.Errorf("cat type = %v", cat1["type"])
	}
	if cat1["label"] != "Tab 1" {
		t.Errorf("cat label = %v", cat1["label"])
	}
}

func TestConvert_EnumField(t *testing.T) {
	def := &FormDefinition{
		Screen: Screen{Title: "Test"},
		Fields: []Field{
			{Key: "color", Type: "enum", Label: "Color", Options: []Option{
				{Value: "red", Label: "Red"},
				{Value: "blue", Label: "Blue"},
			}},
		},
	}
	res, err := Convert(def)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	prop := res.JSONSchema["properties"].(map[string]any)["color"].(map[string]any)
	if prop["type"] != "string" {
		t.Error("enum should be string type")
	}
	oneOf := prop["oneOf"].([]any)
	if len(oneOf) != 2 {
		t.Fatalf("oneOf len = %d, want 2", len(oneOf))
	}
	first := oneOf[0].(map[string]any)
	if first["const"] != "red" || first["title"] != "Red" {
		t.Errorf("oneOf[0] = %v", first)
	}
}

func TestConvert_NestedObject(t *testing.T) {
	def := &FormDefinition{
		Screen: Screen{Title: "Test"},
		Fields: []Field{
			{Key: "address", Type: "object", Label: "Address", Fields: []Field{
				{Key: "street", Type: "string"},
				{Key: "city", Type: "string"},
			}},
		},
	}
	res, err := Convert(def)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	addr := res.JSONSchema["properties"].(map[string]any)["address"].(map[string]any)
	if addr["type"] != "object" {
		t.Errorf("address type = %v", addr["type"])
	}
	addrProps := addr["properties"].(map[string]any)
	if len(addrProps) != 2 {
		t.Errorf("address properties = %d", len(addrProps))
	}

	// UI should render object as Group with nested controls
	elems := res.UISchema["elements"].([]any)
	group := elems[0].(map[string]any)
	if group["type"] != "Group" {
		t.Errorf("object ui type = %v, want Group", group["type"])
	}
	subElems := group["elements"].([]any)
	if len(subElems) != 2 {
		t.Errorf("sub elements = %d", len(subElems))
	}
	streetScope := subElems[0].(map[string]any)["scope"]
	if streetScope != "#/properties/address/properties/street" {
		t.Errorf("street scope = %v", streetScope)
	}
}

func TestConvert_DateTimeTypes(t *testing.T) {
	def := &FormDefinition{
		Screen: Screen{Title: "Test"},
		Fields: []Field{
			{Key: "d", Type: "date"},
			{Key: "t", Type: "time"},
			{Key: "dt", Type: "datetime"},
		},
	}
	res, err := Convert(def)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	props := res.JSONSchema["properties"].(map[string]any)
	if props["d"].(map[string]any)["format"] != "date" {
		t.Error("date format wrong")
	}
	if props["t"].(map[string]any)["format"] != "time" {
		t.Error("time format wrong")
	}
	if props["dt"].(map[string]any)["format"] != "date-time" {
		t.Error("datetime format wrong")
	}
}

func TestConvert_ArrayField(t *testing.T) {
	def := &FormDefinition{
		Screen: Screen{Title: "Test"},
		Fields: []Field{
			{Key: "tags", Type: "array", Items: &Field{Key: "tag", Type: "string"}},
		},
	}
	res, err := Convert(def)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	tags := res.JSONSchema["properties"].(map[string]any)["tags"].(map[string]any)
	if tags["type"] != "array" {
		t.Error("tags should be array")
	}
	items := tags["items"].(map[string]any)
	if items["type"] != "string" {
		t.Error("items should be string")
	}
}

func TestConvert_TableField(t *testing.T) {
	def := &FormDefinition{
		Screen: Screen{Title: "Test"},
		Fields: []Field{
			{Key: "rows", Type: "table", Columns: []Field{
				{Key: "name", Type: "string"},
				{Key: "qty", Type: "integer"},
			}},
		},
	}
	res, err := Convert(def)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	rows := res.JSONSchema["properties"].(map[string]any)["rows"].(map[string]any)
	if rows["type"] != "array" {
		t.Error("table should be array")
	}
	items := rows["items"].(map[string]any)
	if items["type"] != "object" {
		t.Error("table items should be object")
	}
}

func TestConvert_RoundTrip_JSON(t *testing.T) {
	// Verify the output can be marshaled to valid JSON
	def := &FormDefinition{
		Screen: Screen{Title: "Test"},
		Groups: []Group{
			{Key: "g1", Label: "G1", Fields: []Field{
				{Key: "x", Type: "string", Required: true, Default: "hello"},
			}},
		},
	}
	res, err := Convert(def)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if _, err := json.Marshal(res.JSONSchema); err != nil {
		t.Fatalf("marshal JSONSchema: %v", err)
	}
	if _, err := json.Marshal(res.UISchema); err != nil {
		t.Fatalf("marshal UISchema: %v", err)
	}
}

func ptr(f float64) *float64 { return &f }
