// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/lint"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// ModuleAudit v1 runs the perspectives we have automation for and
// writes a `review` of the module (the reviews pack) with a `finding`
// per thing found. v1 implements 4 of the 13 documented perspectives:
//
//   - `correctness` — runs go.Vet + go.Build over the module's
//     code_packages. Build failure / vet diagnostic → critical.
//   - `quality` — runs go.Test. Test failures → high. Skipped
//     tests → info.
//   - `package-layout` — runs go.DepCheck. Public→internal/local
//     import violations → high.
//   - `dead-code` — derived from go.PkgInventory. Packages with
//     zero exports + no imports anywhere → info.
//
// The other 9 perspectives (architecture, security, threat-model,
// red-team-architecture, performance, library-audit, ux-cli,
// documentation, governance) are intentionally NOT automated in v1
// — they require LLM judgement and are the job of the `reviewer`
// persona running with this verb's output as a starting point.
//
// A v1 audit can therefore catch every CI-level breakage (build
// errors, test failures, lint, layout) but doesn't replace human
// (or LLM-driven) review.
func (s *GocodeServiceImpl) ModuleAudit(ctx stdctx.Context, in api.ModuleAuditInput) (api.ModuleAuditOutput, error) {
	if in.ID == "" {
		return api.ModuleAuditOutput{}, fmt.Errorf("%w: ModuleAudit: id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModuleAuditOutput{}, fmt.Errorf("ModuleAudit: resolve CWMService: %w", err)
	}

	inputs, err := projection.LoadManifestInputs(ctx, cwm, in.ID)
	if err != nil {
		return api.ModuleAuditOutput{}, fmt.Errorf("ModuleAudit: load inputs: %w", err)
	}

	// Resolve the go-tool service so we can shell out to the real
	// toolchain. Resolved from context like every other inter-service
	// dependency — this verb does not hard-code a GoToolService impl.
	gotool, err := application.UseService[api.GoToolService](ctx)
	if err != nil {
		return api.ModuleAuditOutput{}, fmt.Errorf("ModuleAudit: resolve GoToolService: %w", err)
	}

	// 1. The automated perspectives and the lint detectors (collectAuditFindings).
	sc := moduleScope(inputs.Module)
	sc.dir = projectRootOf(ctx, cwm, in.ID)
	findings := collectAuditFindings(ctx, gotool, inputs, sc)

	var counts api.ModuleAuditOutput
	for _, f := range findings {
		switch f.Severity {
		case api.FindingSeverityCritical:
			counts.CriticalCount++
		case api.FindingSeverityHigh:
			counts.HighCount++
		case api.FindingSeverityMedium:
			counts.MediumCount++
		case api.FindingSeverityLow:
			counts.LowCount++
		}
	}

	// 2. Filed as the module's go-checks review by the reviews service: the
	//    review closed (an audit is written in one go), its findings under the
	//    module followed by fingerprint, the older reviews of the type folded.
	filed, err := fileGoChecks(ctx, reviewsapi.FileInput{
		Object: in.ID, Type: projection.GoChecksReviewType, Trigger: reviewsapi.TriggerAutomatic,
		RequestedBy: api.AuditReviewer, Reviewers: []string{api.AuditReviewer}, Result: checkResult(findings),
	})
	if err != nil {
		return counts, fmt.Errorf("ModuleAudit: file the review: %w", err)
	}
	counts.ReviewID = filed.Review
	return counts, nil
}

// fileGoChecks files a go-checks result through the reviews service: the one
// in this process when it is served here, else the one serving the universe
// (ktw-agents:reviews/file) through the action service.
func fileGoChecks(ctx stdctx.Context, in reviewsapi.FileInput) (reviewsapi.FileOutput, error) {
	if svc, err := application.UseService[reviewsapi.ReviewsService](ctx); err == nil && svc != nil {
		return svc.File(ctx, in)
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return reviewsapi.FileOutput{}, fmt.Errorf("no reviews service and no action service: %w", err)
	}
	input, err := json.Marshal(in)
	if err != nil {
		return reviewsapi.FileOutput{}, err
	}
	got, err := actions.Run(ctx, actionsapi.RunInput{URI: reviewsFileURI, Input: input})
	if err != nil {
		return reviewsapi.FileOutput{}, err
	}
	var out reviewsapi.FileOutput
	if err := json.Unmarshal(got.Value, &out); err != nil {
		return reviewsapi.FileOutput{}, fmt.Errorf("reviews/file answered no filing: %w", err)
	}
	return out, nil
}

// reviewsFileURI is the reviews service's File, served by ktw-agents.
const reviewsFileURI = "ktw-agents:reviews/file"

// collectAuditFindings runs the 4 automated perspectives, then the lint
// detectors — the audit captures lint as structured data per
// modules/code-gocode/concepts.md §4. ModuleAudit and ModuleReviewCheck (the
// go-checks review type's check) share it.
func collectAuditFindings(ctx stdctx.Context, gotool api.GoToolService, inputs projection.ManifestInputs, sc checkScope) []api.AuditFinding {
	var findings []api.AuditFinding
	if sc.dir == "" {
		// Run elsewhere, the tools find no module: every check would fail
		// on the folder, never on the code.
		return []api.AuditFinding{{
			Title:       "the module's project root is not known: the go tools were not run",
			Perspective: api.AuditPerspectiveCorrectness,
			Severity:    api.FindingSeverityHigh,
			FindingID:   "correctness-root-1",
			Body:        "The go checks run in the module's project root, as CWM resolves it; it could not be resolved, so no build, vet or test result would mean anything.",
		}}
	}
	findings = append(findings, bounded(ctx, api.AuditPerspectiveCorrectness, checkBounds.correctness, func(ctx stdctx.Context) []api.AuditFinding { return auditCorrectness(ctx, gotool, sc) })...)
	findings = append(findings, bounded(ctx, api.AuditPerspectiveQuality, checkBounds.quality, func(ctx stdctx.Context) []api.AuditFinding { return auditQuality(ctx, gotool, sc) })...)
	findings = append(findings, bounded(ctx, api.AuditPerspectivePackageLayout, checkBounds.layout, func(ctx stdctx.Context) []api.AuditFinding { return auditPackageLayout(ctx, gotool, sc) })...)
	findings = append(findings, bounded(ctx, api.AuditPerspectiveDeadCode, checkBounds.deadCode, func(ctx stdctx.Context) []api.AuditFinding { return auditDeadCode(ctx, gotool, sc) })...)
	findings = append(findings, bounded(ctx, api.AuditPerspectiveLint, checkBounds.lint, func(stdctx.Context) []api.AuditFinding { return auditLint(inputs) })...)
	return findings
}

// auditLint runs the lint detectors over the module's objects.
func auditLint(inputs projection.ManifestInputs) []api.AuditFinding {
	var findings []api.AuditFinding
	lintViols, _, _, _ := lint.Run(lint.Inputs{
		Module:        inputs.Module,
		UseCases:      inputs.UseCases,
		OpenDecisions: inputs.OpenDecisions,
	}, nil)
	for i, v := range lintViols {
		severity := api.FindingSeverityMedium
		if v.Severity == lint.SeverityError {
			severity = api.FindingSeverityHigh
		}
		findings = append(findings, api.AuditFinding{
			Title:       v.Reason,
			Perspective: api.AuditPerspectiveLint,
			Severity:    severity,
			FindingID:   fmt.Sprintf("lint-%d", i+1),
			Body:        v.FixSuggestion,
		})
	}
	return findings
}

// auditVerdict is the review's rating from what the audit found.
func auditVerdict(c api.ModuleAuditOutput) string {
	switch {
	case c.CriticalCount > 0:
		return "critical"
	case c.HighCount+c.MediumCount > 0:
		return "needs-attention"
	}
	return "good"
}

// checkScope is what the checks run on: the package roots (the dependency
// check runs per root) and the go tool's patterns (build, vet, test, the
// inventory). A module's scope is its code_packages with their sub-packages;
// a change's scope the packages its paths touch, without sub-packages.
type checkScope struct {
	roots    []string
	patterns []string
	// dir is the folder the go tools run in: the module's project root (its
	// patterns are relative to it), never the process's working folder.
	dir string
}

// projectRootOf is the project root of the module id, as CWM resolves it;
// empty when it is not known.
func projectRootOf(ctx stdctx.Context, cwm cwmapi.CWMService, id string) string {
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: id})
	if err != nil {
		return ""
	}
	return res.ProjectRoot
}

