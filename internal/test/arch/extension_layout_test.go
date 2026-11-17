// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// extensionRoles are the directory names the layout convention reserves, one
// per role (modules/framework/extension-layout.md §2). A directory directly
// under internal/extensions/<ext>/ is one of these, `internal`, `testdata`,
// `embed`, or a declared deviation.
var extensionRoles = []string{"api", "local", "contrib", "lint", "commands", "remote", "plugin", "testing"}

// extensionFamilies are the extensions whose every child is one member of a
// family (one package per agent tool under tools/), named in snake case after
// the member's kebab wire name.
var extensionFamilies = map[string]*regexp.Regexp{
	"tools": regexp.MustCompile(`^[a-z][a-z0-9_]*$`),
}

// extensionLayoutDeviations is the allow-list: every top-level directory that
// is not a role, per extension, equal to the inventory's deviations
// (modules/framework/_generated/extension-layout.md). An entry leaves the list
// when its package moves; a new name fails until it is moved under internal/
// or declared here with the importer that earns it. The private extensions'
// deviations are declared in private_test.go.
var extensionLayoutDeviations = map[string][]string{
	"code":         {"assistant", "coverage", "cwmprojection", "modulehealth", "realization", "tierpromotion"},
	"connmgr":      {"hubdial"},           // hubdial: another universe's hub members, listed and restarted (marketplace, orchestrator)
	"cwm":          {"folder", "migrate"}, // folder: CWM in process on a project no hub serves (marketplace's soldocs lays out a solution's documents); migrate: a project folder no hub serves - the copy marketplace's import of earlier projects makes
	"drawio":       {"render"},
	"humangate":    {"formdef"},
	"manifest":     {"teststub", "universe"},
	"marketplace":  {"authoring", "e2etest", "follow", "index", "keysfile"}, // follow: the universe hub (app/ktw-hub) follows its keys and config files
	"methods":      {"methodsservice", "verify"},
	"orchestrator": {"admin", "backupjob", "catalog"}, // backupjob: the universe backup job, run by the binary that serves the universe (app/ktw-agents) without linking the orchestrator core
	"profile":      {"agentplugin", "person"},         // person: the CLI roots' person cause (internal/appcore, app/ktw-agents/cmd)
	"scenario":     {"publish", "sandbox"},
	"skills":       {"agentplugin"},
	"slidedeck":    {"display"},
	"sources":      {"report"}, // report: the neutral tabular report runner the source connectors' sidecars run their reports on (outside sources, so it cannot move under sources/internal)
	"tui":          {"render"},
}

// extensionRootBesideSubpackages are the extensions that hold Go files at
// their root AND sub-packages; a flat extension is its root package alone.
var extensionRootBesideSubpackages = []string{"humangate", "tools", "tui"}

// extensionFilesInInternal are the extensions whose internal/ holds Go files
// directly (a package named `internal`) instead of internal/core.
var extensionFilesInInternal = []string{}

// TestExtensionLayout_DirectoryNamesAreRolesOrDeclared: every directory
// directly under internal/extensions/<ext>/ is a role, `internal`, `testdata`,
// `embed`, a family member, or declared in extensionLayoutDeviations; every
// declared deviation exists; root Go files beside sub-packages and Go files
// directly in internal/ are declared. A stale declaration is as wrong as an
// undeclared directory.
func TestExtensionLayout_DirectoryNamesAreRolesOrDeclared(t *testing.T) {
	root := repoRoot(t)
	extRoot := filepath.Join(root, "internal", "extensions")
	exts, err := os.ReadDir(extRoot)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]bool{"internal": true, "testdata": true, "embed": true}
	for _, r := range extensionRoles {
		roles[r] = true
	}
	var problems []string
	seenDeviation := map[string]bool{}
	seenRoot := map[string]bool{}
	seenInternal := map[string]bool{}
	for _, e := range exts {
		if !e.IsDir() {
			continue
		}
		ext := e.Name()
		dir := filepath.Join(extRoot, ext)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		allowed := map[string]bool{}
		for _, d := range extensionLayoutDeviations[ext] {
			allowed[d] = true
		}
		rootGo, subDirs := false, false
		for _, en := range entries {
			n := en.Name()
			if !en.IsDir() {
				if strings.HasSuffix(n, ".go") {
					rootGo = true
				}
				continue
			}
			if n == "testdata" || n == "embed" || strings.HasPrefix(n, ".") {
				continue
			}
			subDirs = true
			switch {
			case roles[n]:
			case allowed[n]:
				seenDeviation[ext+"/"+n] = true
			case extensionFamilies[ext] != nil && extensionFamilies[ext].MatchString(n):
			default:
				problems = append(problems, "undeclared: internal/extensions/"+ext+"/"+n+"/ (not a role: move it under internal/ or declare it in extensionLayoutDeviations with the importer that earns it)")
			}
		}
		if rootGo && subDirs {
			if contains(extensionRootBesideSubpackages, ext) {
				seenRoot[ext] = true
			} else {
				problems = append(problems, "root package beside sub-packages: internal/extensions/"+ext+"/*.go (wiring goes to local/, implementation to internal/core/)")
			}
		}
		if m, _ := filepath.Glob(filepath.Join(dir, "internal", "*.go")); len(m) > 0 {
			if contains(extensionFilesInInternal, ext) {
				seenInternal[ext] = true
			} else {
				problems = append(problems, "Go files directly in internal/: internal/extensions/"+ext+"/internal/*.go (the implementation is internal/core/)")
			}
		}
	}
	for ext, names := range extensionLayoutDeviations {
		for _, n := range names {
			if !seenDeviation[ext+"/"+n] {
				problems = append(problems, "declared but absent: internal/extensions/"+ext+"/"+n+"/ (remove it from extensionLayoutDeviations)")
			}
		}
	}
	for _, ext := range extensionRootBesideSubpackages {
		if !seenRoot[ext] {
			problems = append(problems, "declared but absent: root package of internal/extensions/"+ext+" (remove it from extensionRootBesideSubpackages)")
		}
	}
	for _, ext := range extensionFilesInInternal {
		if !seenInternal[ext] {
			problems = append(problems, "declared but absent: internal/extensions/"+ext+"/internal/*.go (remove it from extensionFilesInInternal)")
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("extension layout rule (modules/framework/extension-layout.md):\n  %s", strings.Join(problems, "\n  "))
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
