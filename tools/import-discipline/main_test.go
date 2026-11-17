// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

const pre = "github.com/katyoti/katyoti-fwk/"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
		why     string
	}{
		// exact
		{"a/b", "a/b", true, "exact match"},
		{"a/b", "a/bc", false, "exact must not match a longer segment"},
		{"a/b", "a/b/c", false, "exact must not match a subpackage"},

		// subtree
		{"a/b/...", "a/b", true, "subtree includes the root itself"},
		{"a/b/...", "a/b/c", true, "subtree includes children"},
		{"a/b/...", "a/b/c/d", true, "subtree includes grandchildren"},
		{"a/b/...", "a/bc", false, "subtree must not match a sibling with a shared prefix"},
		{"a/b/...", "a", false, "subtree must not match a parent"},

		// single segment
		{"a/*/c", "a/b/c", true, "* matches one segment"},
		{"a/*/c", "a/c", false, "* requires a segment to be present"},
		{"a/*/c", "a/b/x/c", false, "* must not span separators"},
		{"a/*/c/...", "a/b/c/d", true, "* composes with a subtree"},

		// final element
		{".../remote", "x/y/remote", true, "final element matches at depth"},
		{".../remote", "remote", true, "final element matches a bare path"},
		{".../remote", "x/remote/y", false, "final element must be final"},
		{".../remote", "x/y/myremote", false, "final element must be a whole segment"},
	}
	for _, c := range cases {
		if got := match(c.path, c.pattern); got != c.want {
			t.Errorf("match(%q, %q) = %v, want %v — %s", c.path, c.pattern, got, c.want, c.why)
		}
	}
}

// TestDirectVsClosure is the distinction the whole tool exists for: a package
// can name only permitted imports and still end up linking a forbidden one, one
// hop away. `direct` passes it; `closure` catches it.
func TestDirectVsClosure(t *testing.T) {
	pkgs := []pkgInfo{{
		ImportPath: pre + "innocent",
		Imports:    []string{pre + "middleman"},                     // nothing forbidden here
		Deps:       []string{pre + "middleman", pre + "heavy/core"}, // but it links this
	}}
	r := rule{
		Name:   "no-heavy",
		Forbid: []string{pre + "heavy/..."},
	}

	r.Mode = "direct"
	if got, _ := r.check(pkgs); len(got) != 0 {
		t.Errorf("direct mode should not flag a transitive dependency, got %v", got)
	}

	r.Mode = "closure"
	got, _ := r.check(pkgs)
	if len(got) != 1 {
		t.Fatalf("closure mode should flag the transitive dependency, got %d findings", len(got))
	}
	if !strings.Contains(got[0], "innocent") || !strings.Contains(got[0], "heavy/core") {
		t.Errorf("finding should name both the package and the dependency: %q", got[0])
	}
}

func TestFromRestrictsScope(t *testing.T) {
	pkgs := []pkgInfo{
		{ImportPath: pre + "internal/extensions/x/internal/core", Imports: []string{pre + "internal/core/em"}},
		{ImportPath: pre + "internal/extensions/x/local", Imports: []string{pre + "internal/core/em"}},
	}
	r := rule{
		Name:   "scoped",
		From:   []string{pre + "internal/extensions/*/internal/..."},
		Forbid: []string{pre + "internal/core/em"},
	}
	got, _ := r.check(pkgs)
	if len(got) != 1 {
		t.Fatalf("want exactly the in-scope package flagged, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "internal/core") || strings.Contains(got[0], "/local") {
		t.Errorf("wrong package flagged: %q", got[0])
	}
}

// TestBaseline: a known violation is reported but must not fail the build, while
// anything new must. This is what lets a boundary be adopted on a codebase that
// does not yet satisfy it.
func TestBaseline(t *testing.T) {
	pkgs := []pkgInfo{
		{ImportPath: pre + "old/offender", Imports: []string{pre + "banned/x"}},
		{ImportPath: pre + "new/offender", Imports: []string{pre + "banned/x"}},
	}
	r := rule{
		Name:     "b",
		Forbid:   []string{pre + "banned/..."},
		Baseline: []string{pre + "old/offender"},
	}
	newV, known := r.check(pkgs)
	if len(known) != 1 || !strings.Contains(known[0], "old/offender") {
		t.Errorf("baselined package should be reported as known, got %v", known)
	}
	if len(newV) != 1 || !strings.Contains(newV[0], "new/offender") {
		t.Errorf("non-baselined package should fail, got %v", newV)
	}
}

func TestIncludeTests(t *testing.T) {
	pkgs := []pkgInfo{{
		ImportPath:  pre + "p",
		Imports:     []string{pre + "ok"},
		TestImports: []string{pre + "banned/x"},
	}}
	r := rule{Name: "t", Forbid: []string{pre + "banned/..."}}

	if got, _ := r.check(pkgs); len(got) != 0 {
		t.Errorf("include_tests=false should ignore a test-only import, got %v", got)
	}
	r.IncludeTests = true
	got, _ := r.check(pkgs)
	if len(got) != 1 || !strings.Contains(got[0], "(test)") {
		t.Errorf("include_tests=true should flag it and label it, got %v", got)
	}
}

func TestBudget(t *testing.T) {
	deps := func(n int) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, pre+"dep"+string(rune('a'+i)))
		}
		return append(out, "gopkg.in/yaml.v3") // third-party must not count
	}
	pkgs := []pkgInfo{
		{ImportPath: pre + "x/remote", Deps: deps(3)},
		{ImportPath: pre + "y/remote", Deps: deps(9)},
		{ImportPath: pre + "z/local", Deps: deps(50)}, // out of scope
	}
	r := rule{
		Name:   "budget",
		Budget: &budget{Packages: []string{".../remote"}, Prefix: pre, Max: 5},
	}
	got, _ := r.check(pkgs)
	if len(got) != 1 {
		t.Fatalf("only the over-budget in-scope package should fail, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "y/remote") || !strings.Contains(got[0], "links 9") {
		t.Errorf("finding should name the package and the count: %q", got[0])
	}
}

// TestEmbeddedRulesParse guards the shipped rule file: a typo in rules.yaml
// would otherwise only surface as a rule silently doing nothing.
func TestEmbeddedRulesParse(t *testing.T) {
	rs, err := parseRules()
	if err != nil {
		t.Fatalf("rules.yaml does not parse: %v", err)
	}
	if len(rs.Rules) == 0 {
		t.Fatal("rules.yaml declares no rules")
	}
	for _, r := range rs.Rules {
		if r.Name == "" {
			t.Error("every rule needs a name")
		}
		if r.Budget == nil && len(r.Forbid) == 0 {
			t.Errorf("rule %q forbids nothing and sets no budget — it can never fire", r.Name)
		}
		if r.Mode != "" && r.Mode != "direct" && r.Mode != "closure" {
			t.Errorf("rule %q has unknown mode %q", r.Name, r.Mode)
		}
	}
}

// -rule keeps the named rules, all of them when empty, and refuses a name no
// rule has: a probe that names a renamed rule must fail, never pass on nothing.
func TestSelectRules(t *testing.T) {
	all := []rule{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	if got, err := selectRules(all, ""); err != nil || len(got) != 3 {
		t.Fatalf("empty selection = %v, %v; want every rule", got, err)
	}
	got, err := selectRules(all, "c, a")
	if err != nil || len(got) != 2 || got[0].Name != "c" || got[1].Name != "a" {
		t.Fatalf("selection c,a = %v, %v", got, err)
	}
	if _, err := selectRules(all, "a,nope"); err == nil {
		t.Fatal("an unknown rule name was accepted")
	}
}
