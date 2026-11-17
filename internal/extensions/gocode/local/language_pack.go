// SPDX-License-Identifier: Apache-2.0

package local

import (
	stdctx "context"
	"fmt"
	"path"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	codeapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/discovery"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/lint"
)

// goPack is ktw-gocode's implementation of the LanguagePack contract
// (Track B B7 phase 1 — the THIN wrapper over existing internals;
// CodeService dispatch rewiring lands with the #4 verbs move).
//
// Registered at bootstrap as the named LanguagePack instance "go"
// (per D-1: in-process DI when bundled — today's reality — and the
// same contract serves a future KTSP slot publication).
type goPack struct{}

var _ codeapi.LanguagePack = (*goPack)(nil)

// goGates are the Go-specific FSM gate declarations (gsm1's set);
// GateCheck evaluates them via the existing GoToolService.
var goGates = []codeapi.GateSpec{
	{Name: "go-build-clean", Label: "go build clean", Transition: "en-validation→livrable",
		Description: "go build ./<code facets>/... exits 0"},
	{Name: "go-test-pass", Label: "go test pass", Transition: "en-validation→livrable",
		Description: "go test over the code facets exits 0"},
	{Name: "go-vet-clean", Label: "go vet clean", Transition: "en-validation→livrable",
		Description: "go vet reports no findings"},
	{Name: "golangci-lint-clean", Label: "golangci-lint clean", Transition: "en-validation→livrable",
		Description: "golangci-lint run reports no error-severity findings"},
}

func (p *goPack) Describe(_ stdctx.Context, _ codeapi.DescribeInput) (codeapi.LanguagePackInfo, error) {
	info := codeapi.LanguagePackInfo{
		Language:        "go",
		FileExtensions:  []string{".go"},
		ToolServiceSlot: "GoToolService",
		Gates:           goGates,
		LayoutRules: []codeapi.LayoutRuleSpec{{
			Name:        "go-three-packages",
			AppliesKind: "*",
			Description: "Every module exposes three packages: public/ (exported API), internal/ (implementation), local/ (framework init contributions).",
		}},
	}
	for _, d := range lint.AllDetectors() {
		info.CheckKinds = append(info.CheckKinds, codeapi.CheckKindSpec{
			Name:     d.Name,
			Severity: d.Severity,
		})
	}
	return info, nil
}

func (p *goPack) Discover(ctx stdctx.Context, in codeapi.DiscoverInput) (codeapi.DiscoverOutput, error) {
	inv, err := discovery.Discover(ctx, in.Path, discovery.DefaultOptions())
	if err != nil {
		return codeapi.DiscoverOutput{}, fmt.Errorf("goPack.Discover: %w", err)
	}
	out := codeapi.DiscoverOutput{Inventory: contractInventory(inv)}

	// Facet proposals from layout signals — never the retired flat
	// path fields.
	for _, pkg := range inv.Packages {
		if pkg.IsTest {
			continue
		}
		out.FacetProposals = append(out.FacetProposals, codeapi.FacetProposal{
			Facet:      codeapi.Facet{Kind: codeapi.FacetCode, Path: pkg.Path},
			Confidence: "high",
		})
	}
	if lh := inv.LayoutHints; lh != nil && lh.AppFolder != "" {
		out.FacetProposals = append(out.FacetProposals, codeapi.FacetProposal{
			Facet:      codeapi.Facet{Kind: codeapi.FacetApp, Path: lh.AppFolder},
			Confidence: "medium",
		})
	}
	out.UCProposals = ucProposalsFromTests(inv)
	for _, tf := range inv.TestFunctions {
		out.TestCaseProposals = append(out.TestCaseProposals, codeapi.TestCaseProposal{
			Ref: codeapi.TestRef{Layer: "L1", Ref: tf.Name, Package: tf.Package},
		})
	}
	return out, nil
}

func (p *goPack) ProposeUCsFromTests(ctx stdctx.Context, in codeapi.ProposeUCsInput) (codeapi.ProposeUCsOutput, error) {
	inv, err := discovery.Discover(ctx, in.Path, discovery.DefaultOptions())
	if err != nil {
		return codeapi.ProposeUCsOutput{}, fmt.Errorf("goPack.ProposeUCsFromTests: %w", err)
	}
	return codeapi.ProposeUCsOutput{UCProposals: ucProposalsFromTests(inv)}, nil
}

