// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// contributorTargets are the packages of a kind's owner that a contributor
// calls today to publish its content: a contributor - an extension's contrib
// package or a binary's bootstrap (app/*/cmd) - should link only the generic
// contribution stack and contribute a pack of plain kinds instead
// (goal; rule contributors-link-no-target-package of
// tools/import-discipline/rules.yaml).
var contributorTargets = []string{
	"internal/extensions/cwm/contrib",
	"internal/extensions/slidedeck/contrib",
	"internal/extensions/drawio/contrib",
	"internal/extensions/sources/contrib",
	"internal/extensions/sources/api",
	"internal/extensions/publications/contrib",
	"internal/extensions/task/api",
}

// contributorHosts are the binaries that serve a target, which they link by
// design: target -> host bootstrap. The composition root app/ktw-agents/cmd
// links everything and is out of the rule.
var contributorHosts = map[string][]string{
	"internal/extensions/slidedeck/contrib": {"app/ktw-docs/cmd"},
	"internal/extensions/drawio/contrib":    {"app/ktw-docs/cmd"},
}

// contributorDebt is today's (importer, target) pairs, the backlog of
// parts. It only shrinks: a pair that is not listed
// fails, and a listed pair whose import is gone fails too - remove its line.
var contributorDebt []string

var contributorPkg = regexp.MustCompile(`^(internal/extensions/[^/]+/contrib|app/[^/]+/cmd)$`)

// contributorPairs measures today's (importer, target) pairs from go list:
// production imports only, a target's own extension and its hosts left out.
func contributorPairs(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \",\"}}", module+"/...").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	targets := map[string]bool{}
	for _, tg := range contributorTargets {
		targets[tg] = true
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	contributors := 0
	var pairs []string
	for _, line := range lines {
		pkg, imps, _ := strings.Cut(strings.TrimSpace(line), "|")
		pkg = strings.TrimPrefix(pkg, module+"/")
		if !contributorPkg.MatchString(pkg) || pkg == "app/ktw-agents/cmd" {
			continue
		}
		contributors++
		for _, imp := range strings.Split(imps, ",") {
			tg := strings.TrimPrefix(imp, module+"/")
			if !targets[tg] {
				continue
			}
			owner := strings.Join(strings.SplitN(tg, "/", 4)[:3], "/") + "/"
			if strings.HasPrefix(pkg, owner) || contains(contributorHosts[tg], pkg) {
				continue
			}
			pairs = append(pairs, pkg+" -> "+tg)
		}
	}
	if len(lines) < 200 || contributors < 30 {
		t.Fatalf("go list returned %d packages and %d contributors - the guard is not looking at the tree", len(lines), contributors)
	}
	sort.Strings(pairs)
	return pairs
}

// TestContributorsLinkNoTargetPackage - no contributor gains an import of a
// target package, and the allowlist of today's imports only shrinks.
func TestContributorsLinkNoTargetPackage(t *testing.T) {
	checkContributorPairs(t, contributorPairs(t), contributorDebt)
}

func checkContributorPairs(t *testing.T, got, debt []string) {
	t.Helper()
	listed := map[string]bool{}
	for _, p := range debt {
		listed[p] = true
	}
	seen := map[string]bool{}
	var added, gone []string
	for _, p := range got {
		seen[p] = true
		if !listed[p] {
			added = append(added, p)
		}
	}
	for _, p := range debt {
		if !seen[p] {
			gone = append(gone, p)
		}
	}
	sort.Strings(gone)
	if len(added) > 0 {
		t.Errorf("a contributor imports a target package (%d):\n  %s\n"+
			"contribute the kind's files as a pack (ContributePack, the kit's default pack) instead - see modules/contributions/packages.md",
			len(added), strings.Join(added, "\n  "))
	}
	if len(gone) > 0 {
		t.Errorf("allowlisted pairs no longer import their target (%d) - remove them from the allowlist, it only shrinks:\n  %s",
			len(gone), strings.Join(gone, "\n  "))
	}
}
