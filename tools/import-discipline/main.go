// SPDX-License-Identifier: Apache-2.0

// import-discipline enforces the repo's import-boundary rules.
//
// Boundaries are declared as data in rules.yaml, not as code: adding a rule is
// editing YAML. This replaces the per-boundary checkers that preceded it, which
// were the same algorithm with different constants.
//
// # Two modes, two questions
//
// `direct` asks WHO WROTE THE IMPORT — authorship discipline. A package that
// names a forbidden dependency in its own import block is the violation.
//
// `closure` asks WHO ENDS UP LINKING IT — link cost. A package whose full
// dependency graph reaches a forbidden dependency is the violation, even if it
// never names it. This is the mode that catches weight arriving one hop away,
// through an innocent-looking import of a package that is itself heavy. A
// direct-only check passes such a package clean.
//
// Run via: go run ./tools/import-discipline [-rule <name>[,<name>…]]
// Exits non-zero on violation; CI uses this in the `check` Make target.
// -rule checks only the named rules — the probe a single `rule` object runs.
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed rules.yaml
var rulesYAML []byte

// selfPkg names this tool, which mentions forbidden paths as strings without
// importing them.
const selfPkg = "github.com/katyoti/katyoti-fwk/tools/import-discipline"

type ruleSet struct {
	Rules []rule `yaml:"rules"`
}

type rule struct {
	Name string `yaml:"name"`
	Doc  string `yaml:"doc"`
	Hint string `yaml:"hint"`
	// From restricts which packages the rule applies to. Empty means every
	// package. Use it for "packages shaped like X must not import Y", which
	// an except-list cannot express.
	From   []string `yaml:"from"`
	Forbid []string `yaml:"forbid"`
	Except []string `yaml:"except"`
	Mode   string   `yaml:"mode"`
	// Baseline lists packages that violate this rule TODAY. They are reported
	// as known and do not fail the build; anything else does. This is how a
	// boundary is adopted on a codebase that does not yet satisfy it: the rule
	// starts preventing regressions immediately, and the list is the backlog.
	Baseline     []string `yaml:"baseline"`
	IncludeTests bool     `yaml:"include_tests"`

	Budget *budget `yaml:"budget"`
}

// budget caps how much a package may end up linking. Expressed as a count of
// first-party packages in the dependency closure, which is the number that
// tracks binary weight.
type budget struct {
	Packages []string `yaml:"packages"`
	Prefix   string   `yaml:"prefix"`
	Max      int      `yaml:"max"`
}

type pkgInfo struct {
	ImportPath  string   `json:"ImportPath"`
	Imports     []string `json:"Imports"`
	TestImports []string `json:"TestImports"`
	Deps        []string `json:"Deps"`
}

// parseRules decodes the embedded rule file. Separated from main so a test can
// assert the shipped rules are well-formed — a typo would otherwise surface only
// as a rule silently never firing.
func parseRules() (ruleSet, error) {
	var rs ruleSet
	if err := yaml.Unmarshal(rulesYAML, &rs); err != nil {
		return rs, err
	}
	for i := range rs.Rules {
		if x, ok := privateEntries[rs.Rules[i].Name]; ok {
			rs.Rules[i].Forbid = append(rs.Rules[i].Forbid, x.Forbid...)
			rs.Rules[i].Except = append(rs.Rules[i].Except, x.Except...)
			rs.Rules[i].Baseline = append(rs.Rules[i].Baseline, x.Baseline...)
		}
	}
	return rs, nil
}

// privateEntries adds to a rule the packages kept out of the open-source
// export (private_entries.go, built without oss): rules.yaml is published and
// names none of them. Keyed by rule name.
var privateEntries = map[string]rule{}

func main() {
	only := flag.String("rule", "", "check only these rules (comma-separated names)")
	flag.Parse()
	rs, err := parseRules()
	if err != nil {
		fmt.Fprintln(os.Stderr, "import-discipline: parse rules.yaml:", err)
		os.Exit(2)
	}
	if rs.Rules, err = selectRules(rs.Rules, *only); err != nil {
		fmt.Fprintln(os.Stderr, "import-discipline:", err)
		os.Exit(2)
	}

	pkgs, lerr := listPackages()
	if lerr != nil {
		fmt.Fprintln(os.Stderr, "import-discipline:", lerr)
		os.Exit(2)
	}

	var findings, known []string
	for _, r := range rs.Rules {
		f, k := r.check(pkgs)
		findings = append(findings, f...)
		known = append(known, k...)
	}

	if len(known) > 0 {
		sort.Strings(known)
		fmt.Fprintf(os.Stderr, "import-discipline: %d known violation(s) — the backlog, not a regression:\n", len(known))
		for _, k := range known {
			fmt.Fprintln(os.Stderr, "  "+k)
		}
		fmt.Fprintln(os.Stderr)
	}

	if len(findings) > 0 {
		sort.Strings(findings)
		for _, f := range findings {
			fmt.Fprintln(os.Stderr, f)
		}
		fmt.Fprintf(os.Stderr, "\nimport-discipline: %d NEW violation(s) across %d packages\n",
			len(findings), len(pkgs))
		os.Exit(1)
	}
	fmt.Printf("import-discipline: OK (%d rules, %d packages, %d known)\n",
		len(rs.Rules), len(pkgs), len(known))
}

