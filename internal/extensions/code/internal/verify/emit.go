// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// Report is the top-level result of a verify run, consumed by all emitters.
type Report struct {
	Module         string    `json:"module"`
	CharterPath    string    `json:"charter_path"`
	CharterVersion string    `json:"charter_version"`
	RanAt          time.Time `json:"ran_at"`
	Layer          string    `json:"layer"`
	// Source names the run input: "charter" (Test-plan tables) or
	// "use-cases" (per-UC files with inline ACs — the per-AC model).
	Source      string      `json:"source,omitempty"`
	Results     []*UCResult `json:"results"`
	Totals      Totals      `json:"totals"`
	Tier1DoDMet bool        `json:"tier_1_dod_met"`
}

// Totals counts UCs by verdict for the summary line.
type Totals struct {
	Pass         int `json:"pass"`
	Fail         int `json:"fail"`
	FailDeferred int `json:"fail_deferred"`
	NoTest       int `json:"no_test"`
	Skipped      int `json:"skipped"`
	UCTotal      int `json:"uc_total"`
}

// ComputeTotals tallies a Report's per-UC verdicts.
func (r *Report) ComputeTotals() {
	r.Totals = Totals{UCTotal: len(r.Results)}
	for _, res := range r.Results {
		switch res.Verdict {
		case VerdictPass:
			r.Totals.Pass++
		case VerdictFail:
			r.Totals.Fail++
		case VerdictFailDeferred:
			r.Totals.FailDeferred++
		case VerdictNoTest:
			r.Totals.NoTest++
		case VerdictSkipped:
			r.Totals.Skipped++
		}
	}
	// Tier-1 "DoD met" gate: zero fails, zero no-tests. fail-deferred is OK.
	r.Tier1DoDMet = r.Totals.Fail == 0 && r.Totals.NoTest == 0
}

// ── Text emitter ──────────────────────────────────────────────────────

// TextOptions tunes the text-tree presentation.
type TextOptions struct {
	// Detail expands every UC row regardless of verdict. Default (false)
	// collapses pass + skipped under the group's count summary; only fails,
	// fail-deferreds, and no-tests are expanded.
	Detail bool
}

// EmitText writes a hierarchical tree grouped by charter subsection. Default
// collapses passing groups to a one-line summary; expands rows that need user
// attention (fail / fail-deferred / no-test).
//
// Use EmitTextDetail to expand every row. Both wrap EmitTextOpts.
func EmitText(w io.Writer, r *Report) error {
	return EmitTextOpts(w, r, TextOptions{})
}

// EmitTextDetail expands every UC row.
func EmitTextDetail(w io.Writer, r *Report) error {
	return EmitTextOpts(w, r, TextOptions{Detail: true})
}

