// SPDX-License-Identifier: Apache-2.0

package core

import (
	"reflect"
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a documentation module drops the empty code group version 17
// left on it; a code group holding anything, and a software module's, are
// kept; a second pass changes nothing.
func TestADocumentationModuleDropsItsEmptyCodeGroup(t *testing.T) {
	doc := &api.Object{ID: "rules", Type: "documentation", Capabilities: map[string]map[string]any{
		"code":    {"code_packages": []any{}},
		"module":  {"decisions": "none"},
		"planned": {},
	}}
	changed, err := documentationDropsEmptyCode(doc)
	if err != nil || !changed {
		t.Fatalf("a documentation module's empty code group was not dropped: %v %v", changed, err)
	}
	want := map[string]map[string]any{"module": {"decisions": "none"}, "planned": {}}
	if !reflect.DeepEqual(doc.Capabilities, want) {
		t.Errorf("the documentation module's groups: %v; want %v", doc.Capabilities, want)
	}
	if again, _ := documentationDropsEmptyCode(doc); again {
		t.Error("the step is not idempotent")
	}

	kept := []*api.Object{
		{ID: "docs-with-code", Type: "documentation", Capabilities: map[string]map[string]any{"code": {"code_packages": []any{"internal/x"}}}},
		{ID: "docs-with-kind", Type: "documentation", Capabilities: map[string]map[string]any{"code": {"code_packages": []any{}, "kind": "library"}}},
		{ID: "soft", Type: "software-module", Capabilities: map[string]map[string]any{"code": {"code_packages": []any{}}}},
	}
	for _, o := range kept {
		if changed, _ := documentationDropsEmptyCode(o); changed || o.Capabilities["code"] == nil {
			t.Errorf("%s (%s) lost its code group", o.ID, o.Type)
		}
	}
	if !slices.ContainsFunc(migrationSteps, func(s MigrationStep) bool {
		return s.Object != nil && s.Title == "a documentation module drops its empty code group"
	}) {
		t.Error("the step is not shipped")
	}
}
