// SPDX-License-Identifier: Apache-2.0

package cwmapp_test

import (
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// TestTwoApplicationsSeeTheirOwnPacks: two applications in one process, each
// with its own pack, see each their own declarations — the second one's type,
// its workflow and its id prefix, although the first ran without them.
func TestTwoApplicationsSeeTheirOwnPacks(t *testing.T) {
	// The first application: a pack without the goal type.
	ctx1, cwm1 := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/note.yaml": {Data: []byte("name: note\ntree: node\n")},
	}))
	if _, err := cwm1.ObjectTypeGet(ctx1, cwmapi.ObjectTypeGetInput{Name: "note"}); err != nil {
		t.Fatalf("first app: %v", err)
	}

	// The second: a pack declaring goal, its workflow and its prefix.
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/goal.yaml": {Data: []byte("name: goal\nid_prefix: G\ncapabilities: [lifecycle]\nworkflow: flow\n")},
		"workflows/flow.yaml": {Data: []byte("name: flow\ninitial: open\nstates:\n  - {name: open}\n  - {name: shut}\n" +
			"transitions:\n  - {name: shut, from: [open], to: shut}\n")},
	}))
	got, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: "goal"})
	if err != nil || got.ObjectType.Workflow != "flow" {
		t.Fatalf("second app's goal type: %+v %v", got.ObjectType, err)
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: cwmapi.Object{Type: "goal", Name: "g"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Object.ID != "m.G-001" {
		t.Errorf("id %q: the type's prefix was not used", out.Object.ID)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{
		ID: out.Object.ID, To: "shut", By: cwmapi.Actor{ID: "p", Kind: cwmapi.ActorPerson},
	}); err != nil {
		t.Fatalf("transition: %v", err)
	}
}