// EmitTextOpts is the underlying tree emitter.
func EmitTextOpts(w io.Writer, r *Report, opts TextOptions) error {
	fmt.Fprintf(w, "%s — charter %s, layer %s, %s\n",
		r.Module, r.CharterVersion, r.Layer, r.RanAt.UTC().Format("2006-01-02 15:04Z"))

	groups := groupResults(r.Results)
	last := len(groups) - 1
	for i, g := range groups {
		branch := "├── "
		nested := "│   "
		if i == last {
			branch = "└── "
			nested = "    "
		}
		summary := groupSummary(g.totals, opts.Detail)
		label := g.label
		if label == "" {
			label = "(ungrouped)"
		}
		fmt.Fprintf(w, "%s%-42s %s\n", branch, label, summary)

		for _, res := range g.expanded(opts.Detail) {
			fmt.Fprintf(w, "%s%s\n", nested, formatRow(res))
			if opts.Detail {
				for _, ac := range res.ACs {
					fmt.Fprintf(w, "%s    %s\n", nested, formatACRow(ac))
				}
			}
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Summary")
	fmt.Fprintf(w, "  🟢 pass            %3d\n", r.Totals.Pass)
	fmt.Fprintf(w, "  🔴 fail            %3d\n", r.Totals.Fail)
	fmt.Fprintf(w, "  🟠 fail-deferred   %3d\n", r.Totals.FailDeferred)
	fmt.Fprintf(w, "  ⚪ no-test         %3d\n", r.Totals.NoTest)
	fmt.Fprintf(w, "  ⏸ skipped         %3d\n", r.Totals.Skipped)
	fmt.Fprintln(w)
	if r.Tier1DoDMet {
		fmt.Fprintln(w, "Tier-1 DoD met?  ✓")
	} else {
		fmt.Fprintf(w, "Tier-1 DoD met?  ✗   blocked by %d fails + %d no-tests\n",
			r.Totals.Fail, r.Totals.NoTest)
	}
	return nil
}

// groupBucket holds the per-subsection rollup the tree emitter walks.
type groupBucket struct {
	label   string
	results []*UCResult
	totals  Totals
}

// expanded returns the rows to print under this group's header. By default
// only rows the user must act on (fail / fail-deferred) are returned. Pass,
// skipped, and no-test rows stay collapsed under the group's count summary.
// `--detail` returns all rows.
func (g *groupBucket) expanded(detail bool) []*UCResult {
	if detail {
		return g.results
	}
	out := make([]*UCResult, 0)
	for _, r := range g.results {
		switch r.Verdict {
		case VerdictFail, VerdictFailDeferred:
			out = append(out, r)
		}
	}
	return out
}

// groupResults buckets the results by Group preserving first-seen order.
func groupResults(results []*UCResult) []*groupBucket {
	idx := map[string]*groupBucket{}
	order := []string{}
	for _, res := range results {
		key := res.Group
		bucket, seen := idx[key]
		if !seen {
			bucket = &groupBucket{label: key}
			idx[key] = bucket
			order = append(order, key)
		}
		bucket.results = append(bucket.results, res)
		switch res.Verdict {
		case VerdictPass:
			bucket.totals.Pass++
		case VerdictFail:
			bucket.totals.Fail++
		case VerdictFailDeferred:
			bucket.totals.FailDeferred++
		case VerdictNoTest:
			bucket.totals.NoTest++
		case VerdictSkipped:
			bucket.totals.Skipped++
		}
		bucket.totals.UCTotal++
	}
	out := make([]*groupBucket, 0, len(order))
	for _, key := range order {
		out = append(out, idx[key])
	}
	return out
}

// groupSummary formats the per-group counts shown next to the group label.
// Empty buckets get "(empty)"; otherwise the format is verdict counts joined
// by " · " — only the non-zero verdicts appear.
func groupSummary(t Totals, _ bool) string {
	parts := make([]string, 0, 5)
	if t.Pass > 0 {
		parts = append(parts, fmt.Sprintf("🟢 %d", t.Pass))
	}
	if t.Fail > 0 {
		parts = append(parts, fmt.Sprintf("🔴 %d", t.Fail))
	}
	if t.FailDeferred > 0 {
		parts = append(parts, fmt.Sprintf("🟠 %d", t.FailDeferred))
	}
	if t.NoTest > 0 {
		parts = append(parts, fmt.Sprintf("⚪ %d", t.NoTest))
	}
	if t.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("⏸ %d", t.Skipped))
	}
	if len(parts) == 0 {
		return "(empty)"
	}
	return strings.Join(parts, " · ")
}

// formatRow formats one UC's expanded row line under a group header.
func formatRow(res *UCResult) string {
	sev := res.Severity
	if sev == "" {
		sev = "—"
	}
	extra := ""
	switch res.Verdict {
	case VerdictFail:
		if res.Reason != "" {
			extra = " — " + truncate(res.Reason, 70)
		}
	case VerdictFailDeferred:
		if res.DeferredBy != "" {
			extra = " (" + res.DeferredBy + ")"
		}
	}
	return fmt.Sprintf("%s %-8s %-10s %s %s%s",
		res.Verdict.Symbol(), res.UCID, sev, layerSymbolL1(res.L1), res.Verdict, extra)
}

// layerSymbolL1 returns the same glyph as layerSymbol but specifically for
// L1, falling back to "—" when not run.
func layerSymbolL1(c CombinedOutcome) string {
	if c.UCID == "" {
		return "—"
	}
	return layerSymbol(c)
}

// layerSymbol maps a CombinedOutcome to a one-glyph symbol used in the text
// tree's L1/L2/L3 columns.
func layerSymbol(c CombinedOutcome) string {
	if c.UCID == "" {
		return "—"
	}
	switch c.Status {
	case StatusPass:
		return "✓"
	case StatusFail, StatusError, StatusNoMatch:
		return "✗"
	case StatusSkip:
		return "·"
	}
	return "?"
}

