// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/goals/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// searchRecorder is CWM recording every search it is asked.
type searchRecorder struct {
	cwmapi.CWMService
	searches []cwmapi.ObjectSearchInput
	lists    []cwmapi.ObjectListInput
}

func (r *searchRecorder) ObjectList(ctx context.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	r.lists = append(r.lists, in)
	return r.CWMService.ObjectList(ctx, in)
}

func (r *searchRecorder) ObjectSearch(ctx context.Context, in cwmapi.ObjectSearchInput) (cwmapi.ObjectSearchOutput, error) {
	r.searches = append(r.searches, in)
	return r.CWMService.ObjectSearch(ctx, in)
}

// THE GOALS BOARD ASKS THE INDEX FOR GOALS ONLY. goals runs
// in ktw-methods and reaches CWM over the bus: a board searching the whole
// subtree and keeping the goals afterwards ships a project's every object -
// 22 MB on the dev workspace, refused by NATS. Every search the board runs
// carries Types [goal], and another type under the object is never read; the
// goals' parents outside the rows are read by id, never by listing a whole
// module (object-pane rowsOf).
func TestTheGoalsBoardSearchesTheGoalsOnly(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/goal.yaml":  {Data: []byte("name: goal\ncapabilities: [want]\n")},
		"object-types/thing.yaml": {Data: []byte("name: thing\ntree: node\n")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")
	for _, o := range []cwmapi.Object{
		{ID: "m.G-1", Type: api.GoalType, Name: "A goal", Parent: "m"},
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
	if !strings.Contains(string(raw), "A goal") {
		t.Errorf("the board lacks its goal: %s", raw)
	}
	if len(rec.searches) == 0 {
		t.Fatal("the board ran no search")
	}
	for _, s := range rec.searches {
		if !slices.Equal(s.Types, []string{api.GoalType}) {
			t.Errorf("the board searched %+v: its types are not [goal] - the whole subtree comes back over the bus", s)
		}
	}
	for _, l := range rec.lists {
		if len(l.Fields) == 0 {
			t.Errorf("the board listed %+v whole to name a parent - the module's every object comes back over the bus", l)
		}
	}
}
