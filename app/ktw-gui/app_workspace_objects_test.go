// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// THE BINDING-LEVEL PARITY TEST FOR THE OBJECTS TREE (phase 5d's pattern, first
// use): a stub sender answers `ktw-agents:cwm/object-tree` with what the core
// returns — marshaled through the SAME JSON the wire carries — and the binding
// must flatten it into the rows the frontend draws: project root first,
// modules beneath it, depth and titles as the page says
// (modules/gui/objects-what-the-user-sees.md §2).

type stubSender struct {
	t     *testing.T
	path  string
	input any
	reply any
}

func (s *stubSender) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	s.path, s.input = path, params
	raw, err := json.Marshal(s.reply)
	if err != nil {
		s.t.Fatalf("stub marshal: %v", err)
	}
	return raw, nil
}

func (s *stubSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

func obj(id, typ, title, name string) cwmapi.Object {
	o := cwmapi.Object{ID: id, Type: typ}
	if name != "" {
		o.Name = name
	}
	if title != "" {
		o.Name = title
	}
	return o
}

func TestObjectsTreeBinding_ProjectRootThenModules(t *testing.T) {
	reply := cwmapi.ObjectTreeOutput{
		Total: 5,
		Roots: []cwmapi.ObjectTreeNode{{
			Object: obj("ktw-acme-demo", "project", "Acme demo", "ktw-acme-demo"),
			Children: []cwmapi.ObjectTreeNode{
				{Object: obj("live", "module", "", "live"), Children: []cwmapi.ObjectTreeNode{
					{Object: obj("live.us-1", "feature", "Invoice run", "")},
				}},
				{Object: obj("billing", "module", "", "billing")},
				{Object: obj("live.orphan", "record", "", "")}, // neither title nor name: the id
			},
		}},
	}
	s := &stubSender{t: t, reply: reply}
	rows, err := objectsTreeVia(context.Background(), s)
	if err != nil {
		t.Fatalf("objectsTreeVia: %v", err)
	}
	if s.path != "ktw-agents:cwm/object-tree" {
		t.Errorf("called %q, want ktw-agents:cwm/object-tree", s.path)
	}
	if in, ok := s.input.(cwmapi.ObjectTreeInput); !ok || !in.DrawnOnly {
		t.Errorf("input = %#v, want ObjectTreeInput{DrawnOnly: true} — drawn-ness is CWM's answer, not the GUI's", s.input)
	}

	var got []string
	for _, r := range rows {
		got = append(got, strings.Repeat("  ", r.Depth)+r.Title+" ("+r.ID+"/"+r.Type+"/"+r.Module+")")
	}
	// A bare id IS its module (a module object's id is the module name) — so
	// the module chip of `live` reads `live`, and the project's reads its own
	// name. The substrate's rule, pinned here so a change to it shows up in
	// the one place the GUI depends on it.
	want := []string{
		"Acme demo (ktw-acme-demo/project/ktw-acme-demo)",
		"  live (live/module/live)",
		"    Invoice run (live.us-1/feature/live)",
		"  billing (billing/module/billing)",
		"  live.orphan (live.orphan/record/live)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !rows[0].HasChildren || !rows[1].HasChildren || rows[2].HasChildren || rows[3].HasChildren {
		t.Errorf("HasChildren: project=%v live=%v us-1=%v billing=%v", rows[0].HasChildren, rows[1].HasChildren, rows[2].HasChildren, rows[3].HasChildren)
	}
}

// An empty universe is an empty list, not an error — and a sender failure is
// an error, never an empty list that looks like "no objects".
func TestObjectsTreeBinding_EmptyAndFailure(t *testing.T) {
	rows, err := objectsTreeVia(context.Background(), &stubSender{t: t, reply: cwmapi.ObjectTreeOutput{}})
	if err != nil || len(rows) != 0 {
		t.Errorf("empty tree: rows=%v err=%v", rows, err)
	}
	if _, err := objectsTreeVia(context.Background(), failingSender{}); err == nil {
		t.Errorf("a failing call must surface as an error, not as an empty tree")
	}
}

type failingSender struct{}

func (failingSender) Call(context.Context, string, any, ...ktsapi.CallOption) (json.RawMessage, error) {
	return nil, context.DeadlineExceeded
}
func (failingSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

// The bound method hands the failure to the frontend: a core it cannot reach
// rejects the call (the tree draws the error), never resolves to an empty
// list the tree would draw as "No objects."
func TestObjectsTreeBinding_ReturnsTheFailure(t *testing.T) {
	a := &App{} // no orchestrator: the universe is unreachable
	rows, err := a.WorkspaceObjectsTree("u")
	if err == nil || rows != nil {
		t.Fatalf("an unreachable universe: rows=%v err=%v, want an error", rows, err)
	}
	if !strings.Contains(err.Error(), "objects tree of \"u\"") {
		t.Errorf("the error names the tree and the universe: %v", err)
	}
}
