// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// twoProjectsCWM answers a search as the store set does when the universe
// serves two projects: each object's project in Projects, the states keyed by
// the qualified id - and both projects hold the bare id.
type twoProjectsCWM struct {
	cwmapi.CWMService
}

func (twoProjectsCWM) ObjectSearch(context.Context, cwmapi.ObjectSearchInput) (cwmapi.ObjectSearchOutput, error) {
	return cwmapi.ObjectSearchOutput{
		Objects: []cwmapi.Object{
			{ID: "m.F-001", Type: "feature", Name: "Here"},
			{ID: "m.F-001", Type: "feature", Name: "There"},
			{ID: "m.F-002", Type: "feature", Name: "Only here"},
		},
		Projects: []string{"alpha", "beta", "alpha"},
		States: map[string]cwmapi.ObjectState{
			"alpha:m.F-001": {State: "done", Color: "green"},
			"beta:m.F-001":  {State: "idea", Color: "gray"},
			"alpha:m.F-002": {State: "realized", Color: "teal"},
		},
		Total: 3,
	}, nil
}

func (twoProjectsCWM) ObjectTypeGet(context.Context, cwmapi.ObjectTypeGetInput) (cwmapi.ObjectTypeGetOutput, error) {
	return cwmapi.ObjectTypeGetOutput{}, nil
}

