// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : the level a type sits at is the level capability it carries
// never its name - a module type named anything (documentation, a study)
// reads as a module, a project type named anything as a project, and cwm's
// own readers ask the capability: an object of a module-level type keeps the
// bare id a module has, whatever its type is called.
func TestTheLevelIsACapabilityNotATypeName(t *testing.T) {
	for _, c := range []struct {
		caps []string
		want string
	}{
		{[]string{"module", "lifecycle"}, api.CapModule},
		{[]string{"project"}, api.CapProject},
		{[]string{"lifecycle"}, ""},
		{nil, ""},
	} {
		if got := (api.ObjectType{Name: "anything", Capabilities: c.caps}).Level(); got != c.want {
			t.Errorf("a type carrying %v: level %q, want %q", c.caps, got, c.want)
		}
	}

	svc, _ := newSvc(t)
	ctx := stdctx.Background()
	svc.mergeObjectTypes([]api.ObjectType{
		{Name: "documentation", Capabilities: []string{api.CapModule}},
		{Name: "note"},
	}, nil)
	made, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "demo", Object: api.Object{ID: "handbook", Type: "documentation"}})
	if err != nil {
		t.Fatal(err)
	}
	if made.Object.ID != "handbook" {
		t.Errorf("an object of a module-level type named documentation got id %q, want the bare id a module keeps", made.Object.ID)
	}
	made, err = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "demo", Object: api.Object{ID: "n1", Type: "note"}})
	if err != nil {
		t.Fatal(err)
	}
	if made.Object.ID != "demo.n1" {
		t.Errorf("an object below the module level got id %q, want it qualified by its module", made.Object.ID)
	}
}
