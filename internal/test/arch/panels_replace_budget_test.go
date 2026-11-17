// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// WHAT DISPLAY PANELS REPLACE DOES NOT COME BACK, ITS CLIENT CEILING
// (beside TestWhatPanelsReplaceStaysDeleted): a GUI binding
// whose action run answers its display panel - the object pane's Verify, the
// document export, a method-panel action - waits only as long as its target
// method declares (actionRunBudget), never a blanket ceiling. The generic
// dispatch (WorkspaceRunAction / WorkspaceRunActionAs) waits so too since
// - a script's slot method advertises its run bound - and its
// blanket actionRunTimeout is deleted (TestWhatPanelsReplaceStaysDeleted);
// the control-plane calls keep their byte-transfer budget
// (isLongRunningKTSP).
var panelAnsweringBindings = []string{
	"app/ktw-gui/app_workspace_object_detail.go", // WorkspaceObjectVerifyRun
	"app/ktw-gui/app_export.go",                  // DocumentExportRun
	"app/ktw-gui/app_workspace_capability.go",    // WorkspaceRunMethod
	"app/ktw-gui/app_workspace_actions.go",       // WorkspaceRunAction(As)
}

var blanketActionCeiling = regexp.MustCompile(`\bactionRunTimeout\b`)

func TestAPanelAnsweringBindingKeepsNoBlanketCeiling(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range panelAnsweringBindings {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s: %v (the binding moved: update the list)", rel, err)
		}
		if m := blanketActionCeiling.FindString(string(raw)); m != "" {
			t.Errorf("%s names %q - a binding answering its panel waits as long as its method declares (actionRunBudget)", rel, m)
		}
	}
}
