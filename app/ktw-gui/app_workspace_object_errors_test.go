// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The Errors list reads ObjectErrorList whole — every object in error with its
// errors, and the files CWM could not read — through the JSON the wire carries.
func TestObjectErrorListBinding(t *testing.T) {
	reply := cwmapi.ObjectErrorListOutput{
		Objects: []cwmapi.ObjectErrors{{ID: "live.us-1", Module: "live", Type: "feature", Name: "Invoice run",
			Path:   "modules/live/objects/us-1.cwm.yaml",
			Errors: []cwmapi.ObjectDiagnostic{{Code: cwmapi.DiagnosticDanglingLink, Label: "realizes → live.g-9 does not exist"}}}},
		Unreadable: []cwmapi.InvalidObject{{Path: "modules/live/objects/bad.cwm.yaml", Problems: []string{"unreadable: no id"}}},
		Total:      2,
	}
	s := &stubSender{t: t, reply: reply}
	got, err := objectErrorListVia(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if s.path != "ktw-agents:cwm/object-error-list" {
		t.Errorf("called %q", s.path)
	}
	if got.Total != 2 || len(got.Objects) != 1 || got.Objects[0].Title != "Invoice run" ||
		len(got.Objects[0].Errors) != 1 || got.Objects[0].Errors[0].Code != cwmapi.DiagnosticDanglingLink ||
		len(got.Unreadable) != 1 || got.Unreadable[0].Path != "modules/live/objects/bad.cwm.yaml" {
		t.Fatalf("errors = %+v", got)
	}
}

// A row CWM found in error is marked, so the tree draws it red.
func TestObjectsTreeBinding_MarksObjectsInError(t *testing.T) {
	reply := cwmapi.ObjectTreeOutput{Total: 2, Roots: []cwmapi.ObjectTreeNode{{
		Object:   obj("live", "module", "", "live"),
		Children: []cwmapi.ObjectTreeNode{{Object: obj("live.us-1", "feature", "Invoice run", ""), HasErrors: true}},
	}}}
	rows, err := objectsTreeVia(context.Background(), &stubSender{t: t, reply: reply})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].HasErrors || !rows[1].HasErrors {
		t.Fatalf("HasErrors: live=%v us-1=%v", rows[0].HasErrors, rows[1].HasErrors)
	}
}

// A row CWM marks as a system module (or in one) keeps the mark, so the
// frontend shows it to experts only.
func TestObjectsTreeBinding_KeepsTheSystemMark(t *testing.T) {
	reply := cwmapi.ObjectTreeOutput{Total: 2, Roots: []cwmapi.ObjectTreeNode{
		{Object: obj("live", "module", "", "live")},
		{Object: obj("sys-inbox", "system-module", "", "sys-inbox"), System: true},
	}}
	rows, err := objectsTreeVia(context.Background(), &stubSender{t: t, reply: reply})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].System || !rows[1].System {
		t.Fatalf("System: live=%v sys-inbox=%v", rows[0].System, rows[1].System)
	}
}
