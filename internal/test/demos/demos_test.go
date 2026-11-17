// SPDX-License-Identifier: Apache-2.0

// Package demos contains the e2e regression test for the shipped
// task/workflow demos in app/ktw-agents/embed/tasks/. Each demo is run via
// `ktw-agents run <name> --raw --dry-run` and the output is asserted on.
//
// Why dry-run only: --dry-run substitutes LLM/program children with a
// simulator (zero token cost, ~50ms per demo). Real-LLM runs are out of
// scope here — they belong in a separate (gated, optional) suite that
// burns budget. This file is the cheap, always-on regression net.
//
// What this test guards:
//   - Each demo's frontmatter parses (loader doesn't reject it)
//   - The demo's workflow topology executes without error
//   - Workflow demos emit `workflow.<runID>.done` (i.e. they actually
//     terminate, not hang on a dangling branch — important after the
//     ledger fix in ms06b)
//   - Non-workflow tasks (type: task) emit a `Dry-run:` summary header
//
// What this test does NOT guard:
//   - Real-LLM correctness (token/quality/output)
//   - Tools that interact with external state (filesystem mutation, network)
//   - The 2 `type: agent` demos (demo-delegate-team-coordination,
//     demo-delegate-team-messaging) — they hang under --dry-run because the
//     simulator-substitution path doesn't cover agent-loop runs the same
//     way. Tracked as a follow-up task-engine concern, not a content bug.
package demos

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// demoSpec describes a shipped demo and what to look for in its
// `--raw --dry-run` output.
type demoSpec struct {
	name        string // task name (matches the embed/tasks/<name>/ folder)
	mustContain string // substring that MUST appear in stdout
	skipReason  string // non-empty → test skipped, reason logged
}

// All shipped demos worth regression-testing. Add entries here when new
// demos land. Workflow demos check for `workflow.<...>.done` (which only
// fires on clean termination); non-workflow tasks check for `Dry-run:`
// (the dry-run renderer's header).
var demos = []demoSpec{
	// type: task — single-agent demos (substituted directly to simulator)
	{name: "hello-world", mustContain: "Dry-run:"},
	// Renamed from `document-demo` in b8d676f8 (2026-06-28); this list kept
	// the old name and the suite had been failing on it since.
	{name: "display-document-demo", mustContain: "Dry-run:"},
	{name: "codebase-audit", mustContain: "Dry-run:"},
	{name: "goal-planner", mustContain: "Dry-run:"},

	// type: workflow — orchestration demos
	{name: "demo-delegate-pipeline", mustContain: "workflow."},
	{name: "demo-delegate-parallel-fallback", mustContain: "workflow."},
	{name: "demo-delegate-magentic", mustContain: "workflow."},
	{name: "demo-delegate-team-tree", mustContain: "workflow."},
	{name: "task-builder-master", mustContain: "workflow."},

	// type: agent — skipped pending --dry-run path support for agent-loop
	// runs. Each hangs waiting on an LLM call that isn't substituted.
	{name: "demo-delegate-team-coordination", skipReason: "type:agent --dry-run hangs (task-engine TODO, not a demo bug)"},
	{name: "demo-delegate-team-messaging", skipReason: "type:agent --dry-run hangs (task-engine TODO, not a demo bug)"},
}

// repoRoot walks up from the current test file to find the project root
// (the directory containing go.mod). All paths in the test resolve from
// here so the test works regardless of the working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod above %s", wd)
	return ""
}

// binaryPath returns the prebuilt ktw-agents binary at <root>/bin/ktw-agents.exe
// (Windows) or .../ktw-agents (Unix). Tests skip cleanly if the binary
// isn't built — this avoids slow `go build` invocations inside the test
// loop while keeping the test runnable in any local checkout.
func binaryPath(t *testing.T) string {
	t.Helper()
	p := hubharness.Binary(t, "ktw-agents")
	if p == "" {
		t.Skipf("ktw-agents binary not found in bin/; build with `go build -o bin/ktw-agents ./app/ktw-agents/` and re-run")
	}
	return p
}

// TestDemoRegressions runs each shipped demo via `ktw-agents run <name>
// --raw --dry-run` and asserts the expected substring appears in stdout.
// Failure on any demo means a regression — either the demo's TASK.md
// stopped parsing, the workflow no longer terminates, or the dry-run
// output format changed.
func TestDemoRegressions(t *testing.T) {
	bin := binaryPath(t)

	// Start ktw-hub for this test — ktw-agents now fetches catalog data
	// via `ktw-hub:/CatalogService/*` over the bus (2026-06-05 layering
	// rule; see hub-owns-cache-layering-rule). Without the hub running,
	// `ktw-agents run <demo>` sees an empty universe and "task <demo>
	// not found" wins every assertion. `hubharness.Start` skips this
	// test if `bin/ktw-hub` isn't built so a fresh clone doesn't fail
	// loudly.
	//
	// The dataDir is co-located with the test's tempdir so the hub and
	// ktw-agents subprocess agree on the lockfile path. The universe
	// includes ktw-demo as a stdio sidecar so the hub's auto-hydration
	// populates its manifest at boot — without that, demos shipped in
	// ktw-demo (demo-delegate-magentic, demo-delegate-parallel-fallback,
	// demo-delegate-pipeline) would 404 on lookup. ktw-agents' own
	// embedded demos (hello-world, etc.) come from its local manifest
	// builder regardless of who's in the universe.
	demoBin := hubharness.Binary(t, "ktw-demo")
	if demoBin == "" {
		hubharness.MissingBinary(t, "demos: bin/ktw-demo not built")
	}
	dataDir := hubharness.MakeTempUniverseWithSidecars(t, []hubharness.SidecarBin{
		{Name: "ktw-demo", Binary: demoBin},
	})
	hubharness.Start(t, dataDir)

	for _, d := range demos {
		t.Run(d.name, func(t *testing.T) {
			if d.skipReason != "" {
				t.Skip(d.skipReason)
			}

			cmd := exec.Command(bin, "run", d.name, "--raw", "--dry-run")
			cmd.Dir = repoRoot(t)
			// Per-subtest CWM workspace dir — the CWM substrate holds
			// an exclusive lease on its workspace folder, and a
			// subprocess kill mid-run sometimes leaves the lock
			// behind. Giving each subtest its OWN workspace dir
			// guarantees no inter-subtest interference. The hub +
			// catalog state still come from the shared `dataDir`.
			subWS := t.TempDir()
			cmd.Env = append(os.Environ(),
				"KTW_DATA="+dataDir,
				"KTW_WORKSPACE_DIR="+subWS,
			)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				t.Fatalf("run %s: %v\n--- stdout ---\n%s\n--- stderr ---\n%s",
					d.name, err, stdout.String(), stderr.String())
			}

			out := stdout.String()
			if !strings.Contains(out, d.mustContain) {
				t.Errorf("demo %s: stdout missing %q\n--- stdout ---\n%s",
					d.name, d.mustContain, out)
			}

			// Generic regression check: surface "Error" or "panic" lines,
			// which shouldn't appear in a clean dry-run.
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "Error: ") || strings.Contains(line, "panic:") {
					t.Errorf("demo %s: error-like line in stdout: %s", d.name, line)
				}
			}
		})
	}
}
