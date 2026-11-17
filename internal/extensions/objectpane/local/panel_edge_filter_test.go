// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A PANEL ON THE TARGET OF AN EDGE: `incoming: [kind]` shows a panel on the
// object an edge of that kind points at, `outgoing: [kind]` on the object it
// leaves — each list any-of, ANDed with the rest of the filter. A filter key no
// field takes skips that panel only, never the others.
func TestAPanelSelectsByIncomingAndOutgoingEdge(t *testing.T) {
	const edge = "from: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n"
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml":       "name: thing\ntree: node\n",
		"object-types/note.yaml":        "name: note\ntree: node\n",
		"edge-types/challenges.yaml":    "name: challenges\n" + edge,
		"edge-types/refines.yaml":       "name: refines\n" + edge,
		"object-panels/challenged.yaml": "name: Challenges\nfilter: {incoming: [challenges]}\n" + ext,
		"object-panels/any-of.yaml":     "name: AnyOf\nfilter: {incoming: [refines, challenges]}\n" + ext,
		"object-panels/challenger.yaml": "name: Challenger\nfilter: {outgoing: [challenges]}\n" + ext,
		"object-panels/and-type.yaml":   "name: AndType\nfilter: {types: [note], incoming: [challenges]}\n" + ext,
		"object-panels/refined.yaml":    "name: Refined\nfilter: {incoming: [refines]}\n" + ext,
		"object-panels/misspelt.yaml":   "name: Misspelt\nfilter: {incomming: [challenges]}\n" + ext,
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.target", Type: "thing"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.lone", Type: "thing"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.c", Type: "thing", Edges: []cwmapi.ObjectEdge{{Kind: "challenges", To: "m.target"}}})

	strip := func(id string) map[string]bool {
		out := map[string]bool{}
		for p := range panelByID(detail(t, ctx, svc, id, true).Panels) {
			out[p] = true
		}
		return out
	}
	target, lone, challenger := strip("m.target"), strip("m.lone"), strip("m.c")

	for _, c := range []struct {
		panel, on string
		strip     map[string]bool
		want      bool
	}{
		{"tab:challenged", "the challenged object", target, true},
		{"tab:any-of", "the challenged object (any of the kinds)", target, true},
		{"tab:challenged", "an object nothing points at", lone, false},
		{"tab:challenged", "the challenger (its edge is outgoing)", challenger, false},
		{"tab:challenger", "the challenger", challenger, true},
		{"tab:challenger", "the challenged object (its edge is incoming)", target, false},
		{"tab:challenger", "an object with no edge", lone, false},
		{"tab:and-type", "a thing (types ANDed)", target, false},
		{"tab:refined", "an object challenged but not refined", target, false},
		{"tab:misspelt", "any object (a misspelt key is refused, never an empty filter)", target, false},
	} {
		if c.strip[c.panel] != c.want {
			t.Errorf("%s on %s: shown=%v, want %v", c.panel, c.on, c.strip[c.panel], c.want)
		}
	}
}
