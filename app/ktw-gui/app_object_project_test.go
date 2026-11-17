// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"
)

// : the bridge reads the project of each object in either answer
// form - one store (no key: today's screens) or cwm's store set (each object
// located in its project) - for the tree, a search and an object's detail.
func TestTheBridgeReadsEachObjectsProjectInEitherAnswerForm(t *testing.T) {
	// A search: one store answers its objects alone, the store set names each
	// one's project in the parallel projects list.
	bare := []byte(`{"objects":[{"id":"crm.F-001","type":"feature","name":"Login"}],"total":1}`)
	got, err := locatedObjects(bare)
	if err != nil || len(got) != 1 || got[0].Object.ID != "crm.F-001" || got[0].Project != "" {
		t.Fatalf("one store's answer: %+v, %v - want crm.F-001 with no project", got, err)
	}
	joined := []byte(`{"objects":[
		{"id":"crm.F-001","type":"feature","name":"Login"},
		{"id":"crm.F-001","type":"feature","name":"Login too"}],"projects":["erp","crm"],"total":2}`)
	got, err = locatedObjects(joined)
	if err != nil || len(got) != 2 {
		t.Fatalf("the store set's answer: %+v, %v", got, err)
	}
	if got[0].Project != "erp" || got[1].Project != "crm" || got[1].Object.ID != "crm.F-001" || got[1].Object.DisplayTitle() != "Login too" {
		t.Fatalf("each object gets the project at its position: %+v", got)
	}
	// A projects list that does not match the objects one for one cannot be
	// read by position: no object is marked.
	got, err = locatedObjects([]byte(`{"objects":[{"id":"a.F-1"},{"id":"b.F-1"}],"projects":["erp"],"total":2}`))
	if err != nil || len(got) != 2 || got[0].Project != "" || got[1].Project != "" {
		t.Fatalf("a mismatched projects list marks no object: %+v, %v", got, err)
	}

	// The tree: a node's project is its own key, else its root's.
	tree := []byte(`{"roots":[
		{"object":{"id":"erp.root"},"project":"erp","children":[{"object":{"id":"erp.F-1"}}]},
		{"object":{"id":"crm.root"},"project":"crm","children":[{"object":{"id":"crm.F-1"},"children":[{"object":{"id":"crm.F-2"}}]}]}],"total":5}`)
	if p := treeProjects(tree); len(p) != 5 || p[0] != "erp" || p[1] != "erp" || p[2] != "crm" || p[3] != "crm" || p[4] != "crm" {
		t.Fatalf("tree projects depth first = %v, want [erp erp crm crm crm]", p)
	}
	if p := treeProjects([]byte(`{"roots":[{"object":{"id":"a.b"}}],"total":1}`)); len(p) != 1 || p[0] != "" {
		t.Fatalf("one store's tree names no project: %v", p)
	}

	// The tree through the binding: each flattened row carries its project.
	s := &multiStub{t: t, replies: map[string]any{"ktw-agents:cwm/object-tree": json.RawMessage(tree)}}
	rows, err := objectsTreeVia(t.Context(), s)
	if err != nil || len(rows) != 5 {
		t.Fatalf("objectsTreeVia: %+v, %v", rows, err)
	}
	if rows[0].Project != "erp" || rows[4].ID != "crm.F-2" || rows[4].Project != "crm" {
		t.Fatalf("each tree row carries its project: %+v", rows)
	}

	// A search through the binding: each found object carries its project.
	s = &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-search": json.RawMessage(joined),
		"ktw-agents:cwm/object-list":   json.RawMessage(`{"objects":[],"total":0}`),
	}}
	found, err := objectFindVia(t.Context(), s, "login")
	if err != nil || len(found.Objects) != 2 || found.Objects[0].Project != "erp" || found.Objects[1].Project != "crm" {
		t.Fatalf("objectFindVia: %+v, %v - want both projects' objects, each with its project", found, err)
	}

	// An object's detail: the key beside its object, or none.
	if p := projectOf([]byte(`{"project":"crm","object":{"id":"crm.F-1"}}`)); p != "crm" {
		t.Fatalf("detail project = %q, want crm", p)
	}
	if p := projectOf([]byte(`{"object":{"id":"crm.F-1"}}`)); p != "" {
		t.Fatalf("one store's detail names no project: %q", p)
	}
}
