// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"testing"
	"testing/fstest"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The module's status counts what its subtree holds, against a real CWM: a
// use-case filed under a goal is the module's, and a decision authored
// with the shipped decision workflow is open until its lifecycle moves on —
// it carries no `state` field, its state is its lifecycle's.
func TestTheManifestReadsTheModulesSubtreeAndTheDecisionLifecycle(t *testing.T) {
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(), cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":   {Data: []byte("name: module\ncapabilities: [container, module]\n")},
		"object-types/goal.yaml":     {Data: []byte("name: goal\nfolder: goals\ntree: node\n")},
		"object-types/use-case.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-types/use-case.yaml")},
		"object-types/decision.yaml": {Data: []byte("name: decision\nfolder: decisions\nworkflow: decision\ncapabilities: [lifecycle]\n")},
		"workflows/manual.yaml":      {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/manual.yaml")},
		"workflows/decision.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/decision.yaml")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")
	create := func(o cwmapi.Object) string {
		t.Helper()
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
		return o.ID
	}
	g := create(cwmapi.NewContainerObject("m", "g1", "goal", "m", "A goal", ""))
	create(cwmapi.NewContainerObject("m", "UC-001", "use-case", "m", "At the root", ""))
	create(cwmapi.NewContainerObject("m", "UC-002", "use-case", g, "Under the goal", ""))
	create(cwmapi.NewContainerObject("m", "OD-001", "decision", "m", "Still open", ""))
	answered := create(cwmapi.NewContainerObject("m", "OD-002", "decision", "m", "Answered", ""))
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: answered, To: "applies", By: cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}}); err != nil {
		t.Fatalf("answer OD-002: %v", err)
	}

	in, err := LoadManifestInputs(ctx, cwm, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(in.UseCases) != 2 {
		t.Errorf("use-cases = %+v, want both — the one under the goal too", in.UseCases)
	}
	states := map[string]gcapi.ODState{}
	for _, od := range in.OpenDecisions {
		states[od.ODID] = od.State
	}
	if states["OD-001"] != gcapi.ODStateOpen || states["OD-002"] != gcapi.ODStateResolved {
		t.Errorf("decision states = %v, want OD-001 open and OD-002 resolved", states)
	}
	m := BuildManifest(in)
	if len(m.OpenDecisions) != 1 || m.OpenDecisions[0].ID != "OD-001" || len(m.ResolvedDecisions) != 1 {
		t.Errorf("manifest decisions: open %+v resolved %+v", m.OpenDecisions, m.ResolvedDecisions)
	}
}
