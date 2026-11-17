// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// schedulerTree is the scheduler's whole tree: its API, its core, its stores
// and its wiring.
const schedulerTree = module + "/internal/extensions/scheduler/..."

// planificationTree is the owner whose words (the K1-K6 blocker kinds) the
// scheduler never speaks: it hands each wait over as a neutral cause and
// holder (api.QueueWait) and an owner's wait namer labels it.
const planificationTree = module + "/internal/extensions/planification"

// TestTheSchedulerImportsNoPlanification — no package of the scheduler's tree
// depends, even transitively, on planification, nor do its tests import it
func TestTheSchedulerImportsNoPlanification(t *testing.T) {
	deps, err := exec.Command("go", "list", "-deps", schedulerTree).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -deps: %v", err)
	}
	tests, err := exec.Command("go", "list", "-f",
		"{{.ImportPath}}|{{join .TestImports \",\"}},{{join .XTestImports \",\"}}", schedulerTree).Output()
	if err != nil {
		t.Fatalf("go list test imports: %v", err)
	}
	depLines := strings.Split(strings.TrimSpace(string(deps)), "\n")
	if len(depLines) < 20 || !strings.Contains(string(deps), module+"/internal/extensions/scheduler/internal/core") {
		t.Fatalf("go list -deps returned %d packages without the scheduler's core - the guard is not looking at the scheduler", len(depLines))
	}
	forbidden := func(imp string) bool {
		return imp == planificationTree || strings.HasPrefix(imp, planificationTree+"/")
	}
	var violations []string
	for _, d := range depLines {
		if forbidden(d) {
			violations = append(violations, "the scheduler depends on "+d)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(tests)), "\n") {
		pkg, imps, _ := strings.Cut(line, "|")
		for _, imp := range strings.Split(imps, ",") {
			if forbidden(imp) {
				violations = append(violations, pkg+" (tests) imports "+imp)
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("the scheduler reaches planification (%d):\n  %s\n"+
			"the scheduler emits a neutral wait (api.QueueWait); planification names it through a wait namer (.wait-namers) - see modules/scheduler",
			len(violations), strings.Join(violations, "\n  "))
	}
}
