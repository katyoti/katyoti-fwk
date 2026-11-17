// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// System modules are shown to experts only, decided from data:
// CWM's tree marks a system module and what lies in it (the system-module
// capability, never the name) - the GUI's tree and pickers leave those out
// below Expert - and the Objects search leaves their rows out below Expert.
func TestSystemModulesShowAtExpertOnly(t *testing.T) {
	ctx, cwm, svc, _ := world(t)
	if _, err := cwmapi.EnsureSystemModule(ctx, cwm, "sys-ops", "operational state"); err != nil {
		t.Fatal(err)
	}
	tree, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{})
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string]bool{}
	var walk func([]cwmapi.ObjectTreeNode)
	walk = func(ns []cwmapi.ObjectTreeNode) {
		for _, n := range ns {
			flags[n.Object.ID] = n.System
			walk(n.Children)
		}
	}
	walk(tree.Roots)
	if !flags["sys-ops"] || flags["m"] || flags["m.g1"] {
		t.Errorf("the tree's system flags: %v; want sys-ops only", flags)
	}

	found := func(level string) bool {
		t.Helper()
		d, err := svc.Search(ctx, api.SearchInput{Level: level, SearchCriteria: api.SearchCriteria{Text: "operational"}})
		if err != nil {
			t.Fatal(err)
		}
		return strings.Contains(pageJSON(t, d), "sys-ops")
	}
	for _, level := range []string{"essential", "standard"} {
		if found(level) {
			t.Errorf("at %s the search offers the system module", level)
		}
	}
	if !found("expert") {
		t.Error("at Expert the search should offer the system module")
	}
}
