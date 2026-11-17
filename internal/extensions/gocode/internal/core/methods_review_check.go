// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// ModuleReviewCheck is the check of the go-checks review type: it runs the
// automated perspectives and the lint detectors on the module (in.Object) and
// answers a review result — the reviews service files it (the review, its
// findings followed by fingerprint). It writes nothing itself.
func (s *GocodeServiceImpl) ModuleReviewCheck(ctx stdctx.Context, in api.ModuleReviewCheckInput) (api.ModuleReviewCheckOutput, error) {
	if in.Object == "" {
		return api.ModuleReviewCheckOutput{}, fmt.Errorf("%w: ModuleReviewCheck: object is required", api.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModuleReviewCheckOutput{}, fmt.Errorf("ModuleReviewCheck: resolve CWMService: %w", err)
	}
	inputs, err := projection.LoadManifestInputs(ctx, cwm, in.Object)
	if err != nil {
		return api.ModuleReviewCheckOutput{}, fmt.Errorf("ModuleReviewCheck: load inputs: %w", err)
	}
	gotool, err := application.UseService[api.GoToolService](ctx)
	if err != nil {
		return api.ModuleReviewCheckOutput{}, fmt.Errorf("ModuleReviewCheck: resolve GoToolService: %w", err)
	}
	root := projectRootOf(ctx, cwm, in.Object)
	sc := moduleScope(inputs.Module)
	if len(in.Paths) > 0 {
		// A request's change: only the packages its paths touch.
		if sc = changeScope(inputs.Module, root, in.Paths); len(sc.patterns) == 0 {
			return api.ModuleReviewCheckOutput{ReviewResult: noGoChange()}, nil
		}
	}
	sc.dir = root
	res := checkResult(collectAuditFindings(ctx, gotool, inputs, sc))
	if len(in.Paths) > 0 {
		res.Coverage = reviewsapi.CoveragePartial // the change's packages, not the module's whole code
	}
	return api.ModuleReviewCheckOutput{ReviewResult: res}, nil
}

// noGoChange is the result for a change that touched no Go file of the
// module: nothing for the toolchain to check, so nothing is filed (coverage
// none) - the previous review and its open findings stand.
func noGoChange() reviewsapi.ReviewResult {
	score := 100
	return reviewsapi.ReviewResult{Rating: reviewsapi.RatingGood, Score: &score, Coverage: reviewsapi.CoverageNone,
		Summary: "The change touched no Go file of the module: nothing to build, vet or test.",
		Report:  "# Go checks\n\nThe change touched no Go file of the module: nothing to build, vet or test.\n"}
}

// checkResult is what the audit found as a review result: its rating (the
// audit's verdict rule), a score (100 less 25 per critical, 10 per high, 3
// per medium, 1 per low, never below 0), a report by perspective, and one
// finding each, keyed by its perspective and title so a later run finds it
// again.
func checkResult(findings []api.AuditFinding) reviewsapi.ReviewResult {
	var counts api.ModuleAuditOutput
	score := 100
	byPersp := map[string][]api.AuditFinding{}
	var res reviewsapi.ReviewResult
	for _, f := range findings {
		switch f.Severity {
		case api.FindingSeverityCritical:
			counts.CriticalCount++
			score -= 25
		case api.FindingSeverityHigh:
			counts.HighCount++
			score -= 10
		case api.FindingSeverityMedium:
			counts.MediumCount++
			score -= 3
		case api.FindingSeverityLow:
			counts.LowCount++
			score--
		}
		p := string(f.Perspective)
		byPersp[p] = append(byPersp[p], f)
		res.Findings = append(res.Findings, reviewsapi.ResultFinding{
			Severity: string(f.Severity), Statement: f.Title, Location: p,
			Key: p + ": " + f.Title, Evidence: f.Body,
		})
	}
	if score < 0 {
		score = 0
	}
	res.Rating = auditVerdict(counts)
	res.Score = &score
	res.Summary = fmt.Sprintf("Build, vet, tests, package layout, dead code and lint: %d critical, %d high, %d medium, %d low.",
		counts.CriticalCount, counts.HighCount, counts.MediumCount, counts.LowCount)
	var b strings.Builder
	b.WriteString("# Go checks\n\n| Perspective | Findings |\n|---|---|\n")
	persps := make([]string, 0, len(byPersp))
	for p := range byPersp {
		persps = append(persps, p)
	}
	sort.Strings(persps)
	for _, p := range persps {
		fmt.Fprintf(&b, "| %s | %d |\n", p, len(byPersp[p]))
	}
	if len(persps) == 0 {
		b.WriteString("| — | 0 |\n\nEverything builds, vets, tests and lints clean.\n")
	}
	for _, p := range persps {
		fmt.Fprintf(&b, "\n## %s\n\n", p)
		for _, f := range byPersp[p] {
			fmt.Fprintf(&b, "- **%s** — %s\n", f.Severity, f.Title)
		}
	}
	res.Report = b.String()
	return res
}
