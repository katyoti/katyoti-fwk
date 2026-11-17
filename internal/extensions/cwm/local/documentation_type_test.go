// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : documentation is a module type of its own - the module level
// planned, with no code - holding documents, decisions, goals and features.
func TestDocumentationIsAPlannedModuleType(t *testing.T) {
	ctx, cwm := cwmapp.NewStrict(t, cwmapp.Pack(fstest.MapFS{
		"object-types/documentation.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-types/documentation.yaml")},
		"capabilities/planned.yaml":       {Data: []byte("name: planned\n")},
		"object-types/goal.yaml":          {Data: []byte("name: goal\n")},
		"object-types/use-case.yaml":      {Data: []byte("name: use-case\naccept_parents: [feature]\n")},
	}))
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "handbook", Object: cwmapi.Object{ID: "handbook", Type: "documentation", Name: "User guide"}})
	if err != nil || made.Object.ID != "handbook" {
		t.Fatalf("a documentation module: %+v %v", made.Object, err)
	}
	ty, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: "documentation"})
	if err != nil || ty.ObjectType.Level() != cwmapi.CapModule || !slices.Contains(ty.ObjectType.Capabilities, "planned") || slices.Contains(ty.ObjectType.Capabilities, "code") {
		t.Fatalf("the documentation type: %+v %v; want the module level, planned, no code", ty.ObjectType, err)
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "handbook", Object: cwmapi.Object{ID: "handbook.G-001", Type: "goal", Parent: "handbook", Name: "Users find their way"}}); err != nil {
		t.Errorf("a goal in documentation: %v", err)
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "handbook", Object: cwmapi.Object{ID: "handbook.UC-001", Type: "use-case", Parent: "handbook", Name: "A use case"}}); err == nil {
		t.Error("documentation took a use case directly: it holds documents, decisions, goals and features")
	}
}
