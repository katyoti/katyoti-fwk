// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// cmdExtraFiles is the allow-list behind the `cmd/` layout rule: a binary's
// `cmd/` holds `root.go` (the entry) and `bootstrap.go` (the wiring), plus the
// command files that belong to that binary alone and no extension owns. An
// extra file must be declared here, with the binary, or the guard refuses it,
// so the rule bends visibly and never silently (re-audit N15, decided
// 2026-09-26: moving these files was out of proportion; declaring them is not).
// The files of the private composition are declared in private_test.go.
var cmdExtraFiles = map[string][]string{
	"ktw-agents":  {"config.go", "console.go", "dryrun_tracer.go", "io.go", "run.go", "sm.go", "terminal.go"},
	"ktw-compose": {"solution.go"},
	"ktw-hub":     {"cache_ops.go", "catalog_refresh.go", "catalog_service.go", "connmgr_service.go"},
	"ktw-test":    {"test.go"},
	// ktw-utils is not a sidecar (a plain cobra CLI); its cmd/ is its own.
	"ktw-utils": {"gen_ktsp_client.go", "inspect.go", "journal.go", "session.go", "session_aggregate.go", "session_follow.go", "session_tree.go", "version.go"},
}

// TestCmdLayout_ExtraFilesAreDeclared: every non-test Go file under
// app/<x>/cmd/ other than root.go and bootstrap.go is declared in
// cmdExtraFiles for that binary, and every declared file exists (a stale
// declaration is as wrong as an undeclared file).
func TestCmdLayout_ExtraFilesAreDeclared(t *testing.T) {
	root := repoRoot(t)
	apps, err := os.ReadDir(filepath.Join(root, "app"))
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	seen := map[string]bool{}
	for _, a := range apps {
		if !a.IsDir() {
			continue
		}
		cmdDir := filepath.Join(root, "app", a.Name(), "cmd")
		entries, err := os.ReadDir(cmdDir)
		if err != nil {
			continue // no cmd/: not a binary built on the rule
		}
		allowed := map[string]bool{}
		for _, f := range cmdExtraFiles[a.Name()] {
			allowed[f] = true
		}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") || n == "root.go" || n == "bootstrap.go" {
				continue
			}
			seen[a.Name()+"/"+n] = true
			if !allowed[n] {
				problems = append(problems, "undeclared: app/"+a.Name()+"/cmd/"+n+" (declare it in cmdExtraFiles or move it to the binary's extension package)")
			}
		}
	}
	for bin, files := range cmdExtraFiles {
		for _, f := range files {
			if !seen[bin+"/"+f] {
				problems = append(problems, "declared but absent: app/"+bin+"/cmd/"+f)
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("cmd/ layout rule:\n  %s", strings.Join(problems, "\n  "))
	}
}

// repoRoot: this file lives at internal/test/arch/, four levels below it.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(here))))
}
