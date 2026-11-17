// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
)

// The Go struct and the shipped DSL declaration must agree field for field.
//
// This is lesson at the Go/DSL boundary. ObjectType just absorbed 30
// fields from NodeType and ItemTypeRegistration; if the declaration keeps the
// old six, the extra fields are UNDECLARED SHAPE — invisible to schema-driven
// validation and to any form the GUI generates from the schema, and under D5
// they would all surface as failures at once.
//
// Drift in either direction is a defect, so both are checked: a Go field with
// no declared counterpart is unvalidated, and a declared field with no Go
// counterpart is a promise nothing can keep.
func TestObjectTypeSchemaMatchesStruct(t *testing.T) {
	// The declaration is a SYSTEM data-type, beside node-type and item-type —
	// every binary publishing a CWM type pack publishes object types now, so the
	// kind cannot live in one method pack.
	path := filepath.Join("..", "..", "..", "..",
		"internal", "core", "contribution", "embed", "data-type", "object-type.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shipped declaration not reachable: %v", err)
	}
	out, err := parser.ParseFile(context.Background(), schemaapi.ParseFileInput{Path: path})
	if err != nil {
		t.Fatalf("shipped object-type declaration does not parse: %v", err)
	}
	root, ok := out.Schema.Types[out.Schema.Root]
	if !ok {
		t.Fatalf("root type %q missing from the declaration", out.Schema.Root)
	}

	declared := map[string]bool{}
	for name := range root.Fields {
		declared[name] = true
	}

	inGo := map[string]bool{}
	ot := reflect.TypeOf(ObjectType{})
	for i := 0; i < ot.NumField(); i++ {
		tag := ot.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		inGo[strings.Split(tag, ",")[0]] = true
	}

	var missing, extra []string
	for f := range inGo {
		if !declared[f] {
			missing = append(missing, f)
		}
	}
	for f := range declared {
		if !inGo[f] {
			extra = append(extra, f)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("ObjectType fields with NO declaration (undeclared shape — unvalidated, and invisible to schema-driven forms): %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("declared fields with no Go counterpart (a promise nothing keeps): %v", extra)
	}
}

// The same drift guard for `capability`.
//
// It is here because the drift ALREADY HAPPENED: phase 2 added `Attach` to the
// Go struct and the DSL kept eight fields, so `attach` was undeclared shape —
// invisible to schema-driven validation and to any form generated from it,
// exactly the defect exists to catch one level down. Nothing failed; the
// census only showed a hash moving.
func TestCapabilitySchemaMatchesStruct(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..",
		"internal", "core", "contribution", "embed", "data-type", "capability.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shipped declaration not reachable: %v", err)
	}
	out, err := parser.ParseFile(context.Background(), schemaapi.ParseFileInput{Path: path})
	if err != nil {
		t.Fatalf("shipped capability declaration does not parse: %v", err)
	}
	root, ok := out.Schema.Types[out.Schema.Root]
	if !ok {
		t.Fatalf("root type %q missing", out.Schema.Root)
	}

	declared := map[string]bool{}
	for name := range root.Fields {
		declared[name] = true
	}
	inGo := map[string]bool{}
	ct := reflect.TypeOf(Capability{})
	for i := 0; i < ct.NumField(); i++ {
		tag := ct.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		inGo[strings.Split(tag, ",")[0]] = true
	}

	var missing, extra []string
	for f := range inGo {
		if !declared[f] {
			missing = append(missing, f)
		}
	}
	for f := range declared {
		if !inGo[f] {
			extra = append(extra, f)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("Capability fields with NO declaration (undeclared shape — unvalidated): %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("declared fields with no Go counterpart (a promise nothing keeps): %v", extra)
	}
}