// ── JUnit XML emitter ─────────────────────────────────────────────────

// EmitJUnit writes a JUnit XML report consumed by VS Code Test Explorer,
// GitHub Actions, GitLab CI, IntelliJ, etc.
//
// One <testsuite> per severity bucket (core / security / qol). One
// <testcase> per UC. Failures populate <failure>; skips populate <skipped>;
// fail-deferred uses <skipped> with a clear message (CIs render it as
// non-blocking).
func EmitJUnit(w io.Writer, r *Report) error {
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}

	suites := struct {
		XMLName  xml.Name `xml:"testsuites"`
		Name     string   `xml:"name,attr"`
		Tests    int      `xml:"tests,attr"`
		Failures int      `xml:"failures,attr"`
		Skipped  int      `xml:"skipped,attr"`
		Time     string   `xml:"time,attr"`
		Suites   []jSuite `xml:"testsuite"`
	}{
		Name:     r.Module,
		Tests:    r.Totals.UCTotal,
		Failures: r.Totals.Fail,
		Skipped:  r.Totals.FailDeferred + r.Totals.NoTest + r.Totals.Skipped,
		Time:     fmt.Sprintf("%.3f", totalTime(r).Seconds()),
	}

	bucket := map[string][]jCase{}
	bucketOrder := []string{}
	for _, res := range r.Results {
		key := res.Severity
		if key == "" {
			key = "uncategorized"
		}
		if _, seen := bucket[key]; !seen {
			bucketOrder = append(bucketOrder, key)
		}
		if len(res.ACs) > 0 {
			bucket[key] = append(bucket[key], jCasesFromACs(r.Module, res)...)
		} else {
			bucket[key] = append(bucket[key], jCaseFromUC(r.Module, res))
		}
	}
	for _, key := range bucketOrder {
		cases := bucket[key]
		s := jSuite{Name: r.Module + "-" + key, Tests: len(cases), Cases: cases}
		for _, c := range cases {
			if c.Failure != nil {
				s.Failures++
			}
			if c.Skipped != nil {
				s.SkippedCount++
			}
		}
		suites.Suites = append(suites.Suites, s)
	}

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(suites)
}

type jSuite struct {
	XMLName      xml.Name `xml:"testsuite"`
	Name         string   `xml:"name,attr"`
	Tests        int      `xml:"tests,attr"`
	Failures     int      `xml:"failures,attr"`
	SkippedCount int      `xml:"skipped,attr"`
	Cases        []jCase  `xml:"testcase"`
}

type jCase struct {
	XMLName   xml.Name  `xml:"testcase"`
	ClassName string    `xml:"classname,attr"`
	Name      string    `xml:"name,attr"`
	Time      string    `xml:"time,attr"`
	Failure   *jFailure `xml:"failure,omitempty"`
	Skipped   *jSkipped `xml:"skipped,omitempty"`
}

type jFailure struct {
	XMLName xml.Name `xml:"failure"`
	Message string   `xml:"message,attr"`
	Type    string   `xml:"type,attr"`
	Body    string   `xml:",chardata"`
}

type jSkipped struct {
	XMLName xml.Name `xml:"skipped"`
	Message string   `xml:"message,attr"`
}

func jCaseFromUC(module string, res *UCResult) jCase {
	c := jCase{
		ClassName: module + "." + res.UCID,
		Name:      strings.TrimSpace(res.Title),
		Time:      fmt.Sprintf("%.3f", res.L1.TotalTime.Seconds()),
	}
	if c.Name == "" {
		c.Name = res.UCID
	}
	switch res.Verdict {
	case VerdictFail:
		c.Failure = &jFailure{Message: res.Reason, Type: "test-fail", Body: l1OutputDigest(res.L1)}
	case VerdictFailDeferred:
		msg := "fail-deferred"
		if res.DeferredBy != "" {
			msg = "fail-deferred by " + res.DeferredBy
		}
		c.Skipped = &jSkipped{Message: msg}
	case VerdictNoTest:
		c.Skipped = &jSkipped{Message: "no test plan row"}
	case VerdictSkipped:
		c.Skipped = &jSkipped{Message: "skipped by filter"}
	}
	return c
}

