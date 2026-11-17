// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE PROJECT TREE HOLDS THE PLACES YOU WORK IN (approved by the
// person): the project, its modules and groups, and objects that hold other
// objects you open one by one. A record of a place or a surface with its own
// entry is listed by its parent's panel, never drawn. The set of shipped
// object types the tree draws (`load: with-tree`, or no `load:` and `tree:`
// root / node — modules/gui/objects-what-the-user-sees.md §2) is exactly the
// approved list below; a type that moves in or out of the tree changes this
// list on the person's word.
var approvedDrawnTypes = []string{
	// RQ-010's list, under today's names (project -> the project types,
	// code-module -> software-module; project groups retired).
	"software-project",
	"system-module", "software-module", "group",
	"composition", "marketplace-project",
	// Root places declared since RQ-010's list, confirmed in the tree by the
	// person. The private packs' root places are listed in
	// private_test.go.
	"documentation",
}

// Moved to panels: none of them is drawn again. Each stays listed in its
// parent's pane.
var movedToPanels = []string{
	// By (the private packs' types are listed in private_test.go).
	"use-case", "review", "stream", "inbox", "triage",
	"compose-artifact", "compose-skill", "compose-task",
	"signoff-item",
	// Declared after RQ-010's list; placed in panels by the person
	//
	"evaluation-grid", "solution-evaluation", "information-source",
}

func TestTheDrawnTypesAreTheApprovedTreeList(t *testing.T) {
	root := repoRoot(t)
	var files []string
	for _, pattern := range []string{"app/*/embed/cwm/object-types/*.yaml", "internal/extensions/*/contrib/embed/cwm/object-types/*.yaml"} {
		m, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) < 30 {
		t.Fatalf("only %d object-type files found - the scan is not working", len(files))
	}
	drawn := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var ot cwmapi.ObjectType
		if err := config.DecodeBytes(data, marshal.YAML, &ot); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		name := ot.Name
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(f), ".yaml")
		}
		isDrawn := ot.Load == "with-tree" || (ot.Load == "" && ot.Drawn())
		if isDrawn {
			rel, _ := filepath.Rel(root, f)
			drawn[name] = filepath.ToSlash(rel)
		}
	}

	want := map[string]bool{}
	for _, n := range approvedDrawnTypes {
		want[n] = true
	}
	var extra, missing []string
	for n, f := range drawn {
		if !want[n] {
			extra = append(extra, n+" ("+f+")")
		}
	}
	for n := range want {
		if _, ok := drawn[n]; !ok {
			missing = append(missing, n)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	if len(extra) > 0 {
		t.Errorf("drawn in the project tree but not on the approved list - a record or a surface goes to its parent's panel; a new place needs the person's word:\n  %s", strings.Join(extra, "\n  "))
	}
	if len(missing) > 0 {
		t.Errorf("on the approved tree list but no longer drawn - update the list with the person's word:\n  %s", strings.Join(missing, "\n  "))
	}
	for _, n := range movedToPanels {
		if f, ok := drawn[n]; ok {
			t.Errorf("%s was moved to a panel but is drawn again (%s)", n, f)
		}
	}
}
