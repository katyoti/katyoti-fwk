// SPDX-License-Identifier: Apache-2.0

// Command test-affected names the Go packages a change touches:
// the packages of the files changed - the working tree and the untracked
// files against HEAD, or everything since a ref - and the packages importing
// them (direct importers; -deep: every dependent). A file that is not Go
// (testdata, an embedded file) counts for the nearest folder holding Go
// files; a folder of another Go module (test_files, third_party) counts for
// none.
//
// It prints one "./<folder>/" per line, sorted; scripts/test-affected.sh runs
// them in the quick lane. One process, one walk of the sources: the shell
// version cost minutes in process starts on Windows.
//
// Usage: go run ./tools/test-affected [-since <ref>] [-deep] [-root <dir>] [-slow <report> [-bound <seconds>]] [paths...]
//
// Paths given name the changed files themselves (the files a step changed, in
// a working tree others change too); git is not asked then.
//
// With -slow (a timing report of a full run, bin/test-runs/full-*.txt), an
// importer that took more than -bound seconds there (default 120) is left out
// and named on stderr: an everyday run stays minutes long, and the slow
// package's owner is told to tag its heavy tests long. A
// package that changed itself always runs.
//
// Two more rules keep the everyday run minutes long with no report at all
// , both lifted by -all:
//   - a whole-repo suite - a package under internal/test/ holding tests only
//     (internal/test/arch, internal/test/catalog, ...) - is the full lane's: it
//     is left out and named unless it changed itself;
//   - a changed package with more than -fanout importers (default 20) brings
//     only its importers of the same owning module (the module whose
//     code_packages, in modules/<m>/objects/<m>.cwm.yaml, holds it - the
//     longest folder wins); the others are left out and named for the full
//     lane. Not under -deep, which asks for every dependent.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func main() {
	since := flag.String("since", "", "the changes since this ref (committed ones included)")
	deep := flag.Bool("deep", false, "every package depending on a changed one, not only its importers")
	root := flag.String("root", ".", "the repository root (holds go.mod)")
	slow := flag.String("slow", "", "a full run's timing report: importers slower than -bound there are left out")
	bound := flag.Float64("bound", 120, "seconds: an importer slower than this in the -slow report is left out")
	fanout := flag.Int("fanout", 20, "a changed package with more importers brings only those of its owning module")
	all := flag.Bool("all", false, "leave out no whole-repo suite and no importer of a wide package")
	flag.Parse()
	pkgs, changed, t, err := affected(*root, *since, *deep, flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "test-affected:", err)
		os.Exit(2)
	}
	if !*all {
		owners, err := ReadOwners(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "test-affected: the owning modules:", err)
			os.Exit(2)
		}
		fan := *fanout
		if *deep {
			fan = 0
		}
		var left []Left
		pkgs, left = trim(t, pkgs, changed, owners, fan)
		for _, l := range left {
			fmt.Fprintf(os.Stderr, "test-affected: skipped ./%s/ - %s: the full lane runs it, or run it with --all\n", l.Pkg, l.Why)
		}
	}
	if *slow != "" {
		module, err := modulePath(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "test-affected:", err)
			os.Exit(2)
		}
		times, err := ReadTimes(*slow, module)
		if err != nil {
			fmt.Fprintln(os.Stderr, "test-affected: the slow report:", err)
			os.Exit(2)
		}
		var skipped []Skipped
		pkgs, skipped = SkipSlow(pkgs, changed, times, *bound)
		for _, k := range skipped {
			fmt.Fprintf(os.Stderr, "test-affected: skipped ./%s/ - an importer that took %.0fs in %s (over %.0fs): tag its heavy tests long, or run it with --all\n",
				k.Pkg, k.Seconds, filepath.Base(*slow), *bound)
		}
	}
	fmt.Fprintf(os.Stderr, "test-affected: %d packages (%d changed)\n", len(pkgs), len(changed))
	for _, p := range pkgs {
		fmt.Printf("./%s/\n", p)
	}
}

// Affected answers the folders (slash-separated, from root) of the packages
// to test, and how many of them changed themselves: those of the paths given
// (from root), else those of git's changes.
func Affected(root, since string, deep bool, paths []string) ([]string, int, error) {
	pkgs, changed, _, err := affected(root, since, deep, paths)
	return pkgs, len(changed), err
}