// moduleScope is the module's whole code.
func moduleScope(mod api.Module) checkScope {
	return checkScope{roots: moduleRoots(mod), patterns: modulePackages(mod)}
}

// changeScope is the module's packages a change's paths touch: the folder of
// each changed .go file under one of the module's roots, run from root (the
// project root; the working folder when empty). A folder that no longer holds
// a Go file - its package deleted by the change - is left out: the go tools
// would fail on it. Empty when the change touches no live Go package of the
// module.
func changeScope(mod api.Module, root string, paths []string) checkScope {
	roots := moduleRoots(mod)
	seen := map[string]bool{}
	sc := checkScope{dir: root}
	for _, p := range paths {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if !strings.HasSuffix(p, ".go") {
			continue
		}
		dir := "./" + strings.TrimPrefix(filepath.ToSlash(filepath.Dir(p)), "./")
		if seen[dir] {
			continue
		}
		if !holdsGoFile(filepath.Join(root, filepath.FromSlash(dir))) {
			seen[dir] = true
			continue
		}
		for _, r := range roots {
			if (dir == r || strings.HasPrefix(dir, r+"/")) && !seen[dir] {
				seen[dir] = true
				sc.roots = append(sc.roots, dir)
				sc.patterns = append(sc.patterns, dir)
			}
		}
	}
	sort.Strings(sc.roots)
	sort.Strings(sc.patterns)
	return sc
}

