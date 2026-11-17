// SPDX-License-Identifier: Apache-2.0

package core

import (
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a module object's targets edge becomes a takes edge on
// the increment or release it named - a goal, a feature, a use case, a
// decision -, once; a request keeps its targets; a targets edge naming nothing
// stored is left for the guard to name; a second run changes nothing.
func TestMigrationTurnsTargetsIntoTakes(t *testing.T) {
	aim := func(id, typ, to string) *api.Object {
		return &api.Object{ID: id, Type: typ, Name: id, Edges: []api.ObjectEdge{{Kind: "serves", To: "m.G-1"}, {Kind: "targets", To: to}}}
	}
	inc := &api.Object{ID: "roadmap.IN-001", Type: "increment", Name: "inc",
		Edges: []api.ObjectEdge{{Kind: "in-stream", To: "roadmap.SM-001"}, {Kind: "takes", To: "m.G-1"}}}
	rel := &api.Object{ID: "roadmap.RL-001", Type: "release", Name: "rel"}
	objs := []*api.Object{
		aim("m.F-001", "feature", inc.ID),
		aim("m.G-1", "goal", inc.ID), // already taken: not taken twice
		aim("m.AC-001", "use-case", rel.ID),
		aim("m.OD-001", "decision", inc.ID),
		aim("m.RQ-001", "request", inc.ID),
		aim("m.F-002", "feature", "roadmap.IN-099"), // dangling
		inc, rel,
	}
	changed, removed, err := targetsBecomeTakes(objs)
	if err != nil || len(removed) != 0 {
		t.Fatalf("err %v, removed %v", err, removed)
	}
	var ids []string
	for _, i := range changed {
		ids = append(ids, objs[i].ID)
	}
	want := []string{"m.F-001", "m.G-1", "m.AC-001", "m.OD-001", inc.ID, rel.ID}
	if !slices.Equal(ids, want) {
		t.Errorf("changed %v; want %v", ids, want)
	}
	targets := func(o *api.Object) []string {
		var out []string
		for _, e := range o.Edges {
			if e.Kind == "targets" {
				out = append(out, e.To)
			}
		}
		return out
	}
	for _, o := range objs[:4] {
		if got := targets(o); len(got) != 0 {
			t.Errorf("%s keeps its targets %v", o.ID, got)
		}
		if !slices.ContainsFunc(o.Edges, func(e api.ObjectEdge) bool { return e.Kind == "serves" }) {
			t.Errorf("%s lost its other edges: %v", o.ID, o.Edges)
		}
	}
	if got := targets(objs[4]); !slices.Equal(got, []string{inc.ID}) {
		t.Errorf("the request's targets %v; want it untouched", got)
	}
	if got := targets(objs[5]); !slices.Equal(got, []string{"roadmap.IN-099"}) {
		t.Errorf("the dangling targets %v; want it left for the guard", got)
	}
	takes := func(o *api.Object) []string {
		var out []string
		for _, e := range o.Edges {
			if e.Kind == "takes" {
				out = append(out, e.To)
			}
		}
		return out
	}
	if got := takes(inc); !slices.Equal(got, []string{"m.G-1", "m.F-001", "m.OD-001"}) {
		t.Errorf("the increment takes %v", got)
	}
	if got := takes(rel); !slices.Equal(got, []string{"m.AC-001"}) {
		t.Errorf("the release takes %v", got)
	}
	again, _, _ := targetsBecomeTakes(objs)
	if len(again) != 0 {
		t.Errorf("a second run changed %d objects; want none", len(again))
	}
}