// On a universe serving two repositories, every row keeps its own object's
// state and colour: rows read the states by the qualified id the store set
// keys them by, so two projects holding one bare id never swap or lose them.
func TestRowsKeepTheirStateOnTwoProjects(t *testing.T) {
	ctx := context.Background()
	want := []struct{ name, state, color string }{
		{"Here", "done", "green"},
		{"There", "idea", "gray"},
		{"Only here", "realized", "teal"},
	}
	rows, err := ReadRows(ctx, twoProjectsCWM{}, cwmapi.ObjectSearchInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(want) {
		t.Fatalf("ReadRows: %d rows, want %d", len(rows), len(want))
	}
	for i, w := range want {
		if r := rows[i]; r.Name != w.name || r.State != w.state || r.StateColor != w.color {
			t.Errorf("ReadRows row %d: %q %q/%q, want %q %q/%q", i, r.Name, r.State, r.StateColor, w.name, w.state, w.color)
		}
	}

	of, err := ReadRowsOf(ctx, twoProjectsCWM{}, []string{"m.F-002", "m.F-001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(of) != 3 || of[0].State != "realized" || of[0].StateColor != "teal" || of[1].State != "done" || of[1].StateColor != "green" ||
		of[2].State != "idea" || of[2].StateColor != "gray" {
		t.Errorf("ReadRowsOf: %+v, want m.F-002 realized/teal then m.F-001 done/green and idea/gray (each project's)", of)
	}
}

// homonymCWM serves alpha and beta, both holding the goal, each with a
// sub-goal of its own (alpha, beta); with one project (single) it
// answers alpha's objects alone, no project named.
type homonymCWM struct {
	cwmapi.CWMService
	single bool
}

func (c homonymCWM) ObjectSearch(_ context.Context, in cwmapi.ObjectSearchInput) (cwmapi.ObjectSearchOutput, error) {
	if c.single {
		return cwmapi.ObjectSearchOutput{Objects: []cwmapi.Object{
			{ID: "m.G-2", Type: "goal", Name: "alpha sub-goal", Parent: "m.G-1"},
		}}, nil
	}
	return cwmapi.ObjectSearchOutput{
		Objects: []cwmapi.Object{
			{ID: "m.G-2", Type: "goal", Name: "alpha sub-goal", Parent: "m.G-1"},
			{ID: "m.G-3", Type: "goal", Name: "beta sub-goal", Parent: "m.G-1"},
			{ID: "m.G-1", Type: "goal", Name: "alpha goal", Parent: "m"},
			{ID: "m.G-1", Type: "goal", Name: "beta goal", Parent: "m"},
		},
		Projects: []string{"alpha", "beta", "alpha", "beta"},
	}, nil
}

// ObjectList lists module m as the store set does: the parents of both
// projects, beta's last (a bare-keyed map would keep or take the wrong one).
func (c homonymCWM) ObjectList(context.Context, cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	if c.single {
		return cwmapi.ObjectListOutput{Objects: []cwmapi.Object{{ID: "m.G-1", Type: "goal", Name: "alpha goal"}}}, nil
	}
	return cwmapi.ObjectListOutput{
		Objects: []cwmapi.Object{
			{ID: "m.G-1", Type: "goal", Name: "alpha goal"},
			{ID: "m.G-1", Type: "goal", Name: "beta goal"},
		},
		Projects: []string{"alpha", "beta"},
	}, nil
}

// ObjectGet reads one parent by its id, as the store set does: a qualified
// id answers its project's object; with one project the bare id alpha's.
func (c homonymCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	switch {
	case in.ID == "beta:m.G-1":
		return cwmapi.ObjectGetOutput{Object: cwmapi.Object{ID: "m.G-1", Type: "goal", Name: "beta goal"}, Project: "beta"}, nil
	case in.ID == "alpha:m.G-1" || (c.single && in.ID == "m.G-1"):
		return cwmapi.ObjectGetOutput{Object: cwmapi.Object{ID: "m.G-1", Type: "goal", Name: "alpha goal"}, Project: "alpha"}, nil
	}
	return cwmapi.ObjectGetOutput{}, cwmapi.ErrNotFound
}

func (homonymCWM) ObjectTypeGet(context.Context, cwmapi.ObjectTypeGetInput) (cwmapi.ObjectTypeGetOutput, error) {
	return cwmapi.ObjectTypeGetOutput{}, nil
}

func navigations(t *testing.T, n render.Node) string {
	t.Helper()
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A ROW NAMES AND OPENS ITS OWN PROJECT'S OBJECTS. On a
// universe serving alpha and beta, both holding: alpha's sub-goal reads
// its parent as alpha's goal (never beta's, the last listed) and its links
// open alpha: and alpha: - a bare id held by two projects is refused
// by the store set. A qualified id given to ReadRowsOf answers that project's
// object, a bare one each project's. One project: rows and links unchanged.
func TestRowsNameAndOpenTheirOwnProjectsObjects(t *testing.T) {
	ctx := context.Background()
	two := homonymCWM{}
	rows, err := ReadRows(ctx, two, cwmapi.ObjectSearchInput{Modules: []string{"m"}})
	if err != nil {
		t.Fatal(err)
	}
	// The parents among the rows: named from the rows themselves.
	if len(rows) != 4 || rows[0].ParentName != "alpha goal" || rows[1].ParentName != "beta goal" {
		t.Errorf("ReadRows: %+v, want alpha's sub-goal under alpha goal, beta's under beta goal", rows)
	}
	// The parents left out, as a board of the sub-goals does: named from ObjectList.
	subs, err := ReadRowsOf(ctx, two, []string{"alpha:m.G-2", "beta:m.G-3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("ReadRowsOf(alpha:m.G-2, beta:m.G-3): %d rows, want 2: %+v", len(subs), subs)
	}
	for _, c := range []struct{ name, project, parent, self, up string }{
		{"alpha sub-goal", "alpha", "alpha goal", "alpha:m.G-2", "alpha:m.G-1"},
		{"beta sub-goal", "beta", "beta goal", "beta:m.G-3", "beta:m.G-1"},
	} {
		var r *ObjectRow
		for i := range subs {
			if subs[i].Name == c.name {
				r = &subs[i]
			}
		}
		if r == nil {
			t.Fatalf("no row %q in %+v", c.name, subs)
		}
		if r.Project != c.project || r.ParentName != c.parent {
			t.Errorf("%s: project %q parent %q, want %q %q", c.name, r.Project, r.ParentName, c.project, c.parent)
		}
		page := navigations(t, RowNode(*r))
		for _, id := range []string{c.self, c.up} {
			if !strings.Contains(page, `"object":"`+id+`"`) {
				t.Errorf("%s: no link opening %s in %s", c.name, id, page)
			}
		}
	}

	// A qualified id keeps its project's object; a bare one each project's.
	if got, err := ReadRowsOf(ctx, two, []string{"beta:m.G-1"}); err != nil || len(got) != 1 || got[0].Name != "beta goal" {
		t.Errorf("ReadRowsOf(beta:m.G-1) = %+v, %v; want beta goal alone", got, err)
	}
	if got, err := ReadRowsOf(ctx, two, []string{"m.G-1"}); err != nil || len(got) != 2 || got[0].Name != "alpha goal" || got[1].Name != "beta goal" {
		t.Errorf("ReadRowsOf(m.G-1) = %+v, %v; want alpha goal then beta goal", got, err)
	}

	// One project: bare ids everywhere.
	one, err := ReadRows(ctx, homonymCWM{single: true}, cwmapi.ObjectSearchInput{})
	if err != nil || len(one) != 1 {
		t.Fatalf("one project: %+v, %v", one, err)
	}
	if r := one[0]; r.Project != "" || r.ParentName != "alpha goal" {
		t.Errorf("one project: project %q parent %q, want none and alpha goal", r.Project, r.ParentName)
	}
	page := navigations(t, RowNode(one[0]))
	if !strings.Contains(page, `"object":"m.G-2"`) || !strings.Contains(page, `"object":"m.G-1"`) || strings.Contains(page, `:m.G-`) {
		t.Errorf("one project: links not bare: %s", page)
	}
}

// parentsCWM answers a search of objs and records how their parents are read:
// each ObjectGet's id, each ObjectList's input.
type parentsCWM struct {
	cwmapi.CWMService
	objs  []cwmapi.Object
	gets  []string
	lists []cwmapi.ObjectListInput
}

func (c *parentsCWM) ObjectSearch(context.Context, cwmapi.ObjectSearchInput) (cwmapi.ObjectSearchOutput, error) {
	return cwmapi.ObjectSearchOutput{Objects: c.objs}, nil
}

func (c *parentsCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	c.gets = append(c.gets, in.ID)
	return cwmapi.ObjectGetOutput{Object: cwmapi.Object{ID: in.ID, Name: "name of " + in.ID}}, nil
}

func (c *parentsCWM) ObjectList(_ context.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	c.lists = append(c.lists, in)
	var out []cwmapi.Object
	for i := 0; i <= ParentsByID+1; i++ {
		out = append(out, cwmapi.Object{ID: fmt.Sprintf("n.F-%d", i), Name: fmt.Sprintf("listed n.F-%d", i)})
	}
	return cwmapi.ObjectListOutput{Objects: out}, nil
}

func (*parentsCWM) ObjectTypeGet(context.Context, cwmapi.ObjectTypeGetInput) (cwmapi.ObjectTypeGetOutput, error) {
	return cwmapi.ObjectTypeGetOutput{}, nil
}

// A LIST NAMES THE PARENTS OUTSIDE IT WITHOUT A MODULE'S WHOLE OBJECTS
// . The goals and features boards search one type and draw
// its rows over the bus: listing a parent's whole module to name it shipped
// 64 modules for a project's Goals tab (planification 5.2 MB of NATS's
// 8 MiB). A parent outside the rows is read by its id, once; a module holding
// more than ParentsByID of them is listed by names only - never an ObjectList
// without Fields.
func TestRowsNameTheirParentsByIDNotByWholeModules(t *testing.T) {
	ctx := context.Background()
	few := &parentsCWM{objs: []cwmapi.Object{
		{ID: "m.G-1", Type: "goal", Name: "one", Parent: "m"},
		{ID: "m.G-2", Type: "goal", Name: "two", Parent: "m"},
		{ID: "m.G-3", Type: "goal", Name: "three", Parent: "m.G-1"},
		{ID: "k.G-1", Type: "goal", Name: "four", Parent: "k.F-7"},
	}}
	rows, err := ReadRows(ctx, few, cwmapi.ObjectSearchInput{Types: []string{"goal"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(few.gets, []string{"m", "k.F-7"}) || len(few.lists) != 0 {
		t.Errorf("parents read by gets %v and lists %+v; want ObjectGet of m and k.F-7 alone (m.G-1 is a row)", few.gets, few.lists)
	}
	if rows[0].ParentName != "name of m" || rows[2].ParentName != "one" || rows[3].ParentName != "name of k.F-7" {
		t.Errorf("parent names: %q %q %q", rows[0].ParentName, rows[2].ParentName, rows[3].ParentName)
	}

	many := &parentsCWM{}
	for i := 0; i <= ParentsByID; i++ {
		many.objs = append(many.objs, cwmapi.Object{ID: fmt.Sprintf("m.G-%d", i), Type: "goal", Parent: fmt.Sprintf("n.F-%d", i)})
	}
	rows, err = ReadRows(ctx, many, cwmapi.ObjectSearchInput{Types: []string{"goal"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(many.gets) != 0 || len(many.lists) != 1 || many.lists[0].Module != "n" ||
		!slices.Equal(many.lists[0].Fields, []string{"name"}) || many.lists[0].Limit >= 0 {
		t.Errorf("%d parents in one module read by gets %v and lists %+v; want one list of n's names (Fields [name], every object)", ParentsByID+1, many.gets, many.lists)
	}
	if rows[0].ParentName != "listed n.F-0" {
		t.Errorf("parent name %q, want listed n.F-0", rows[0].ParentName)
	}
}