// holdsGoFile tells whether folder dir exists and holds a .go file.
func holdsGoFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

// checkBounds is how long each perspective may run before it is reported as
// an overrun rather than awaited: the whole check stays well within its
// method's timeout (module.ReviewCheck's TimeoutS).
var checkBounds = struct {
	correctness, quality, layout, deadCode, lint time.Duration
}{8 * time.Minute, 15 * time.Minute, 3 * time.Minute, 3 * time.Minute, 3 * time.Minute}

// bounded runs one perspective with its bound: the toolchain it calls is
// stopped at the deadline (its context), and a perspective still running then
// answers a finding saying it did not finish — never a hang.
func bounded(ctx stdctx.Context, p api.AuditPerspective, bound time.Duration, run func(stdctx.Context) []api.AuditFinding) []api.AuditFinding {
	ctx, cancel := stdctx.WithTimeout(ctx, bound)
	defer cancel()
	done := make(chan []api.AuditFinding, 1)
	go func() { done <- run(ctx) }()
	select {
	case got := <-done:
		if ctx.Err() == nil {
			return got
		}
	case <-ctx.Done():
	}
	return []api.AuditFinding{{
		Title:       fmt.Sprintf("%s did not finish within %s", p, bound),
		Perspective: p,
		Severity:    api.FindingSeverityHigh,
		FindingID:   string(p) + "-overrun-1",
		Body:        "The check was stopped at its bound; what it was running (a test that hangs, a tool waiting on a lock) did not end in time.",
	}}
}

// moduleRoots are the module's code_packages as local package roots
// (`./internal/extensions/x`): a declared path is repository-relative, and a
// bare `internal/extensions/x` would read as a standard-library import path.
// Empty when the module declares none.
func moduleRoots(mod api.Module) []string {
	var out []string
	for _, p := range mod.FacetPaths(api.FacetCode) {
		p = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(p), "/..."), "/")
		if p == "" || p == "." {
			continue
		}
		if !strings.HasPrefix(p, "./") && !strings.HasPrefix(p, "../") {
			p = "./" + p
		}
		out = append(out, p)
	}
	return out
}

// modulePackages are the go tool's patterns for the module's code: each root
// with its sub-packages, or the whole workspace when it declares none.
func modulePackages(mod api.Module) []string {
	roots := moduleRoots(mod)
	if len(roots) == 0 {
		return []string{"./..."}
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		out = append(out, r+"/...")
	}
	return out
}

