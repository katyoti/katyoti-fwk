// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// searchRecorder is CWM recording every search it is asked.
type searchRecorder struct {
	cwmapi.CWMService
	searches []cwmapi.ObjectSearchInput
}

func (r *searchRecorder) ObjectSearch(ctx context.Context, in cwmapi.ObjectSearchInput) (cwmapi.ObjectSearchOutput, error) {
	r.searches = append(r.searches, in)
	return r.CWMService.ObjectSearch(ctx, in)
}

// THE FEATURES BOARD ASKS THE INDEX FOR FEATURES ONLY.
// features runs in ktw-methods and reaches CWM over the bus: a board
// searching the whole subtree and keeping the features afterwards ships a
// project's every object - 22 MB on the dev workspace, refused by NATS. The
// board's search carries Types [feature], the names of the goals served are
// read with Types [goal], and no search runs without a type.
func TestTheFeaturesBoardSearchesTheFeaturesOnly(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"capabilities/feature.yaml": {Data: []byte("name: feature\n")},
		"object-types/feature.yaml": {Data: []byte("name: feature\ncapabilities: [feature]\n")},
		"object-types/goal.yaml":    {Data: []byte("name: goal\n")},
		"object-types/thing.yaml":   {Data: []byte("name: thing\ntree: node\n")},
		"edge-types/serves.yaml":    {Data: []byte("name: serves\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")
	for _, o := range []cwmapi.Object{
		{ID: "m.G-1", Type: goalType, Name: "A goal", Parent: "m"},
		{ID: "m.F-1", Type: api.FeatureType, Name: "A feature", Parent: "m",
			Edges: []cwmapi.ObjectEdge{{Kind: "serves", To: "m.G-1"}}},
		{ID: "m.T-1", Type: "thing", Name: "A thing", Parent: "m"},
	} {
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	rec := &searchRecorder{CWMService: cwm}
	d, err := board(ctx, rec, api.BoardInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(d)
	for _, want := range []string{"A feature", "A goal"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the board lacks %q: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "A thing") {
		t.Errorf("the board shows another type: %s", raw)
	}
	if len(rec.searches) == 0 {
		t.Fatal("the board ran no search")
	}
	sawFeatures := false
	for _, s := range rec.searches {
		switch {
		case len(s.Types) == 1 && s.Types[0] == api.FeatureType:
			sawFeatures = true
		case len(s.Types) == 1 && s.Types[0] == goalType:
		default:
			t.Errorf("the board searched %+v: its types are not [feature] or [goal] - the whole subtree comes back over the bus", s)
		}
	}
	if !sawFeatures {
		t.Error("the board never searched Types [feature]")
	}
}
