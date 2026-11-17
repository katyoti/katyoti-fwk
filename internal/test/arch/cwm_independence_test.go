// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// cwmTree is CWM's whole tree: its API, its core, its wiring and its client.
const cwmTree = module + "/internal/extensions/cwm/..."

// reactionRunners are the packages that decide and run what happens after an
// event: the events extension (the taker, the runner, the sources), the
// scheduler (the queues) and planification (requests, plans). CWM records the
// events and their matches and guards the loop; it hands each match over
// through cwmapi.TakeMatches and never imports who takes it
// (modules/events/reactions.md §2).
var reactionRunners = []string{
	module + "/internal/extensions/events",
	module + "/internal/extensions/scheduler",
	module + "/internal/extensions/planification",
}

// TestCWMImportsNoReactionRunner — no package of CWM's tree depends, even
// transitively, on the events extension, the scheduler or planification;
// nor do its tests import them.
func TestCWMImportsNoReactionRunner(t *testing.T) {
	deps, err := exec.Command("go", "list", "-deps", cwmTree).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -deps: %v", err)
	}
	tests, err := exec.Command("go", "list", "-f",
		"{{.ImportPath}}|{{join .TestImports \",\"}},{{join .XTestImports \",\"}}", cwmTree).Output()
	if err != nil {
		t.Fatalf("go list test imports: %v", err)
	}
	depLines := strings.Split(strings.TrimSpace(string(deps)), "\n")
	if len(depLines) < 20 || !strings.Contains(string(deps), module+"/internal/extensions/cwm/internal/core") {
		t.Fatalf("go list -deps returned %d packages without CWM's core — the guard is not looking at CWM", len(depLines))
	}
	forbidden := func(imp string) bool {
		for _, f := range reactionRunners {
			if imp == f || strings.HasPrefix(imp, f+"/") {
				return true
			}
		}
		return false
	}
	var violations []string
	for _, d := range depLines {
		if forbidden(d) {
			violations = append(violations, "CWM depends on "+d)
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
		t.Errorf("CWM reaches what runs the reactions (%d):\n  %s\n"+
			"CWM records and guards; it hands matches over through cwmapi.TakeMatches — see modules/events/reactions.md §2",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// cwmLocal is CWM's wiring: the service factory, the boot gate, the agent
// plugin's contribution. A binary's bootstrap adds it; an extension that
// reacts to CWM, or takes its matches, declares itself through the contract
// (cwmapi.ObserveEvents, cwmapi.TakeMatches) and never links CWM's wiring.
const cwmLocal = module + "/internal/extensions/cwm/local"

// cwmLocalTestImporters are the packages whose TESTS assemble a whole
// application with CWM in it, beside internal/test/...: each with what it
// proves, so the list cannot quietly become a way around the rule.
var cwmLocalTestImporters = map[string]string{
	module + "/internal/extensions/gocode/local": "the object-migration smoke runs gocode against a real CWM",
}

// TestCWMLocalIsImportedOnlyByBinariesAndTests — cwm/local is imported only by
// a binary's cmd package (app/*/cmd), by internal/test/..., and by CWM's own
// tree; a test elsewhere imports it only when cwmLocalTestImporters names it.
func TestCWMLocalIsImportedOnlyByBinariesAndTests(t *testing.T) {
	out, err := exec.Command("go", "list", "-f",
		"{{.ImportPath}}|{{join .Imports \",\"}}|{{join .TestImports \",\"}},{{join .XTestImports \",\"}}",
		module+"/...").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	allowed := func(pkg string) bool {
		switch {
		case strings.HasPrefix(pkg, module+"/app/") && strings.HasSuffix(pkg, "/cmd"):
			return true
		case strings.HasPrefix(pkg, module+"/internal/test/"):
			return true
		case strings.HasPrefix(pkg, module+"/internal/extensions/cwm/"):
			return true
		}
		return false
	}
	has := func(list string) bool {
		for _, imp := range strings.Split(list, ",") {
			if imp == cwmLocal {
				return true
			}
		}
		return false
	}
	var importers, violations []string
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		pkg := parts[0]
		if has(parts[1]) {
			importers = append(importers, pkg)
			if !allowed(pkg) {
				violations = append(violations, pkg+" imports "+cwmLocal)
			}
		}
		if has(parts[2]) && !allowed(pkg) && cwmLocalTestImporters[pkg] == "" {
			violations = append(violations, pkg+" (tests) imports "+cwmLocal)
		}
	}
	if len(lines) < 200 || len(importers) == 0 {
		t.Fatalf("go list returned %d packages and %d importers of cwm/local — the guard is not looking at the tree", len(lines), len(importers))
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("cwm/local is CWM's wiring, imported by a binary's cmd and by tests only (%d):\n  %s\n"+
			"declare an observer or a match taker through cwmapi.ObserveEvents / cwmapi.TakeMatches",
			len(violations), strings.Join(violations, "\n  "))
	}
}