// buildable keeps the patterns go build accepts: a "..." pattern (go skips
// the packages of tests only it matches) and a folder holding at least one
// Go file that is not a test.
func buildable(patterns []string) []string {
	var out []string
	for _, p := range patterns {
		if strings.Contains(p, "...") {
			out = append(out, p)
			continue
		}
		files, _ := filepath.Glob(filepath.Join(filepath.FromSlash(p), "*.go"))
		for _, f := range files {
			if !strings.HasSuffix(f, "_test.go") {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// leaveOutConstrained drops the named folders (patterns without "...", which
// the go tool skips on its own) whose Go files are all excluded by build
// constraints - a package of tests behind a tag such as katyoti_e2e: go list
// answers which, never a reading of the tags by hand. A folder go list
// cannot answer for is kept, so the go tool still reports it.
func leaveOutConstrained(ctx stdctx.Context, dir string, patterns []string) []string {
	var named []string
	for _, p := range patterns {
		if !strings.Contains(p, "...") {
			named = append(named, p)
		}
	}
	if len(named) == 0 {
		return patterns
	}
	args := append([]string{"list", "-e", "-f",
		"{{len .GoFiles}} {{len .CgoFiles}} {{len .TestGoFiles}} {{len .XTestGoFiles}} {{len .IgnoredGoFiles}} {{.Dir}}"}, named...)
	res, err := runTool(ctx, dir, "go", args...)
	if err != nil || res.ExitCode != 0 {
		return patterns
	}
	var excluded []os.FileInfo
	for _, line := range strings.Split(res.Stdout, "\n") {
		var gof, cgo, tst, xtst, ign int
		if n, _ := fmt.Sscanf(line, "%d %d %d %d %d", &gof, &cgo, &tst, &xtst, &ign); n != 5 {
			continue
		}
		if gof+cgo+tst+xtst > 0 || ign == 0 {
			continue
		}
		fields := strings.SplitN(strings.TrimRight(line, "\r"), " ", 6)
		if len(fields) < 6 {
			continue
		}
		if fi, err := os.Stat(fields[5]); err == nil {
			excluded = append(excluded, fi)
		}
	}
	if len(excluded) == 0 {
		return patterns
	}
	var out []string
	for _, p := range patterns {
		if !strings.Contains(p, "...") && isOneOf(filepath.Join(dir, filepath.FromSlash(p)), excluded) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// isOneOf reports whether the folder at path is one of the folders.
func isOneOf(path string, folders []os.FileInfo) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	for _, f := range folders {
		if os.SameFile(fi, f) {
			return true
		}
	}
	return false
}

// auditCorrectness runs go vet + go build on the module's code_packages
// (or the workspace root when unset) and produces critical findings
// for any failures.
func auditCorrectness(ctx stdctx.Context, gotool api.GoToolService, sc checkScope) []api.AuditFinding {
	var out []api.AuditFinding
	pkgs := sc.patterns

	// go build refuses a package of tests only ("no non-test Go files"); go
	// vet and go test still cover it, so the build leaves it out.
	if built := buildable(pkgs); len(built) > 0 {
		build, err := gotool.GoBuild(ctx, api.GoBuildInput{Packages: built, WorkDir: sc.dir})
		if err != nil || build.ExitCode != 0 {
			out = append(out, api.AuditFinding{
				Title:       "go build failed",
				Perspective: api.AuditPerspectiveCorrectness,
				Severity:    api.FindingSeverityCritical,
				FindingID:   "correctness-build-1",
				Body:        "go build emitted errors:\n\n```\n" + truncate(build.Stderr, 4096) + "\n```",
			})
		}
	}

	// go vet refuses a named folder whose Go files a build tag all leaves
	// out ("build constraints exclude all Go files"); vet the others.
	vetted := leaveOutConstrained(ctx, sc.dir, pkgs)
	if len(vetted) == 0 {
		return out
	}
	vet, err := gotool.GoVet(ctx, api.GoVetInput{Packages: vetted, WorkDir: sc.dir})
	if err != nil || vet.ExitCode != 0 {
		title := fmt.Sprintf("go vet flagged %d issue(s)", len(vet.Issues))
		if len(vet.Issues) == 0 {
			title = "go vet exited non-zero"
		}
		out = append(out, api.AuditFinding{
			Title:       title,
			Perspective: api.AuditPerspectiveCorrectness,
			Severity:    api.FindingSeverityCritical,
			FindingID:   "correctness-vet-1",
			Body:        "go vet diagnostics:\n\n```\n" + truncate(vet.Stderr, 4096) + "\n```",
		})
	}
	return out
}

// auditQuality runs go test. Test failures → high; skipped → info.
func auditQuality(ctx stdctx.Context, gotool api.GoToolService, sc checkScope) []api.AuditFinding {
	// go test fails its setup on a named folder a build tag leaves empty,
	// as go vet does: test the others.
	pkgs := leaveOutConstrained(ctx, sc.dir, sc.patterns)
	if len(pkgs) == 0 {
		return nil
	}
	test, err := gotool.GoTest(ctx, api.GoTestInput{Packages: pkgs, JSON: true, WorkDir: sc.dir})
	if err != nil {
		return []api.AuditFinding{{
			Title:       "go test invocation errored",
			Perspective: api.AuditPerspectiveQuality,
			Severity:    api.FindingSeverityHigh,
			FindingID:   "quality-test-invoke-1",
			Body:        err.Error(),
		}}
	}
	var out []api.AuditFinding
	failures := testFailures(test.Results)
	if test.Failed > 0 || len(failures) > 0 {
		title := fmt.Sprintf("%d test(s) failing", test.Failed)
		if test.Failed == 0 {
			title = fmt.Sprintf("%d package(s) failing", len(failures))
		}
		if hasTimeout(failures) {
			title += " (timeout)"
		}
		body := fmt.Sprintf("passed=%d failed=%d skipped=%d", test.Passed, test.Failed, test.Skipped)
		if len(failures) > 0 {
			body += "\n\n" + truncate(renderTestFailures(failures), 8192)
		}
		out = append(out, api.AuditFinding{
			Title:       title,
			Perspective: api.AuditPerspectiveQuality,
			Severity:    api.FindingSeverityHigh,
			FindingID:   "quality-test-fail-1",
			Body:        body,
		})
	}
	if test.Skipped > 0 {
		out = append(out, api.AuditFinding{
			Title:       fmt.Sprintf("%d test(s) skipped", test.Skipped),
			Perspective: api.AuditPerspectiveQuality,
			Severity:    api.FindingSeverityInfo,
			FindingID:   "quality-test-skip-1",
		})
	}
	return out
}

// testFailure is one failing test of a go test run, or a package that failed
// without a failing test (a test build error, a panic, a timeout).
type testFailure struct {
	pkg, test string
	// timeout is go test's "test timed out after <d>" duration when the
	// package hit its -timeout; test is then the test that was running.
	timeout string
	lines   []string
}

// testKey names a test of a package; test is empty for the package itself.
type testKey struct{ pkg, test string }

// failureLines is how many lines of a failing test's output the finding keeps.
const failureLines = 8

// testFailures reads go test's -json events: each test with a fail event, and
// each package that failed with none of its tests failing. A package that hit
// go test's -timeout is one failure naming the test that was running.
func testFailures(results []api.GoTestResult) []testFailure {
	output := map[testKey][]string{}
	var failed []testKey
	pkgFailed := map[string]bool{}
	var pkgOrder []string
	testFailedIn := map[string]bool{}
	timeouts := map[string]testKey{}
	timeoutAfter := map[string]string{}
	for _, r := range results {
		k := testKey{r.Package, r.Test}
		switch r.Action {
		case "output":
			line := strings.TrimRight(r.Output, "\r\n")
			if d, ok := strings.CutPrefix(strings.TrimSpace(line), "panic: test timed out after "); ok {
				if _, seen := timeouts[r.Package]; !seen {
					timeouts[r.Package] = k
					timeoutAfter[r.Package] = d
				}
			}
			if keepOutputLine(line) && len(output[k]) < failureLines {
				output[k] = append(output[k], line)
			}
		case "fail":
			if r.Test != "" {
				failed = append(failed, k)
				testFailedIn[r.Package] = true
			} else if !pkgFailed[r.Package] {
				pkgFailed[r.Package] = true
				pkgOrder = append(pkgOrder, r.Package)
			}
		}
	}
	var out []testFailure
	for _, k := range failed {
		if t, ok := timeouts[k.pkg]; ok && t == k {
			continue // reported once, as the package's timeout
		}
		if hasFailingSubtest(failed, k.pkg, k.test) {
			continue // the subtest names the failure; its parent only follows it
		}
		out = append(out, testFailure{pkg: k.pkg, test: k.test, lines: output[k]})
	}
	for _, pkg := range pkgOrder {
		if t, ok := timeouts[pkg]; ok {
			out = append(out, testFailure{pkg: pkg, test: t.test, timeout: timeoutAfter[pkg]})
			continue
		}
		if testFailedIn[pkg] {
			continue
		}
		out = append(out, testFailure{pkg: pkg, lines: output[testKey{pkg, ""}]})
	}
	return out
}

// keepOutputLine leaves out go test's framing lines (=== RUN, --- FAIL, the
// package's FAIL/ok summary) and blank lines: the finding keeps what the test said.
func keepOutputLine(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" || t == "FAIL" || t == "PASS" {
		return false
	}
	for _, p := range []string{"=== ", "--- FAIL", "--- PASS", "--- SKIP", "FAIL\t", "ok \t", "ok  \t"} {
		if strings.HasPrefix(t, p) {
			return false
		}
	}
	return true
}

// hasFailingSubtest reports whether a subtest of test in pkg failed.
func hasFailingSubtest(failed []testKey, pkg, test string) bool {
	for _, k := range failed {
		if k.pkg == pkg && strings.HasPrefix(k.test, test+"/") {
			return true
		}
	}
	return false
}

func hasTimeout(failures []testFailure) bool {
	for _, f := range failures {
		if f.timeout != "" {
			return true
		}
	}
	return false
}

// renderTestFailures writes one entry per failure: the package and the test,
// "timed out after <d>" for a timeout, and the first lines of the test's output.
func renderTestFailures(failures []testFailure) string {
	var b strings.Builder
	b.WriteString("Failing:\n")
	for _, f := range failures {
		switch {
		case f.timeout != "" && f.test != "":
			fmt.Fprintf(&b, "\n- %s: timed out after %s (running %s)\n", f.pkg, f.timeout, f.test)
		case f.timeout != "":
			fmt.Fprintf(&b, "\n- %s: timed out after %s\n", f.pkg, f.timeout)
		case f.test != "":
			fmt.Fprintf(&b, "\n- %s %s\n", f.pkg, f.test)
		default:
			fmt.Fprintf(&b, "\n- %s: package failed without a failing test\n", f.pkg)
		}
		if len(f.lines) > 0 {
			b.WriteString("\n```\n" + strings.Join(f.lines, "\n") + "\n```\n")
		}
	}
	return b.String()
}

// auditPackageLayout shells out to GoDepCheck over each of the module's code
// packages and surfaces public → internal/local boundary violations.
func auditPackageLayout(ctx stdctx.Context, gotool api.GoToolService, sc checkScope) []api.AuditFinding {
	var violations []api.DepViolation
	for _, p := range sc.roots {
		dep, err := gotool.GoDepCheck(ctx, api.GoDepCheckInput{ModulePath: p, WorkDir: sc.dir})
		if err != nil {
			continue
		}
		violations = append(violations, dep.Violations...)
	}
	var out []api.AuditFinding
	for i, v := range violations {
		out = append(out, api.AuditFinding{
			Title:       fmt.Sprintf("%s → %s: %s", v.Importer, v.Imported, v.Reason),
			Perspective: api.AuditPerspectivePackageLayout,
			Severity:    api.FindingSeverityHigh,
			FindingID:   fmt.Sprintf("package-layout-%d", i+1),
		})
	}
	return out
}

// auditDeadCode flags packages with no exports and no callers. The
// v1 implementation just reports zero-export packages — a real
// "dead code" detector needs the full import graph + cross-module
// awareness, which is a Phase F deliverable.
func auditDeadCode(ctx stdctx.Context, gotool api.GoToolService, sc checkScope) []api.AuditFinding {
	inv, err := gotool.GoPkgInventory(ctx, api.GoPkgInventoryInput{Packages: sc.patterns, WorkDir: sc.dir})
	if err != nil {
		return nil
	}
	var out []api.AuditFinding
	for i, pkg := range inv.Packages {
		if pkg.IsCmd {
			continue // main packages legitimately have no exports
		}
		if pkg.Bucket == "public" && pkg.PublicCount == 0 {
			out = append(out, api.AuditFinding{
				Title:       fmt.Sprintf("public package %s has zero exports", pkg.ImportPath),
				Perspective: api.AuditPerspectiveDeadCode,
				Severity:    api.FindingSeverityLow,
				FindingID:   fmt.Sprintf("dead-code-%d", i+1),
				Body:        "Public bucket exists to export API surface. A zero-export public package is likely an unused stub.",
			})
		}
	}
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…[truncated]"
}
