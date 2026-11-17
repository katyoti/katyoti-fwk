// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// world writes a small module: b imports a, c imports b (in its test only),
// d stands alone, other/ is a module of its own.
func world(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                 "module example.com/w\n\ngo 1.22\n",
		"a/a.go":                 "package a\n",
		"a/testdata/case.txt":    "input\n",
		"b/b.go":                 "package b\n\nimport _ \"example.com/w/a\"\n",
		"c/c.go":                 "package c\n",
		"c/c_test.go":            "package c_test\n\nimport _ \"example.com/w/b\"\n",
		"d/d.go":                 "package d\n",
		"other/go.mod":           "module example.com/other\n",
		"other/o.go":             "package o\n\nimport _ \"example.com/w/a\"\n",
		"modules/m/README.md":    "a page\n",
		"modules/m/fixture/f.go": "package fixture\n\nimport _ \"example.com/w/d\"\n",
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestAChangedPackageBringsItsImportersAndOnlyThem(t *testing.T) {
	root := world(t)
	for name, c := range map[string]struct {
		paths   []string
		deep    bool
		want    []string
		changed int
	}{
		"a go file: its package and its importer":        {[]string{"a/a.go"}, false, []string{"a", "b"}, 1},
		"a testdata file counts for its package":         {[]string{"a/testdata/case.txt"}, false, []string{"a", "b"}, 1},
		"deep: the importer's importer, by its test too": {[]string{"a/a.go"}, true, []string{"a", "b", "c"}, 1},
		"another module's file counts for none":          {[]string{"other/o.go"}, false, []string{}, 0},
		"a page counts for none":                         {[]string{"modules/m/README.md"}, false, []string{}, 0},
		"a package under modules/ is one":                {[]string{"d/d.go"}, false, []string{"d", "modules/m/fixture"}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			got, changed, err := Affected(root, "", c.deep, c.paths)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) || changed != c.changed {
				t.Errorf("Affected(%v) = %v (%d changed), want %v (%d)", c.paths, got, changed, c.want, c.changed)
			}
		})
	}
}