// jCasesFromACs emits one JUnit testcase per acceptance criterion — the
// per-AC model's natural granularity for Test Explorer / CI annotation.
func jCasesFromACs(module string, res *UCResult) []jCase {
	cases := make([]jCase, 0, len(res.ACs))
	for _, ac := range res.ACs {
		c := jCase{
			ClassName: module + "." + res.UCID,
			Name:      ac.ID,
			Time:      fmt.Sprintf("%.3f", ac.Duration.Seconds()),
		}
		switch ac.Status {
		case "fail":
			c.Failure = &jFailure{Message: ac.Reason, Type: "ac-fail", Body: ac.TestRef}
		case "fail-deferred":
			c.Skipped = &jSkipped{Message: "fail-deferred: " + ac.Reason}
		case "no-test":
			c.Skipped = &jSkipped{Message: "no probe declared"}
		case "skipped":
			c.Skipped = &jSkipped{Message: ac.Reason}
		}
		cases = append(cases, c)
	}
	return cases
}

func l1OutputDigest(c CombinedOutcome) string {
	if len(c.Tests) == 0 {
		return ""
	}
	for _, t := range c.Tests {
		if t.Status == StatusFail || t.Status == StatusError {
			return t.Output
		}
	}
	return c.Tests[0].Output
}

func totalTime(r *Report) time.Duration {
	var d time.Duration
	for _, res := range r.Results {
		d += res.L1.TotalTime
		d += res.L2.TotalTime
		d += res.L3.TotalTime
	}
	return d
}

// ── JSON emitter ──────────────────────────────────────────────────────

// EmitJSON writes the structured form documented in verbs/verify.md.
func EmitJSON(w io.Writer, r *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(toJSONShape(r))
}

// toJSONShape rebuilds the Report into the legacy keys per the verb spec
// (`by_uc` / `totals`) — keeps the JSON contract stable for downstream tools.
func toJSONShape(r *Report) any {
	byUC := map[string]any{}
	for _, res := range r.Results {
		row := map[string]any{
			"verdict":  res.Verdict.String(),
			"severity": res.Severity,
		}
		if res.L1.UCID != "" {
			row["L1"] = res.L1.Status.String()
		}
		if res.Reason != "" {
			row["reason"] = res.Reason
		}
		if res.DeferredBy != "" {
			row["deferred_by"] = res.DeferredBy
		}
		if len(res.ACs) > 0 {
			acs := make([]map[string]any, 0, len(res.ACs))
			for _, ac := range res.ACs {
				entry := map[string]any{
					"id":     ac.ID,
					"layer":  ac.Layer,
					"status": ac.Status,
				}
				if ac.TestRef != "" {
					entry["test_ref"] = ac.TestRef
				}
				if ac.Anti {
					entry["anti"] = true
				}
				if ac.Reason != "" {
					entry["reason"] = ac.Reason
				}
				acs = append(acs, entry)
			}
			row["acs"] = acs
		}
		byUC[res.UCID] = row
	}
	out := map[string]any{
		"module":          r.Module,
		"charter_version": r.CharterVersion,
		"layer":           r.Layer,
		"ran_at":          r.RanAt.UTC().Format(time.RFC3339),
		"by_uc":           byUC,
		"totals":          r.Totals,
		"tier_1_dod_met":  r.Tier1DoDMet,
	}
	if r.Source != "" {
		out["source"] = r.Source
	}
	return out
}

// formatACRow renders one per-AC line for the text tree's --detail mode.
func formatACRow(ac ACResult) string {
	sym := "❓"
	switch ac.Status {
	case "pass":
		sym = "🟢"
	case "fail":
		sym = "🔴"
	case "fail-deferred":
		sym = "🟠"
	case "no-test":
		sym = "⚪"
	case "skipped":
		sym = "⏸"
	}
	kind := ""
	if ac.Anti {
		kind = " [anti]"
	}
	ref := ac.TestRef
	if ref == "" {
		ref = "(no probe)"
	}
	s := fmt.Sprintf("· %s %s %s%s — %s", sym, ac.ID, ac.Layer, kind, ref)
	if ac.Reason != "" {
		s += "  (" + ac.Reason + ")"
	}
	return s
}
