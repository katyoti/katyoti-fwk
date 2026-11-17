// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/projection"
)

// BuildCharter composes the projected charter.md markdown from
// authored prose blocks + projected sections. Output is
// deterministic and idempotent: identical inputs yield byte-
// identical markdown.
//
// Section layout (mirrors modules/tasks/charter.md):
//
//  1. Title + Meta
//  2. Concepts + Glossary (authored from charter.concepts_md)
//  3. Use Cases (projected from use-case items)
//  4. Definition of Done — per UC (projected from use-case bodies)
//  5. Test plan (projection.RenderTestPlanSection — the one renderer)
//  6. Open Decisions (projected — open + resolved)
//  7. Public API Contract (authored from charter.api_contract_md)
//  8. Audit History (projected from audit reviews + findings)
//  9. Manifest Stats footer (projected snapshot)
//
// This is the ONLY writer of charter.md: every section is rendered here, and
// nothing splices into the file afterwards. A use case's status — in the UC
// table, the DoD headers and the snapshot — is the latest test-run verdict
// (UCStatuses over in.UCFiles), the same reading the Test plan renders.
//
// Returns the rendered markdown as a string.
func BuildCharter(in CharterInputs) string {
	var b bytes.Buffer
	status := UCStatuses(in.UCFiles)

	// Section 1 — Title + Meta
	writeTitleSection(&b, in)
	// Section 2 — Concepts (authored)
	writeAuthoredBlock(&b, "Concepts & Glossary", conceptsFor(in.CharterItem),
		`> _Concepts and glossary terms used throughout the charter. Authored
> prose; the architect / documenter personas keep this current as
> the module evolves._`)
	// Section 3 — Use Cases table
	writeUseCasesTable(&b, in.UseCases, status)
	// Section 4 — DoD per UC
	writeDoDPerUC(&b, in.UseCases, status)
	// Section 5 — Test plan
	b.WriteString(projection.RenderTestPlanSection(in.Module.ID, in.UCFiles))
	b.WriteString("\n")
	// Section 6 — Open Decisions
	writeOpenDecisions(&b, in.OpenDecisions)
	// Section 7 — Public API Contract (authored)
	writeAuthoredBlock(&b, "Public API Contract", apiContractFor(in.CharterItem),
		`> _The public surface this module commits to. Authored by the
> architect; updated when public symbols are added / removed /
> renamed. Stability claims (stable / provisional / deprecated)
> live here, not in inline godoc._`)
	// Section 8 — Audit History
	writeAuditHistory(&b, in.LatestAudit, in.OpenFindings)
	// Section 9 — Footer (manifest stats snapshot)
	writeStatsFooter(&b, in, status)

	return b.String()
}

// --- section writers -----------------------------------------------------

func writeTitleSection(b *bytes.Buffer, in CharterInputs) {
	displayName := in.Module.Name
	fmt.Fprintf(b, "# %s — Module Charter\n\n", displayName)

	version := "—"
	status := "draft"
	if in.CharterItem != nil {
		if in.CharterItem.Version != "" {
			version = in.CharterItem.Version
		}
		if in.CharterItem.Status != "" {
			status = in.CharterItem.Status
		}
	}

	fmt.Fprintf(b, "**Module**: `%s`\n", in.Module.Name)
	fmt.Fprintf(b, "**Kind**: `%s`\n", in.Module.Kind)
	fmt.Fprintf(b, "**Charter version**: `%s`  ·  **status**: `%s`\n", version, status)
	fmt.Fprintf(b, "**Tier**: %d / %d  ·  **state**: `%s`\n",
		in.Module.TierCurrent, in.Module.TierTarget, in.Module.TierState)
	if codePaths := in.Module.FacetPaths(gcapi.FacetCode); len(codePaths) > 0 {
		fmt.Fprintln(b, "**Source paths**:")
		sortedPkgs := append([]string(nil), codePaths...)
		sort.Strings(sortedPkgs)
		for _, p := range sortedPkgs {
			fmt.Fprintf(b, "- `%s`\n", p)
		}
	}
	b.WriteString("\n")

	b.WriteString(`> _This file is a PROJECTION. The architect / specifier / tester
> author the typed items it composes (charter / use-case /
> decision / finding); ` + "`module charter-publish`" + ` rebuilds
> this markdown from those items. **Do not hand-edit projected
> sections** — they will be overwritten on the next publish. The
> drift-detection lint detector flags hand-edits._

`)
}