// affected is Affected with the changed packages named, and the tree read.
func affected(root, since string, deep bool, paths []string) ([]string, map[string]bool, *tree, error) {
	module, err := modulePath(root)
	if err != nil {
		return nil, nil, nil, err
	}
	files := make([]string, 0, len(paths))
	for _, p := range paths {
		files = append(files, path.Clean(filepath.ToSlash(p)))
	}
	if len(files) == 0 {
		if files, err = changedFiles(root, since); err != nil {
			return nil, nil, nil, err
		}
	}
	tree, err := scan(root, module)
	if err != nil {
		return nil, nil, nil, err
	}
	seen := map[string]bool{}
	var frontier []string
	for _, f := range files {
		if p, ok := tree.packageOf(f); ok && !seen[p] {
			seen[p] = true
			frontier = append(frontier, p)
		}
	}
	changed := map[string]bool{}
	for _, p := range frontier {
		changed[p] = true
	}
	for len(frontier) > 0 {
		var next []string
		for _, p := range frontier {
			for _, imp := range tree.importers[p] {
				if !seen[imp] {
					seen[imp] = true
					next = append(next, imp)
				}
			}
		}
		if !deep {
			break
		}
		frontier = next
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, changed, tree, nil
}

// Left is a package the everyday run leaves to the full lane, and why.
type Left struct {
	Pkg string
	Why string
}

// trim leaves out of pkgs what the full lane runs: the
// whole-repo suites, and, of a changed package with more than fanout
// importers (0: no bound), the importers another module owns. A changed
// package always runs; an importer that any changed package keeps stays, and
// a package reached otherwise (-deep) runs.
func trim(t *tree, pkgs []string, changed map[string]bool, owners map[string]string, fanout int) ([]string, []Left) {
	keep := map[string]bool{}
	why := map[string]string{}
	for p := range changed {
		keep[p] = true
	}
	for p := range changed {
		imps := t.importers[p]
		wide := fanout > 0 && len(imps) > fanout
		owner := OwnerOf(owners, p)
		for _, imp := range imps {
			switch {
			case t.wholeRepoSuite(imp):
				why[imp] = "a whole-repo suite"
			case wide && OwnerOf(owners, imp) != owner:
				if why[imp] == "" {
					why[imp] = fmt.Sprintf("one of the %d importers of ./%s/, outside %s", len(imps), p, moduleName(owner))
				}
			default:
				keep[imp] = true
			}
		}
	}
	var kept []string
	var left []Left
	for _, p := range pkgs {
		if why[p] != "" && !keep[p] {
			left = append(left, Left{Pkg: p, Why: why[p]})
			continue
		}
		kept = append(kept, p)
	}
	return kept, left
}

func moduleName(m string) string {
	if m == "" {
		return "its owning module (none)"
	}
	return "its module " + m
}

// ReadOwners reads, from each modules/<m>/objects/<m>.cwm.yaml, the folders
// its code_packages list names: folder -> module. A line scan of that one
// list, not a YAML decode: the tool stays free of the workspace's packages.
// No modules/ folder: no owner.
func ReadOwners(root string) (map[string]string, error) {
	owners := map[string]string{}
	entries, err := os.ReadDir(filepath.Join(root, "modules"))
	if err != nil {
		if os.IsNotExist(err) {
			return owners, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, "modules", e.Name(), "objects", e.Name()+".cwm.yaml"))
		if err != nil {
			continue // a folder that is not a module
		}
		in, indent := false, 0
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimRight(line, "\r")
			trimmed := strings.TrimSpace(line)
			lead := len(line) - len(strings.TrimLeft(line, " "))
			if trimmed == "code_packages:" {
				in, indent = true, lead
				continue
			}
			if !in || trimmed == "" {
				continue
			}
			if item, ok := strings.CutPrefix(trimmed, "- "); ok && lead >= indent {
				owners[path.Clean(strings.Trim(strings.TrimSpace(item), `"'`))] = e.Name()
				continue
			}
			in = false
		}
	}
	return owners, nil
}

// OwnerOf is the module owning a package folder: the longest code_packages
// folder holding it; "" for none.
func OwnerOf(owners map[string]string, pkg string) string {
	best, owner := -1, ""
	for dir, m := range owners {
		if (pkg == dir || strings.HasPrefix(pkg, dir+"/")) && len(dir) > best {
			best, owner = len(dir), m
		}
	}
	return owner
}