// selectRules keeps the rules named in a comma-separated list; an empty list
// keeps them all, and a name no rule has is an error — a probe naming a rule
// that was renamed must fail, never pass on nothing.
func selectRules(all []rule, names string) ([]rule, error) {
	if strings.TrimSpace(names) == "" {
		return all, nil
	}
	var out []rule
	for _, n := range strings.Split(names, ",") {
		n = strings.TrimSpace(n)
		found := false
		for _, r := range all {
			if r.Name == n {
				out = append(out, r)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no rule named %q in rules.yaml", n)
		}
	}
	return out, nil
}

func listPackages() ([]pkgInfo, error) {
	out, err := exec.Command("go", "list", "-json", "./...").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("go list failed: %s", ee.Stderr)
		}
		return nil, fmt.Errorf("go list failed: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var pkgs []pkgInfo
	for {
		var p pkgInfo
		if err := dec.Decode(&p); err != nil {
			break
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// check returns (new violations, known violations). Known ones are on the
// rule's baseline and do not fail the build.
func (r rule) check(pkgs []pkgInfo) (newV, known []string) {
	if r.Budget != nil {
		return r.checkBudget(pkgs), nil
	}

	record := func(pkg, msg string) {
		if matchAny(pkg, r.Baseline) {
			known = append(known, fmt.Sprintf("[%s] %s", r.Name, pkg))
			return
		}
		newV = append(newV, msg)
	}

	for _, p := range pkgs {
		if p.ImportPath == selfPkg || matchAny(p.ImportPath, r.Except) {
			continue
		}
		if len(r.From) > 0 && !matchAny(p.ImportPath, r.From) {
			continue
		}

		// direct: only what this package names itself.
		// closure: everything it ends up linking, which is what costs weight.
		candidates := p.Imports
		if r.Mode == "closure" {
			candidates = p.Deps
		}
		for _, imp := range candidates {
			if matchAny(imp, r.Forbid) {
				record(p.ImportPath, r.format(p.ImportPath, imp, ""))
				break // one finding per package per rule — the list, not the count, is the signal
			}
		}

		if r.IncludeTests {
			for _, imp := range p.TestImports {
				if matchAny(imp, r.Forbid) {
					record(p.ImportPath, r.format(p.ImportPath, imp, " (test)"))
					break
				}
			}
		}
	}
	return newV, known
}

func (r rule) checkBudget(pkgs []pkgInfo) []string {
	var out []string
	for _, p := range pkgs {
		if p.ImportPath == selfPkg || !matchAny(p.ImportPath, r.Budget.Packages) {
			continue
		}
		n := 0
		for _, d := range p.Deps {
			if strings.HasPrefix(d, r.Budget.Prefix) {
				n++
			}
		}
		if n > r.Budget.Max {
			out = append(out, fmt.Sprintf(
				"VIOLATION [%s]: %s links %d first-party packages (budget %d)%s",
				r.Name, p.ImportPath, n, r.Budget.Max, r.suffix()))
		}
	}
	return out
}

func (r rule) format(pkg, imp, kind string) string {
	return fmt.Sprintf("VIOLATION [%s]%s: %s imports %s%s", r.Name, kind, pkg, imp, r.suffix())
}

func (r rule) suffix() string {
	var b strings.Builder
	if r.Hint != "" {
		fmt.Fprintf(&b, "\n    → %s", strings.TrimSpace(r.Hint))
	}
	if r.Doc != "" {
		fmt.Fprintf(&b, "\n    see %s", r.Doc)
	}
	return b.String()
}

// match reports whether importPath satisfies one pattern. Segment-wise, with
// four forms and no glob engine — the repo's boundaries are subtrees, single
// segments and package names, and a real glob syntax would invite patterns
// nobody can predict.
//
//	some/pkg/...   the package and everything beneath it
//	some/*/pkg     `*` matches exactly one segment
//	.../name       any package whose final element is `name`
//	some/pkg       that package exactly
func match(importPath, pattern string) bool {
	// `.../name` — a bare final element, distinct from a mid-pattern `...`.
	if rest, ok := strings.CutPrefix(pattern, ".../"); ok && !strings.Contains(rest, "/") {
		return importPath == rest || strings.HasSuffix(importPath, "/"+rest)
	}

	pp := strings.Split(pattern, "/")
	ip := strings.Split(importPath, "/")
	for i, seg := range pp {
		if seg == "..." {
			// Subtree: `a/b/...` matches `a/b` and everything beneath it.
			return len(ip) >= i
		}
		if i >= len(ip) {
			return false
		}
		if seg == "*" || seg == ip[i] {
			continue
		}
		return false
	}
	return len(pp) == len(ip)
}

func matchAny(importPath string, patterns []string) bool {
	for _, p := range patterns {
		if match(importPath, p) {
			return true
		}
	}
	return false
}
