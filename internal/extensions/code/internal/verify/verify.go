// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucobj"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/verify/l2runner"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Manifest is the trimmed view of /modules/<name>/manifest.json that verify
// needs. The full schema lives in /modules/docs/manifest-spec.md; we decode
// only the fields used here.
type Manifest struct {
	Module         string   `json:"module"`
	SourcePaths    []string `json:"source_paths"`
	CharterVersion string   `json:"charter_version"`
}

// LoadManifest reads /modules/<module>/manifest.json relative to the project
// root.
func LoadManifest(modulesRoot, module string) (*Manifest, error) {
	path := filepath.Join(modulesRoot, module, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if m.Module == "" {
		m.Module = module
	}
	return &m, nil
}

// Options configures a single verify run.
type Options struct {
	// Module is the module name. Maps to /modules/<module>/.
	Module string

	// ProjectRoot is the repository root used to resolve manifest + source roots.
	ProjectRoot string

	// Layer selects which test layers to run. "L1" is the only one
	// implemented by Phase B. "all" includes L1 today.
	Layer string

	// UCFilter is a list of UC patterns. Empty = all UCs.
	// Each pattern is a literal UC ID ("UC-053") or a glob ("UC-04*").
	UCFilter []string

	// SeverityFilter is a list of severity strings. Empty = all severities.
	SeverityFilter []string

	// Timeout caps the batched `go test` invocation.
	Timeout time.Duration

	// OnProgress, if non-nil, is invoked AFTER each UC's verdict is resolved.
	// Idx is 1-based index of the UC being processed; Total is the total UCs
	// in scope (after filtering). The callback is synchronous — implementations
	// should be quick (e.g. print one line). Useful for live CLI feedback.
	OnProgress func(idx, total int, ucID string, verdict Verdict, dur time.Duration)

	// KeepTemp, when true, instructs the L2 runner to leave per-scenario tmp
	// homes on disk after the run for debugging. Default false (cleanup on
	// completion).
	KeepTemp bool

	// NoWrite skips recording the run as a `test-run` object (read-only mode
	// for CI / inspection). Charter-driven runs never write.
	NoWrite bool

	// CWM records the run: the `test-run` object is created through its
	// Object API, so it is validated, held to its type's size budget, indexed
	// and published, and it lands in CWM's workspace — never at a path derived
	// from ProjectRoot. Required unless NoWrite: a run that would record
	// without it is refused before any test runs.
	CWM cwmapi.CWMService

	// RunPath, after a run, is the test-run object it recorded ("" when none).
	RunPath *string
}

// Run is the top-level orchestration: load manifest + charter, parse Test
// plan + UC tables, run L1 tests, resolve verdicts, return a Report.
func Run(ctx context.Context, opts Options) (*Report, error) {
	if opts.Module == "" {
		return nil, fmt.Errorf("Options.Module is required")
	}
	if opts.ProjectRoot == "" {
		opts.ProjectRoot = "."
	}
	if opts.Layer == "" {
		opts.Layer = "L1"
	}
	if !opts.NoWrite && opts.CWM == nil {
		return nil, fmt.Errorf("verify: no CWM service to record the run — pass one, or run with NoWrite (--no-write)")
	}

	modulesRoot := filepath.Join(opts.ProjectRoot, "modules")
	manifest, err := LoadManifest(modulesRoot, opts.Module)
	if err != nil {
		return nil, err
	}

	charterPath := filepath.Join(modulesRoot, opts.Module, "charter.md")
	charter, err := LoadCharter(charterPath)
	if err != nil {
		return nil, err
	}

	// The use cases: the module's use-case and acceptance-criterion objects,
	// with each criterion's last status taken from the latest test-run (a
	// fail-deferred stays deferred; a layer not run keeps its last result).
	// Pre-flight: they must lint clean at error severity before any test runs.
	moduleDir := filepath.Join(modulesRoot, opts.Module)
	verdicts, err := ucobj.LoadVerdicts(moduleDir, opts.Module)
	if err != nil {
		return nil, fmt.Errorf("load the latest test-run: %w", err)
	}
	ucFiles, err := ucobj.LoadDir(moduleDir, opts.Module, verdicts)
	if err != nil {
		return nil, fmt.Errorf("load the use-case objects: %w", err)
	}
	if len(ucFiles) > 0 {
		findings := ucfile.Lint(ucFiles, ucfile.Options{ModuleDir: moduleDir})
		if errs := ucfile.Errors(findings); len(errs) > 0 {
			msgs := make([]string, 0, len(errs))
			for _, f := range errs {
				msgs = append(msgs, f.String())
			}
			return nil, fmt.Errorf("use-cases/ pre-flight failed (%d error findings):\n%s",
				len(errs), strings.Join(msgs, "\n"))
		}
	}

	report := &Report{
		Module:         manifest.Module,
		CharterPath:    charter.Path,
		CharterVersion: manifest.CharterVersion,
		RanAt:          time.Now(),
		Layer:          opts.Layer,
		Source:         "charter",
	}

	// Per-AC model: when the module has use cases, they ARE the run input —
	// the charter tables below serve only modules without any, and the
	// display-group lookup.
	if len(ucFiles) > 0 {
		return runFromUseCases(ctx, opts, manifest, charter, moduleDir, ucFiles, report)
	}

	// Build the per-UC plan: the union of UCs in the use-case tables and
	// rows in the Test plan. UC tables drive the iteration (one result row
	// per declared UC); Test plan rows feed test names.
	planByID := groupTestPlanByUC(charter.TestPlan)

	// Stable iteration: charter UC declaration order.
	ucIDs := orderedUCIDs(charter)

	runner := buildRunner(opts, manifest)

	// Single batched go-test invocation: union of every L1 test name across
	// all in-scope UCs. One process startup amortised across all results,
	// vs. one per UC under per-UC invocation. Real-charter speedup ~10×.
	inScopeIDs, allTests, perUCNames := collectInScope(ucIDs, charter.UCs, planByID, opts)
	var batched map[string]TestOutcome
	if (opts.Layer == "L1" || opts.Layer == "all") && len(allTests) > 0 {
		outcomes := runner.Run(ctx, allTests)
		batched = make(map[string]TestOutcome, len(outcomes))
		for _, o := range outcomes {
			batched[o.Name] = o
		}
	}

	totalInScope := len(inScopeIDs)
	progressIdx := 0
	for _, ucID := range ucIDs {
		info := charter.UCs[ucID]
		if info == nil {
			info = &UCInfo{ID: ucID}
		}
		if !filterMatch(ucID, info.Severity, opts) {
			report.Results = append(report.Results, &UCResult{
				UCID: ucID, Title: info.Title, Severity: info.Severity, Group: info.Group,
				Verdict: VerdictSkipped,
			})
			continue
		}

		row := planByID[ucID]
		res := &UCResult{
			UCID:     ucID,
			Title:    info.Title,
			Severity: info.Severity,
			Group:    info.Group,
		}

		if (opts.Layer == "L1" || opts.Layer == "all") && len(perUCNames[ucID]) > 0 {
			outcomes := make([]TestOutcome, 0, len(perUCNames[ucID]))
			for _, name := range perUCNames[ucID] {
				if o, ok := batched[name]; ok {
					outcomes = append(outcomes, o)
				} else {
					// Shouldn't happen — name was in allTests so the runner
					// must have processed it. Defensive fallback.
					outcomes = append(outcomes, TestOutcome{Name: name, Status: StatusNoMatch})
				}
			}
			res.L1 = Combine(ucID, outcomes)
		}

		if (opts.Layer == "L2" || opts.Layer == "all") && row != nil && len(row.L2Scenarios) > 0 {
			res.L2 = runL2Scenarios(ctx, opts.ProjectRoot, opts.Module, ucID, row.L2Scenarios, opts.KeepTemp)
		}

		res.Verdict = ResolveVerdict(res, row)
		report.Results = append(report.Results, res)

		progressIdx++
		if opts.OnProgress != nil {
			opts.OnProgress(progressIdx, totalInScope, ucID, res.Verdict, res.L1.TotalTime)
		}
	}

	report.ComputeTotals()
	return report, nil
}

// ── Helpers ──────────────────────────────────────────────────────────

// runL2Scenarios runs every `.tasktest` path declared for one UC and reduces
// them to a single CombinedOutcome. Each scenario's verdict maps to a
// TestOutcome: pass→Pass, fail→Fail, error→Error. The CombinedOutcome's
// FailReason cites the first scenario's failure summary so the text tree's
// row gets a useful one-liner.
func runL2Scenarios(ctx context.Context, projectRoot, module, ucID string, paths []string, keepTemp bool) CombinedOutcome {
	r := &l2runner.Runner{ProjectRoot: projectRoot, KeepTemp: keepTemp}
	outcomes := make([]TestOutcome, 0, len(paths))
	for _, path := range paths {
		full := path
		if !filepath.IsAbs(full) {
			full = filepath.Join(projectRoot, "modules", module, path)
		}
		o := TestOutcome{Name: path}
		result, err := r.Run(ctx, full)
		if err != nil {
			o.Status = StatusError
			o.Output = err.Error()
			outcomes = append(outcomes, o)
			continue
		}
		o.Duration = result.Duration
		switch result.Verdict {
		case l2runner.VerdictPass:
			o.Status = StatusPass
		case l2runner.VerdictFail:
			o.Status = StatusFail
			o.Output = strings.Join(result.Failures, "; ")
		case l2runner.VerdictError:
			o.Status = StatusError
			if result.RunErr != nil {
				o.Output = result.RunErr.Error()
			}
		}
		outcomes = append(outcomes, o)
	}
	return Combine(ucID, outcomes)
}

// collectInScope walks every UC, applies filters, and returns the in-scope
// IDs plus the deduplicated union of L1 test names across all of them, plus
// the per-UC name list (preserving order so each UC's CombinedOutcome reflects
// the same test ordering the charter declares).
func collectInScope(
	ucIDs []string,
	ucs map[string]*UCInfo,
	planByID map[string]*UCRow,
	opts Options,
) (inScope []string, allTests []string, perUCNames map[string][]string) {
	perUCNames = map[string][]string{}
	seenTest := map[string]bool{}
	for _, ucID := range ucIDs {
		info := ucs[ucID]
		if info == nil {
			info = &UCInfo{ID: ucID}
		}
		if !filterMatch(ucID, info.Severity, opts) {
			continue
		}
		inScope = append(inScope, ucID)
		row := planByID[ucID]
		if row == nil {
			continue
		}
		perUCNames[ucID] = row.L1Tests
		for _, name := range row.L1Tests {
			if !seenTest[name] {
				seenTest[name] = true
				allTests = append(allTests, name)
			}
		}
	}
	return
}

// groupTestPlanByUC expands rows that bundle multiple UC IDs ("UC-040 / UC-041 /
// UC-042 / UC-045") into one entry per UC ID.
func groupTestPlanByUC(rows []*UCRow) map[string]*UCRow {
	out := map[string]*UCRow{}
	for _, row := range rows {
		for _, id := range row.IDs {
			if existing, seen := out[id]; seen {
				// Multiple Test plan rows for the same UC — accumulate L1 tests.
				existing.L1Tests = append(existing.L1Tests, row.L1Tests...)
				existing.L2Scenarios = append(existing.L2Scenarios, row.L2Scenarios...)
			} else {
				clone := *row
				out[id] = &clone
			}
		}
	}
	return out
}

// orderedUCIDs lists every UC declared in the charter's UC tables, in
// document order. UCs referenced only in the Test plan but not declared in
// any UC table are appended at the end (defensive — shouldn't happen in
// well-formed charters).
func orderedUCIDs(c *Charter) []string {
	seen := map[string]bool{}
	var ids []string

	// UC tables — `## Use Cases` section parser preserves order.
	for _, info := range parseUCsInOrder(c) {
		if seen[info.ID] {
			continue
		}
		seen[info.ID] = true
		ids = append(ids, info.ID)
	}
	// Backstop: any Test plan row UC missing from declarations.
	for _, row := range c.TestPlan {
		for _, id := range row.IDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func parseUCsInOrder(c *Charter) []*UCInfo {
	// Re-read source to preserve order — c.UCs is a map.
	body, err := os.ReadFile(c.Path)
	if err != nil {
		return nil
	}
	return ParseUseCases(string(body))
}

// filterMatch returns true if the UC passes the active --uc / --severity
// filters.
func filterMatch(ucID, severity string, opts Options) bool {
	if len(opts.UCFilter) > 0 {
		ok := false
		for _, pat := range opts.UCFilter {
			if matchUCPattern(pat, ucID) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(opts.SeverityFilter) > 0 {
		ok := false
		for _, s := range opts.SeverityFilter {
			if strings.EqualFold(s, severity) ||
				strings.EqualFold(s, "all") ||
				(strings.EqualFold(s, "core+security") &&
					(strings.EqualFold(severity, "core") || strings.EqualFold(severity, "security"))) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// matchUCPattern matches a UC ID against a literal or glob pattern. Supports
// `*` (zero or more digits) and exact matches. e.g. `UC-04*` matches UC-040…UC-049.
func matchUCPattern(pat, id string) bool {
	if pat == id {
		return true
	}
	if !strings.Contains(pat, "*") {
		return false
	}
	// Translate glob to regex: escape, then `\*` → `\d*`.
	esc := regexp.QuoteMeta(pat)
	re := strings.ReplaceAll(esc, `\*`, `\d*`)
	matched, _ := regexp.MatchString("^"+re+"$", id)
	return matched
}

// buildRunner constructs an L1Runner with source paths resolved relative to
// the project root.
func buildRunner(opts Options, manifest *Manifest) *L1Runner {
	roots := make([]string, 0, len(manifest.SourcePaths))
	for _, p := range manifest.SourcePaths {
		// Expand to ./<path>/... so go test walks all sub-packages.
		roots = append(roots, "./"+strings.TrimSuffix(p, "/")+"/...")
	}
	return &L1Runner{
		SourceRoots: roots,
		WorkingDir:  opts.ProjectRoot,
		Timeout:     opts.Timeout,
	}
}
