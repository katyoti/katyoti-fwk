// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// TestDelegationModes runs the no-LLM delegation-mode suite: one .tasktest per
// delegation mode (tags: [no-llm, delegation-mode]) under
// modules/tasks/tests/L2/modes/. Each scenario delegates a simulator child in
// one mode and asserts the child's resolved task_started.dimensions match that
// mode's preset — zero LLM, deterministic.
//
// Heavy integration (spawns the real binary per scenario); skipped under -short.
func TestDelegationModes(t *testing.T) {
	if testing.Short() {
		t.Skip("delegation-mode suite spawns the real binary per scenario; skipped under -short")
	}

	root := projectRoot(t)
	// The scenarios' universe; under .katyoti/, which .gitignore drops — a
	// checkout without it answers every scenario with verdict=error.
	if _, err := os.Stat(filepath.Join(root, "modules", "tasks", "fixtures", "delegation-modes", ".katyoti")); err != nil {
		t.Fatalf("fixture universe missing (it is under .katyoti/, ignored by git — add it with git add -f): %v", err)
	}
	buildBinaryOnce(t, root)

	modesDir := filepath.Join(root, "modules", "tasks", "tests", "L2", "modes")
	scenarios, err := filepath.Glob(filepath.Join(modesDir, "*.tasktest"))
	if err != nil {
		t.Fatalf("glob scenarios: %v", err)
	}
	if len(scenarios) == 0 {
		t.Fatalf("no .tasktest scenarios under %s", modesDir)
	}
	sort.Strings(scenarios)

	r := &Runner{ProjectRoot: root}
	for _, path := range scenarios {
		name := filepathStem(path)
		t.Run(name, func(t *testing.T) {
			// Validate (parse) first — surfaces malformed scenarios distinctly.
			sc, err := Load(path)
			if err != nil {
				t.Fatalf("load %s: %v", name, err)
			}
			// Enforce the no-llm tagging contract for this suite.
			if !hasTag(sc.Tags, "no-llm") {
				t.Fatalf("%s missing required tag %q (tags: %v)", name, "no-llm", sc.Tags)
			}
			res, err := r.Run(context.Background(), path)
			if err != nil {
				t.Fatalf("run %s: %v", name, err)
			}
			if res.Verdict != VerdictPass {
				t.Errorf("verdict=%v (exit %d)\nrun error: %v\nfailures:\n  %v\nstdout:\n%s\nstderr:\n%s",
					res.Verdict, res.ExitCode, res.RunErr, res.Failures, res.Stdout, res.Stderr)
			}
		})
	}
}

// projectRoot walks up from this test file's directory until it finds go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test dir")
		}
		dir = parent
	}
}

// buildBinaryOnce builds bin/ktw-agents.exe at the project root so the
// scenarios' `./bin/ktw-agents.exe run …` cmd resolves (faster than `go run`
// per scenario).
func buildBinaryOnce(t *testing.T, root string) {
	t.Helper()
	out := filepath.Join(root, "bin", binaryName())
	cmd := exec.Command("go", "build", "-o", out, "./app/ktw-agents")
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build ktw-agents: %v\n%s", err, b)
	}
}

// A scenario's relative command (`./bin/ktw-agents.exe`) runs from the project
// root whatever the caller's working directory: it is made absolute against
// it. A bare name stays a PATH lookup.
func TestResolveCommand(t *testing.T) {
	root := t.TempDir()
	if got := resolveCommand(root, "go"); got != "go" {
		t.Errorf("bare name: %q, want it left to PATH", got)
	}
	got := resolveCommand(root, "./bin/"+binaryName())
	if want := filepath.Join(root, "bin", binaryName()); got != want {
		t.Errorf("relative path: %q, want %q", got, want)
	}
	if got := resolveCommand("", "./bin/x"); got != "./bin/x" {
		t.Errorf("no project root: %q, want it unchanged", got)
	}
	if runtime.GOOS == "windows" {
		exe := filepath.Join(root, "bin", "tool.exe")
		if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exe, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := resolveCommand(root, "./bin/tool"); got != exe {
			t.Errorf("extension-less path on Windows: %q, want %q", got, exe)
		}
	}
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "ktw-agents.exe"
	}
	return "ktw-agents"
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func filepathStem(path string) string {
	base := filepath.Base(path)
	return base[:len(base)-len(filepath.Ext(base))]
}