func (p *goPack) ReverseEngineerFeatures(ctx stdctx.Context, in codeapi.ReverseEngineerInput) (codeapi.ReverseEngineerOutput, error) {
	inv, err := discovery.Discover(ctx, in.Path, discovery.DefaultOptions())
	if err != nil {
		return codeapi.ReverseEngineerOutput{}, fmt.Errorf("goPack.ReverseEngineerFeatures: %w", err)
	}
	// Heuristic carried over from the module-discover verb: a producer
	// package imported by ≥2 local consumers suggests a feature seam.
	// Always low-confidence; never auto-commit.
	consumers := map[string]map[string]bool{}
	for _, imp := range inv.Imports {
		if !imp.IsLocal {
			continue
		}
		set := consumers[imp.Imported]
		if set == nil {
			set = map[string]bool{}
			consumers[imp.Imported] = set
		}
		set[imp.Importer] = true
	}
	var out codeapi.ReverseEngineerOutput
	for produced, set := range consumers {
		if len(set) < 2 {
			continue
		}
		var mods []string
		for c := range set {
			mods = append(mods, c)
		}
		out.FeatureProposals = append(out.FeatureProposals, codeapi.FeatureProposal{
			Title:      fmt.Sprintf("Capability around %s", produced),
			Rationale:  fmt.Sprintf("%d local consumers import it", len(set)),
			Modules:    mods,
			Confidence: "low",
		})
	}
	return out, nil
}

func (p *goPack) RunUnitTest(ctx stdctx.Context, in codeapi.RunUnitTestInput) (codeapi.RunUnitTestOutput, error) {
	if in.Ref.Layer != "L1" {
		// L2 belongs to the tasktest runner; L3 never runs via the
		// automated path (user-request-only doctrine).
		return codeapi.RunUnitTestOutput{Result: codeapi.TestResult{
			Status: codeapi.ACSkipped,
			Reason: fmt.Sprintf("goPack runs L1 probes only (got %s)", in.Ref.Layer),
		}}, nil
	}
	gotool, err := application.UseService[gcapi.GoToolService](ctx)
	if err != nil {
		return codeapi.RunUnitTestOutput{}, fmt.Errorf("goPack.RunUnitTest: resolve GoToolService: %w", err)
	}
	// Tolerate discovery's `<file>::<func>` TestRef convention on top
	// of the contract's plain L1 Go test name.
	funcName, pkg := in.Ref.Ref, in.Ref.Package
	if file, fn, ok := strings.Cut(funcName, "::"); ok {
		funcName = fn
		if pkg == "" {
			if dir := pathDir(file); dir != "." {
				pkg = dir
			}
		}
	}
	pkgs := []string{"./..."}
	if pkg != "" {
		pkgs = []string{"./" + strings.TrimPrefix(pkg, "./")}
	}
	res, err := gotool.GoTest(ctx, gcapi.GoTestInput{
		Packages: pkgs,
		RunRegex: "^" + funcName + "$",
		JSON:     true,
	})
	if err != nil {
		return codeapi.RunUnitTestOutput{Result: codeapi.TestResult{
			Status: codeapi.ACFail, Reason: err.Error(),
		}}, nil
	}
	r := codeapi.TestResult{Status: codeapi.ACPass}
	switch {
	case res.Failed > 0 || res.ExitCode != 0:
		r.Status = codeapi.ACFail
		r.Reason = fmt.Sprintf("go test failed (%d failed, exit %d)", res.Failed, res.ExitCode)
		r.Output = truncateTo(res.Stderr, 1024)
	case res.Passed == 0 && res.Skipped == 0:
		r.Status = codeapi.ACFail
		r.Reason = fmt.Sprintf("probe %s matched no test (dead ref)", in.Ref.Ref)
	case res.Passed == 0 && res.Skipped > 0:
		r.Status = codeapi.ACSkipped
		r.Reason = "test skipped itself"
	}
	return codeapi.RunUnitTestOutput{Result: r}, nil
}