func writeAuthoredBlock(b *bytes.Buffer, heading, body, fallbackPlaceholder string) {
	fmt.Fprintf(b, "## %s\n\n", heading)
	if strings.TrimSpace(body) == "" {
		b.WriteString(fallbackPlaceholder)
		b.WriteString("\n\n_(no content authored yet)_\n\n")
		return
	}
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n\n")
}

func writeUseCasesTable(b *bytes.Buffer, ucs []gcapi.UseCase, status map[string]gcapi.UCStatus) {
	b.WriteString("## Use Cases\n\n")
	if len(ucs) == 0 {
		b.WriteString("_No use-case items at this module yet. Author them with the `specifier` persona._\n\n")
		return
	}
	// Sort by UCID for deterministic output.
	sorted := append([]gcapi.UseCase(nil), ucs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UCID < sorted[j].UCID
	})

	fmt.Fprintln(b, "| UC ID | Title | Severity | Status |")
	fmt.Fprintln(b, "|---|---|---|---|")
	for _, uc := range sorted {
		title := strings.ReplaceAll(uc.Title, "|", `\|`)
		if title == "" {
			title = "_(no title)_"
		}
		statusBadge := statusBadgeFor(status[uc.UCID])
		fmt.Fprintf(b, "| `%s` | %s | `%s` | %s |\n",
			uc.UCID, title, uc.Severity, statusBadge)
	}
	b.WriteString("\n")
}

func writeDoDPerUC(b *bytes.Buffer, ucs []gcapi.UseCase, status map[string]gcapi.UCStatus) {
	b.WriteString("## Definition of Done — per UC\n\n")
	if len(ucs) == 0 {
		b.WriteString("_(no use-cases authored yet)_\n\n")
		return
	}
	sorted := append([]gcapi.UseCase(nil), ucs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UCID < sorted[j].UCID
	})
	for _, uc := range sorted {
		fmt.Fprintf(b, "### `%s` — %s\n\n", uc.UCID, uc.Title)
		fmt.Fprintf(b, "_severity: `%s`  ·  status: %s_\n\n",
			uc.Severity, statusBadgeFor(status[uc.UCID]))
		if uc.FailDeferredBy != "" {
			fmt.Fprintf(b, "_Deferred by: `%s`_\n\n", uc.FailDeferredBy)
		}
		body := strings.TrimSpace(uc.Body)
		if body == "" {
			b.WriteString("_(no DoD body — author one via `uc.Update`)_\n\n")
			continue
		}
		b.WriteString(body)
		b.WriteString("\n\n")
	}
}

func writeOpenDecisions(b *bytes.Buffer, ods []gcapi.OpenDecision) {
	b.WriteString("## Open Decisions\n\n")
	if len(ods) == 0 {
		b.WriteString("_(no decision items)_\n\n")
		return
	}
	sorted := append([]gcapi.OpenDecision(nil), ods...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ODID < sorted[j].ODID
	})
	var open, resolved []gcapi.OpenDecision
	for _, od := range sorted {
		if od.State == gcapi.ODStateResolved {
			resolved = append(resolved, od)
		} else {
			open = append(open, od)
		}
	}
	b.WriteString("### Open\n\n")
	if len(open) == 0 {
		b.WriteString("_(none)_\n\n")
	} else {
		for _, od := range open {
			fmt.Fprintf(b, "- **`%s`** — %s\n", od.ODID, od.Title)
		}
		b.WriteString("\n")
	}
	b.WriteString("### Resolved\n\n")
	if len(resolved) == 0 {
		b.WriteString("_(none)_\n\n")
	} else {
		for _, od := range resolved {
			fmt.Fprintf(b, "- **`%s`** — %s → _%s_\n",
				od.ODID, od.Title, defaultString(od.Resolution, "(no resolution recorded)"))
		}
		b.WriteString("\n")
	}
}