// With no path given, git says what changed: the working tree and the
// untracked files against HEAD, or everything since a ref.
func TestWithNoPathGitSaysWhatChanged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := world(t)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "one")
	if got, _, _ := Affected(root, "", false, nil); len(got) != 0 {
		t.Errorf("a clean tree: %v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "new.go"), []byte("package d\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _, err := Affected(root, "", false, nil)
	if err != nil || !reflect.DeepEqual(got, []string{"d", "modules/m/fixture"}) {
		t.Errorf("an untracked file: %v %v", got, err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "two")
	if got, _, _ := Affected(root, "HEAD~1", false, nil); !reflect.DeepEqual(got, []string{"d", "modules/m/fixture"}) {
		t.Errorf("since HEAD~1: %v", got)
	}
}

// An importer slower than the bound in the last full run's report is left out
// and named; a package that changed itself runs however slow.
func TestASlowImporterIsLeftOutNeverAChangedPackage(t *testing.T) {
	root := world(t)
	report := filepath.Join(t.TempDir(), "full.txt")
	body := "# commit abc (HEAD: x)\n" +
		"ok  \texample.com/w/a\t300.0s\n" +
		"ok  \texample.com/w/b\t200.5s\n" +
		"FAIL\texample.com/w/c\t500.0s\n" +
		"ok  \texample.com/w/d\t(cached)\n" +
		"ok  \texample.com/other\t900.0s\n"
	if err := os.WriteFile(report, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	times, err := ReadTimes(report, "example.com/w")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]float64{"a": 300, "b": 200.5, "c": 500}; !reflect.DeepEqual(times, want) {
		t.Fatalf("ReadTimes = %v, want %v", times, want)
	}
	pkgs, changed, _, err := affected(root, "", true, []string{"a/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	kept, skipped := SkipSlow(pkgs, changed, times, 120)
	if !reflect.DeepEqual(kept, []string{"a"}) {
		t.Errorf("kept %v, want [a]: the changed package runs, its slow importers do not", kept)
	}
	if want := []Skipped{{"b", 200.5}, {"c", 500}}; !reflect.DeepEqual(skipped, want) {
		t.Errorf("skipped %v, want %v", skipped, want)
	}
	if kept, skipped := SkipSlow(pkgs, changed, times, 1000); len(skipped) != 0 || len(kept) != 3 {
		t.Errorf("under the bound: kept %v, skipped %v", kept, skipped)
	}
}

// wide writes a module where w is imported by five packages: two of its own
// module (w/x, w/y), two of another (z1, z2), and two whole-repo suites
// (internal/test/suite, tests only; internal/test/lib holds library code and
// is no suite). The owning modules come from modules/<m>/objects/<m>.cwm.yaml.
func wide(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	imp := func(pkg string) string { return "package " + pkg + "\n\nimport _ \"example.com/w/w\"\n" }
	files := map[string]string{
		"go.mod":                               "module example.com/w\n\ngo 1.22\n",
		"w/w.go":                               "package w\n",
		"w/x/x.go":                             imp("x"),
		"w/y/y_test.go":                        imp("y"),
		"z1/z.go":                              imp("z1"),
		"z2/z.go":                              imp("z2"),
		"internal/test/suite/s_test.go":        imp("suite"),
		"internal/test/lib/lib.go":             imp("lib"),
		"modules/own/objects/own.cwm.yaml":     "id: own\ncapabilities:\n  code:\n    code_packages:\n      - w\n      - internal/test\n    kind: tool\nmodule: {}\n",
		"modules/other/objects/other.cwm.yaml": "id: other\r\ncapabilities:\r\n  code:\r\n    code_packages:\r\n      - z1\r\n      - \"z2\"\r\n",
		"modules/other/README.md":              "a page\n",
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The quick lane stays quick with no full report: a
// whole-repo suite is the full lane's unless it changed itself, and a changed
// package with many importers brings only those of its own module.
func TestTheQuickLaneLeavesWholeRepoSuitesAndForeignImportersOfAWidePackage(t *testing.T) {
	root := wide(t)
	owners, err := ReadOwners(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"w": "own", "internal/test": "own", "z1": "other", "z2": "other"}; !reflect.DeepEqual(owners, want) {
		t.Fatalf("ReadOwners = %v, want %v", owners, want)
	}
	if got := OwnerOf(owners, "w/x"); got != "own" {
		t.Errorf("OwnerOf(w/x) = %q", got)
	}
	run := func(paths []string, fanout int) ([]string, []string) {
		t.Helper()
		pkgs, changed, tr, err := affected(root, "", false, paths)
		if err != nil {
			t.Fatal(err)
		}
		kept, left := trim(tr, pkgs, changed, owners, fanout)
		var names []string
		for _, l := range left {
			names = append(names, l.Pkg)
		}
		return kept, names
	}
	for name, c := range map[string]struct {
		paths      []string
		fanout     int
		kept, left []string
	}{
		"under the bound: every importer but the suite": {[]string{"w/w.go"}, 20, []string{"internal/test/lib", "w", "w/x", "w/y", "z1", "z2"}, []string{"internal/test/suite"}},
		"past the bound: its own module's importers":    {[]string{"w/w.go"}, 3, []string{"internal/test/lib", "w", "w/x", "w/y"}, []string{"internal/test/suite", "z1", "z2"}},
		"no bound (-deep): only the suite left":         {[]string{"w/w.go"}, 0, []string{"internal/test/lib", "w", "w/x", "w/y", "z1", "z2"}, []string{"internal/test/suite"}},
		"a changed suite runs":                          {[]string{"internal/test/suite/s_test.go"}, 3, []string{"internal/test/suite"}, nil},
		"a changed package of another module runs":      {[]string{"w/w.go", "z1/z.go"}, 3, []string{"internal/test/lib", "w", "w/x", "w/y", "z1"}, []string{"internal/test/suite", "z2"}},
	} {
		t.Run(name, func(t *testing.T) {
			kept, left := run(c.paths, c.fanout)
			if !reflect.DeepEqual(kept, c.kept) || !reflect.DeepEqual(left, c.left) {
				t.Errorf("kept %v, left %v; want kept %v, left %v", kept, left, c.kept, c.left)
			}
		})
	}
}