// Skipped is a package left out of an everyday run, and its time in the
// report that left it out.
type Skipped struct {
	Pkg     string
	Seconds float64
}

// SkipSlow leaves out the packages that took more than bound seconds in times,
// unless they changed themselves: those always run.
func SkipSlow(pkgs []string, changed map[string]bool, times map[string]float64, bound float64) ([]string, []Skipped) {
	var kept []string
	var skipped []Skipped
	for _, p := range pkgs {
		if t, ok := times[p]; ok && t > bound && !changed[p] {
			skipped = append(skipped, Skipped{Pkg: p, Seconds: t})
			continue
		}
		kept = append(kept, p)
	}
	return kept, skipped
}

// ReadTimes reads a go test run's package lines ("ok  <import path>  12.3s",
// "FAIL ...") into the seconds of each package folder of module.
func ReadTimes(report, module string) (map[string]float64, error) {
	b, err := os.ReadFile(report)
	if err != nil {
		return nil, err
	}
	times := map[string]float64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || (f[0] != "ok" && f[0] != "FAIL") || !strings.HasSuffix(f[2], "s") {
			continue
		}
		secs, err := strconv.ParseFloat(strings.TrimSuffix(f[2], "s"), 64)
		if err != nil {
			continue
		}
		if rest, ok := strings.CutPrefix(f[1], module+"/"); ok {
			times[rest] = secs
		}
	}
	return times, nil
}

func modulePath(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("no module line in %s/go.mod", root)
}

func changedFiles(root, since string) ([]string, error) {
	diff := []string{"-c", "core.safecrlf=false", "diff", "--name-only", "HEAD", "--"}
	if since != "" {
		diff = []string{"-c", "core.safecrlf=false", "diff", "--name-only", since, "--"}
	}
	var files []string
	for _, args := range [][]string{diff, {"ls-files", "--others", "--exclude-standard"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		var errb bytes.Buffer
		cmd.Stderr = &errb
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
		}
		for _, l := range strings.Split(string(out), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				files = append(files, l)
			}
		}
	}
	return files, nil
}

// tree is the module's Go packages: the folders holding .go files, and for
// each package the packages importing it (tests' imports included).
type tree struct {
	goDirs    map[string]bool     // folders of this module holding .go files
	library   map[string]bool     // folders holding a .go file that is not a test
	otherMods map[string]bool     // folders holding another go.mod
	importers map[string][]string // package folder -> importing package folders
}

func scan(root, module string) (*tree, error) {
	t := &tree{goDirs: map[string]bool{}, library: map[string]bool{}, otherMods: map[string]bool{}, importers: map[string][]string{}}
	imports := map[string]map[string]bool{} // importer folder -> imported folders
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".katyoti", "dist", "bin":
				return filepath.SkipDir
			}
			if rel != "." {
				if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
					t.otherMods[rel] = true
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		dir := path.Dir(rel)
		t.goDirs[dir] = true
		if !strings.HasSuffix(d.Name(), "_test.go") {
			t.library[dir] = true
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, is := range f.Imports {
			ip, _ := strconv.Unquote(is.Path.Value)
			if rest, ok := strings.CutPrefix(ip, module+"/"); ok {
				if imports[dir] == nil {
					imports[dir] = map[string]bool{}
				}
				imports[dir][rest] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for importer, set := range imports {
		for imported := range set {
			if imported != importer {
				t.importers[imported] = append(t.importers[imported], importer)
			}
		}
	}
	return t, nil
}

// wholeRepoSuite says a package is one of the full lane's whole-repo suites:
// under internal/test/, tests only (no library code another package imports).
func (t *tree) wholeRepoSuite(pkg string) bool {
	return strings.HasPrefix(pkg, "internal/test/") && t.goDirs[pkg] && !t.library[pkg]
}

// packageOf is the package folder a changed file belongs to: the nearest
// folder holding .go files, none inside another module.
func (t *tree) packageOf(file string) (string, bool) {
	for d := path.Dir(file); d != "." && d != "/" && d != ""; d = path.Dir(d) {
		if t.otherMods[d] {
			return "", false
		}
		if t.goDirs[d] {
			return d, true
		}
	}
	return "", false
}
