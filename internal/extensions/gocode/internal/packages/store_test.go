// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// repo is a small repository: module m owns internal/a and internal/b, a
// imports b.
func repo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/r\n\ngo 1.22\n")
	write(t, root, "modules/m/objects/m.cwm.yaml", "id: m\ntype: module\ncapabilities:\n  code:\n    code_packages: [internal/a, internal/b]\n")
	write(t, root, "internal/a/a.go", "// Package a does A. It has more.\npackage a\n\nimport (\n\t\"fmt\"\n\t\"example.com/r/internal/b\"\n)\n\nvar _ = fmt.Sprint(b.B)\n")
	write(t, root, "internal/a/a_test.go", "package a\n")
	write(t, root, "internal/b/b.go", "package b\n\nconst B = 1\n")
	write(t, root, "internal/c/c.go", "package c\n")
	return root
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// A change must show in the stamp even within the same clock tick.
	later := time.Now().Add(time.Duration(len(content)) * time.Millisecond)
	_ = os.Chtimes(p, later, later)
}

// clock is a clock a test moves.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func facts(t *testing.T, s *Store, path string) api.PackageFacts {
	t.Helper()
	out, err := s.PackageFacts(context.Background(), api.PackageFactsInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return out.Facts
}

// A package's facts: its owner by the longest code_packages path, its kind
// from the path, its description from the package comment, its files and its
// repository imports — refreshed once its files change and the TTL passed.
func TestTheStoreAnswersAPackagesFacts(t *testing.T) {
	root := repo(t)
	c := &clock{t: time.Unix(1_000_000, 0)}
	s := New(root, t.TempDir()).WithClock(c.now, 30*time.Second)

	a := facts(t, s, "internal/a")
	if a.Owner != "m" || a.Kind != api.KindCore || a.Description != "Package a does A." ||
		strings.Join(a.Imports, ",") != "internal/b" || len(a.Files) != 2 || a.Basis == "" {
		t.Fatalf("facts of internal/a: %+v", a)
	}
	if c := facts(t, s, "internal/c"); c.Owner != "" {
		t.Errorf("an unowned package has no owner: %+v", c)
	}

	write(t, root, "internal/a/a.go", "// Package a does A, changed.\npackage a\n")
	if again := facts(t, s, "internal/a"); again.Basis != a.Basis {
		t.Errorf("within the TTL the files are not looked at again: %s -> %s", a.Basis, again.Basis)
	}
	c.t = c.t.Add(31 * time.Second)
	changed := facts(t, s, "internal/a")
	if changed.Basis == a.Basis || changed.Description != "Package a does A, changed." || len(changed.Imports) != 0 {
		t.Errorf("after the TTL a changed file refreshes the facts: %+v", changed)
	}
}

// An analysis is recorded with the basis it was made on: a shallow one is
// stale once the package's own files change, a deep one (a test run) also
// once an imported package changes; the store keeps them across a restart.
func TestAnAnalysisIsStaleOnceItsBasisMoves(t *testing.T) {
	root := repo(t)
	dir := t.TempDir()
	c := &clock{t: time.Unix(1_000_000, 0)}
	s := New(root, dir).WithClock(c.now, 0)
	ctx := context.Background()
	for _, in := range []api.PackageRecordInput{
		{Path: "internal/a", Kind: "vet", Verdict: "pass"},
		{Path: "internal/a", Kind: "tests", Verdict: "pass", Deep: true},
	} {
		if _, err := s.PackageRecord(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	stale := func(s *Store) map[string]bool {
		m := map[string]bool{}
		for _, a := range facts(t, s, "internal/a").Analyses {
			m[a.Kind] = a.Stale
		}
		return m
	}
	if got := stale(s); got["vet"] || got["tests"] || len(got) != 2 {
		t.Fatalf("fresh analyses: %v", got)
	}
	write(t, root, "internal/b/b.go", "package b\n\nconst B = 2\n")
	if got := stale(s); got["vet"] || !got["tests"] {
		t.Errorf("an import changed: the deep analysis is stale, the shallow one not: %v", got)
	}
	write(t, root, "internal/a/a.go", "package a\n\nimport \"example.com/r/internal/b\"\n\nvar _ = b.B\n")
	if got := stale(s); !got["vet"] {
		t.Errorf("its own file changed: the shallow analysis is stale: %v", got)
	}
	restarted := New(root, dir).WithClock(c.now, 0)
	if got := stale(restarted); len(got) != 2 || !got["vet"] || !got["tests"] {
		t.Errorf("the analyses survive a restart, still stale: %v", got)
	}
	if _, err := restarted.PackageRecord(ctx, api.PackageRecordInput{Path: "internal/a", Kind: "vet", Verdict: "fail"}); err != nil {
		t.Fatal(err)
	}
	if got := stale(restarted); got["vet"] {
		t.Errorf("a new analysis of the kind replaces the old one, fresh: %v", got)
	}
}

// A module's packages are those its code_packages own.
func TestPackageListIsAModulesPackages(t *testing.T) {
	s := New(repo(t), t.TempDir())
	out, err := s.PackageList(context.Background(), api.PackageListInput{Module: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, p := range out.Packages {
		paths = append(paths, p.Path)
	}
	if strings.Join(paths, ",") != "internal/a,internal/b" {
		t.Errorf("m's packages: %v", paths)
	}
}

// A change's Go files go to the modules owning their packages, the longest
// code_packages path winning; a page or an unowned package is left out.
func TestChangeOwnersGroupsAChangesGoFilesByModule(t *testing.T) {
	root := repo(t)
	write(t, root, "modules/n/objects/n.cwm.yaml", "id: n\ntype: module\ncapabilities:\n  code:\n    code_packages: [internal/a/deep/...]\n")
	write(t, root, "internal/a/deep/d.go", "package deep\n")
	s := New(root, t.TempDir())
	out, err := s.ChangeOwners(context.Background(), reviewsapi.ChangeOwnersInput{Paths: []string{
		"internal/a/a.go", "internal/a/deep/d.go", "internal/b/b.go", "internal/c/c.go", "modules/m/concepts.md", "./internal/a/a.go"}})
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(out.Owners)
	if want := "[{m [internal/a/a.go internal/b/b.go]} {n [internal/a/deep/d.go]}]"; got != want {
		t.Errorf("owners: %s; want %s", got, want)
	}
}
