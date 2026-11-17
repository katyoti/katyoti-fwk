// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// sectionSvc builds a service whose catalog declares the type-level `context`
// capability, its type_schema compiled, and the given types, in mode.
func sectionSvc(t *testing.T, mode string, types ...api.ObjectType) *Service {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ObjectTypeValidation: mode}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "content", Schema: "x:/data-type/cap-content"},
		{Name: "context", TypeSchema: "x:/data-type/type-context"},
	}
	cfg.CatalogTypes.CapabilitySchemas = map[string]json.RawMessage{
		"x:/data-type/type-context": json.RawMessage(`{"type":"object","properties":{"propagation":{"type":"string","enum":["local","inherit-from-ancestors"]},"briefing_fields":{"type":"array","items":{"type":"string"}}}}`),
	}
	cfg.CatalogTypes.Objects = types
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func withSection(name, section string, v map[string]any) api.ObjectType {
	return api.ObjectType{Name: name, Sections: map[string]map[string]any{section: v}}
}

// A section is checked against its capability's type_schema when the type is
// declared: a good one registers the type; a wrong field type, or a section no
// capability declares (or one whose capability has no type_schema), refuses
// the type in error mode, keeps it with a warning in warn mode.
func TestTypeSectionsAreValidatedWhenDeclared(t *testing.T) {
	good := withSection("good", "context", map[string]any{"propagation": "local", "briefing_fields": []any{"topic"}})
	wrongType := withSection("wrong-type", "context", map[string]any{"briefing_fields": "topic"})
	wrongValue := withSection("wrong-value", "context", map[string]any{"propagation": "everywhere"})
	undeclared := withSection("undeclared", "nobody", map[string]any{"x": 1})
	noTypeSchema := withSection("no-type-schema", "content", map[string]any{"x": 1})

	svc := sectionSvc(t, api.TypeValidationError, good, wrongType, wrongValue, undeclared, noTypeSchema)
	if _, ok := svc.lookupObjectType("good"); !ok {
		t.Errorf("a good section refused its type: %v", svc.sectionProblems(good, svc.decl().Capabilities))
	}
	for _, bad := range []api.ObjectType{wrongType, wrongValue, undeclared, noTypeSchema} {
		if _, ok := svc.lookupObjectType(bad.Name); ok {
			t.Errorf("type %q registered in error mode: its section does not check", bad.Name)
		}
		problems := svc.sectionProblems(bad, svc.decl().Capabilities)
		if len(problems) == 0 {
			t.Errorf("type %q: no problem reported", bad.Name)
		}
	}
	if p := svc.sectionProblems(undeclared, svc.decl().Capabilities); len(p) != 1 || !strings.Contains(p[0], "no declared capability") {
		t.Errorf("undeclared section problem = %v", p)
	}

	// Warn keeps the type and says why it would be refused.
	warn := sectionSvc(t, api.TypeValidationWarn, undeclared)
	if _, ok := warn.lookupObjectType("undeclared"); !ok {
		t.Error("warn mode refused a type")
	}
}

// A refused type is asked again on the next refresh: once its capability is
// declared, it registers.
func TestARefusedTypeRegistersWhenItsCapabilityArrives(t *testing.T) {
	late := withSection("late", "verification", map[string]any{"gate_blocks_on": "warn"})
	svc := sectionSvc(t, api.TypeValidationError, late)
	if _, ok := svc.lookupObjectType("late"); ok {
		t.Fatal("registered before its capability was declared")
	}
	svc.RefreshCatalogTypes(CatalogTypeSet{
		Objects:      []api.ObjectType{late},
		Capabilities: []api.Capability{{Name: "verification", TypeSchema: "x:/data-type/type-verification"}},
	})
	if _, ok := svc.lookupObjectType("late"); !ok {
		t.Error("the refused type was not registered once its capability arrived")
	}
}

// A type-level capability carries no group on objects: an object storing one
// under its name is refused in error mode.
func TestATypeLevelCapabilityCarriesNoGroup(t *testing.T) {
	svc := sectionSvc(t, api.TypeValidationError, api.ObjectType{Name: "thing", Capabilities: []string{"content"}})
	o := obj("cwm.t", "thing", "")
	o.Capabilities = map[string]map[string]any{"context": {"propagation": "local"}}
	_, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o})
	if err == nil || !strings.Contains(err.Error(), "carries no group on objects") {
		t.Fatalf("err = %v, want the type-level group refused", err)
	}
}
