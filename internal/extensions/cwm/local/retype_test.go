// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : object-retype changes an object's type keeping its id, its file
// and its edges; it refuses an unknown type, a change of level, and a retype
// while the object holds fields the new type does not shape the same way.
func TestObjectRetypeKeepsTheObjectAndRefusesToDropData(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":          {Data: []byte("name: module\ncapabilities: [module]\nallowed_children: ['*']\n")},
		"object-types/software-module.yaml": {Data: []byte("name: software-module\ncapabilities: [module]\nallowed_children: ['*']\n")},
		"object-types/note.yaml":            {Data: []byte("name: note\n")},
		"object-types/memo.yaml":            {Data: []byte("name: memo\n")},
	}))
	for _, o := range []cwmapi.Object{
		{ID: "billing", Type: "module", Name: "Billing", Edges: []cwmapi.ObjectEdge{{Kind: "relates-to", To: "billing.n1"}}},
		{ID: "billing.n1", Type: "note", Parent: "billing", Name: "A note"},
		{ID: "billing.n2", Type: "note", Parent: "billing", Name: "Another", Fields: map[string]any{"due": "2027-01-01"}},
	} {
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	before, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectRetype(ctx, cwmapi.ObjectRetypeInput{ID: "billing", Type: "software-module"})
	if err != nil || got.Previous != "module" || got.Object.Type != "software-module" {
		t.Fatalf("module -> software-module: %+v %v", got, err)
	}
	after, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	if after.Object.ID != "billing" || after.Object.Type != "software-module" || len(after.Object.Edges) != 1 || after.Object.Name != before.Object.Name {
		t.Errorf("after the retype: %+v; want the same id, name and edge under software-module", after.Object)
	}
	for name, in := range map[string]cwmapi.ObjectRetypeInput{
		"an unknown type":      {ID: "billing.n1", Type: "no-such-type"},
		"a change of level":    {ID: "billing.n1", Type: "software-module"},
		"fields the new drops": {ID: "billing.n2", Type: "memo"},
	} {
		if _, err := cwm.ObjectRetype(ctx, in); err == nil {
			t.Errorf("%s: retyped", name)
		}
	}
	if _, err := cwm.ObjectRetype(ctx, cwmapi.ObjectRetypeInput{ID: "billing.n2", Type: "memo"}); err == nil || !strings.Contains(err.Error(), "due") {
		t.Errorf("the refusal names the fields: %v", err)
	}
	if _, err := cwm.ObjectRetype(ctx, cwmapi.ObjectRetypeInput{ID: "billing.n1", Type: "memo"}); err != nil {
		t.Errorf("a note with no fields to a memo: %v", err)
	}
}