func writeAuditHistory(b *bytes.Buffer, latest *gcapi.AuditBundle, findings []gcapi.AuditFinding) {
	b.WriteString("## Audit History\n\n")
	if latest == nil {
		b.WriteString("_(no audit yet)_\n\n")
		return
	}
	// When it ran is the run's own record; none recorded, no date — never
	// the zero time.
	if latest.RanAt.IsZero() {
		fmt.Fprintf(b, "**Latest**: `%s`\n\n", latest.Name)
	} else {
		fmt.Fprintf(b, "**Latest**: `%s`  ·  ran %s\n\n",
			latest.Name, latest.RanAt.UTC().Format(time.RFC3339))
	}

	var crit, high, med, low int
	closed := 0
	for _, f := range findings {
		if f.State == gcapi.FindingStateClosed {
			closed++
			continue
		}
		if f.State != gcapi.FindingStateOpen {
			continue
		}
		switch f.Severity {
		case gcapi.FindingSeverityCritical:
			crit++
		case gcapi.FindingSeverityHigh:
			high++
		case gcapi.FindingSeverityMedium:
			med++
		case gcapi.FindingSeverityLow:
			low++
		}
	}
	fmt.Fprintln(b, "**Findings open by severity**:")
	fmt.Fprintf(b, "- 🔴 critical: %d\n", crit)
	fmt.Fprintf(b, "- 🟠 high: %d\n", high)
	fmt.Fprintf(b, "- 🟡 medium: %d\n", med)
	fmt.Fprintf(b, "- 🟢 low: %d\n", low)
	fmt.Fprintf(b, "- closed since this audit: %d\n\n", closed)

	if crit > 0 || high > 0 {
		b.WriteString("> **Promote gate blocked** — every `critical` and `high` finding\n")
		b.WriteString("> must be closed (UC `closes` edge) or deferred (OD `deferred-by` edge)\n")
		b.WriteString("> before `module promote` will accept this tier flip.\n\n")
	}
}

func writeStatsFooter(b *bytes.Buffer, in CharterInputs, status map[string]gcapi.UCStatus) {
	b.WriteString("## Snapshot — Manifest Stats\n\n")
	var pass, fail, deferred, notest int
	for _, uc := range in.UseCases {
		switch status[uc.UCID] {
		case gcapi.UCStatusPass:
			pass++
		case gcapi.UCStatusFail:
			fail++
		case gcapi.UCStatusFailDeferred:
			deferred++
		default:
			notest++
		}
	}
	fmt.Fprintln(b, "| Metric | Value |")
	fmt.Fprintln(b, "|---|---|")
	fmt.Fprintf(b, "| UC total | %d |\n", len(in.UseCases))
	fmt.Fprintf(b, "| Passing | %d |\n", pass)
	fmt.Fprintf(b, "| Failing | %d |\n", fail)
	fmt.Fprintf(b, "| Fail-deferred | %d |\n", deferred)
	fmt.Fprintf(b, "| No test | %d |\n", notest)
	if in.LatestTestRun != nil {
		fmt.Fprintf(b, "| Last verify | %s |\n",
			in.LatestTestRun.RanAt.UTC().Format(time.DateOnly))
		fmt.Fprintf(b, "| Tier-N DoD met | %v |\n", in.LatestTestRun.TierNDoDMet)
	}
	b.WriteString("\n")
	b.WriteString("_See `manifest.json` for the full machine-readable rollup._\n")
}

// --- formatting helpers --------------------------------------------------

func statusBadgeFor(s gcapi.UCStatus) string {
	switch verdictOf(s) {
	case gcapi.UCStatusPass:
		return "🟢 pass"
	case gcapi.UCStatusFail:
		return "🔴 fail"
	case gcapi.UCStatusFailDeferred:
		return "🟠 fail-deferred"
	case gcapi.UCStatusSkipped:
		return "⏸ skipped"
	default:
		return "⚪ no-test"
	}
}

func conceptsFor(c *Charter) string {
	if c == nil {
		return ""
	}
	return c.ConceptsMD
}

func apiContractFor(c *Charter) string {
	if c == nil {
		return ""
	}
	return c.APIContractMD
}

func defaultString(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}