func (p *goPack) GateCheck(ctx stdctx.Context, in codeapi.GateCheckInput) (codeapi.GateCheckOutput, error) {
	gotool, err := application.UseService[gcapi.GoToolService](ctx)
	if err != nil {
		return codeapi.GateCheckOutput{}, fmt.Errorf("goPack.GateCheck: resolve GoToolService: %w", err)
	}
	pkgs := codeapi.FacetPathsOf(in.Facets, codeapi.FacetCode)
	patterns := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		patterns = append(patterns, "./"+strings.TrimSuffix(strings.TrimPrefix(p, "./"), "/")+"/...")
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	fail := func(reason string) (codeapi.GateCheckOutput, error) {
		return codeapi.GateCheckOutput{Allow: false, Reason: truncateTo(reason, 2048)}, nil
	}
	switch in.Gate {
	case "go-build-clean":
		res, err := gotool.GoBuild(ctx, gcapi.GoBuildInput{Packages: patterns})
		if err != nil || res.ExitCode != 0 {
			return fail("go build failed: " + firstNonEmptyStr(errStr(err), res.Stderr))
		}
	case "go-test-pass":
		res, err := gotool.GoTest(ctx, gcapi.GoTestInput{Packages: patterns, JSON: true})
		if err != nil || res.ExitCode != 0 || res.Failed > 0 {
			return fail(fmt.Sprintf("go test failed (%d failed): %s", res.Failed, firstNonEmptyStr(errStr(err), res.Stderr)))
		}
	case "go-vet-clean":
		res, err := gotool.GoVet(ctx, gcapi.GoVetInput{Packages: patterns})
		if err != nil || res.ExitCode != 0 || len(res.Issues) > 0 {
			return fail(fmt.Sprintf("go vet flagged %d issue(s): %s", len(res.Issues), firstNonEmptyStr(errStr(err), res.Stderr)))
		}
	case "golangci-lint-clean":
		res, err := gotool.GolangciLint(ctx, gcapi.GolangciLintInput{Packages: patterns})
		if err != nil || res.ExitCode != 0 {
			return fail("golangci-lint failed: " + firstNonEmptyStr(errStr(err), res.Stderr))
		}
	default:
		return codeapi.GateCheckOutput{}, fmt.Errorf("goPack.GateCheck: unknown gate %q", in.Gate)
	}
	return codeapi.GateCheckOutput{Allow: true}, nil
}

// ── helpers ──────────────────────────────────────────────────────────

// contractInventory maps discovery's rich inventory onto the compact
// contract shape.
func contractInventory(inv *discovery.Inventory) codeapi.Inventory {
	out := codeapi.Inventory{}
	for _, p := range inv.Packages {
		out.Packages = append(out.Packages, p.Path)
	}
	for _, s := range inv.PublicSymbols {
		if s.Package == "" || s.Package == "." {
			out.PublicSymbols = append(out.PublicSymbols, s.Name)
		} else {
			out.PublicSymbols = append(out.PublicSymbols, s.Package+"."+s.Name)
		}
	}
	for _, t := range inv.TestFunctions {
		out.TestFunctions = append(out.TestFunctions, t.Name)
	}
	if inv.LayoutHints != nil {
		out.SuggestedKind = inv.LayoutHints.SuggestedKind
	}
	return out
}

// ucProposalsFromTests clusters test functions by package and proposes
// one UC seed per cluster (the existing uc.propose-from-tests
// heuristic, contract-shaped).
func ucProposalsFromTests(inv *discovery.Inventory) []codeapi.UCProposal {
	byPkg := map[string][]string{}
	for _, tf := range inv.TestFunctions {
		if tf.Kind != discovery.TestKindTest {
			continue
		}
		byPkg[tf.Package] = append(byPkg[tf.Package], tf.Name)
	}
	var out []codeapi.UCProposal
	for pkg, names := range byPkg {
		out = append(out, codeapi.UCProposal{
			Title:    fmt.Sprintf("Behavior cluster: %s (%d tests)", pkg, len(names)),
			Severity: "core",
			HintedBy: names,
		})
	}
	return out
}

// pathDir is path.Dir over a slash-normalized repo-relative ref —
// discovery TestRefs always carry forward slashes, but tolerate "\".
func pathDir(file string) string {
	return path.Dir(strings.ReplaceAll(file, "\\", "/"))
}

func truncateTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "… [truncated]"
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
