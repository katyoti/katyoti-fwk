// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// THE PANELS' API STAYS LIGHT: a sidecar shows a display
// panel through internal/core/panel alone - the handle, the event types and
// the Reporter its progress reporting feeds -, as linting adds no code to a
// sidecar. The api links no extension, and a package that uses it imports no
// task engine, no agent and no inbox: those are the hosts that run the work
// and draw the panels, never a sidecar's need.

// panelsAPI is the panels' api package.
const panelsAPI = module + "/internal/core/panel"

// panelsForbidden are the engine, the agent and the inbox: what a package
// using the panels' api never imports.
var panelsForbidden = []string{
	module + "/internal/extensions/task/api/engine",
	module + "/internal/extensions/task/internal",
	module + "/internal/extensions/agent",
	module + "/internal/extensions/inbox",
	module + "/internal/extensions/humangate",
}

// panelsHosts are the engine, agent and inbox sides themselves, the agent's
// in-process tools that wait on a panel, the chat's transport and the GUI:
// they run the work and draw the panels, so they hold both the api and what
// it keeps from a sidecar.
var panelsHosts = []string{
	module + "/internal/extensions/task",
	module + "/internal/extensions/agent",
	module + "/internal/extensions/inbox",
	module + "/internal/extensions/humangate",
	// ask_user_data is the agent's own tool, run in the agent's process: it
	// opens its question panel and waits on its input, never a sidecar.
	module + "/internal/extensions/tools/ask_user_data",
	module + "/internal/kts/protocol/terminal",
	module + "/app/ktw-gui",
}

func under(pkg string, roots []string) bool {
	for _, r := range roots {
		if pkg == r || strings.HasPrefix(pkg, r+"/") {
			return true
		}
	}
	return false
}

// TestThePanelsAPIIsAllASidecarImports — the panels' api depends on no
// extension, and no package using it, the hosts aside, imports the task
// engine, an agent or the inbox.
func TestThePanelsAPIIsAllASidecarImports(t *testing.T) {
	deps, err := exec.Command("go", "list", "-deps", panelsAPI).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -deps: %v", err)
	}
	var violations []string
	for _, d := range strings.Split(strings.TrimSpace(string(deps)), "\n") {
		if strings.HasPrefix(d, module+"/internal/extensions/") || strings.HasPrefix(d, module+"/app/") {
			violations = append(violations, "the panels' api depends on "+d)
		}
	}

	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \",\"}}", module+"/...").Output()
	if err != nil {
		t.Fatalf("go list imports: %v", err)
	}
	users := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imps, _ := strings.Cut(line, "|")
		list := strings.Split(imps, ",")
		uses := false
		for _, imp := range list {
			if imp == panelsAPI {
				uses = true
			}
		}
		if !uses || pkg == panelsAPI {
			continue
		}
		users++
		if under(pkg, panelsHosts) {
			continue
		}
		for _, imp := range list {
			if under(imp, panelsForbidden) {
				violations = append(violations, pkg+" uses the panels' api and imports "+imp)
			}
		}
	}
	if users < 3 {
		t.Fatalf("only %d packages use %s - the guard is not looking at its users", users, panelsAPI)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("the panels' api stops being all a sidecar imports (%d):\n  %s\n"+
			"a sidecar shows a panel with internal/core/panel alone - see modules/display/display-panels.md « A sidecar's panel »",
			len(violations), strings.Join(violations, "\n  "))
	}
}
