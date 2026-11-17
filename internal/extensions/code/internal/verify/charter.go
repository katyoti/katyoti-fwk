// SPDX-License-Identifier: Apache-2.0

// Package verify implements `module verify <module>` — UC-driven E2E
// verification with green/red output for L1 (and later L2/L3) layers.
//
// Phase B (this package as initially shipped) covers L1 only: parse the
// charter's Test plan and UC tables; shell out `go test -run` per UC; emit
// text/JUnit/JSON results. Phase C will extend it with a `.tasktest` runner
// for L2.
//
// Driving spec: app/ktw-code/embed/references/modules/verbs/verify.md
package verify

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// UCRow is a single row from the charter's Test plan section.
type UCRow struct {
	// IDs holds the UC identifiers this row applies to. Most rows have one
	// (`UC-053`); some compress related UCs (`UC-040 / UC-041 / UC-042`).
	IDs []string

	// L1Tests holds the Go test names extracted from the L1 cell.
	// Empty list means "no L1 test plan" (verdict will be no-test).
	L1Tests []string

	// L2Scenarios holds .tasktest file paths from the L2 cell.
	// Phase B records but does not run them.
	L2Scenarios []string

	// L3Scenarios holds task names from the L3 cell.
	L3Scenarios []string

	// Notes is any trailing parenthetical or "**partial**" annotation.
	Notes string
}

// UCInfo carries cross-section metadata about a UC: its title, severity, and
// charter-subsection grouping, extracted from the `## Use Cases` section's
// tables and surrounding H4/H5 headers. Populated by ParseUseCases; merged
// with TestPlanRow lookups in the orchestrator.
type UCInfo struct {
	ID       string
	Title    string
	Severity string // core | security | qol | cosmetic | (empty if unknown)
	DoDOK    bool   // true if charter's DoD column was ✓
	Group    string // subsection header (e.g. "Permissions (UC-050 to UC-058)")
}

// Charter aggregates everything the verify command needs from a parsed
// charter.md.
type Charter struct {
	Path     string
	UCs      map[string]*UCInfo // by ID
	TestPlan []*UCRow           // in document order
}

// LoadCharter reads + parses a charter.md file from disk.
func LoadCharter(path string) (*Charter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read charter: %w", err)
	}
	c := &Charter{
		Path: path,
		UCs:  map[string]*UCInfo{},
	}
	body := string(data)

	for _, info := range ParseUseCases(body) {
		c.UCs[info.ID] = info
	}
	c.TestPlan = ParseTestPlan(body)
	return c, nil
}

// ── Use Cases section parser ──────────────────────────────────────────

// ucIDRe matches UC identifiers like UC-001 or UC-053.
var ucIDRe = regexp.MustCompile(`UC-\d+`)

// ucTableRowRe matches a UC row in `## Use Cases` tables:
//
//	| UC-053 | Title text | security | ✓ |
//	| UC-110 | DedupTracker ... | core | ✓ (wired 2026-05-07 by ...) |
//
// Captures: ID, title, severity, DoD column. The DoD column accepts any
// trailing text after the glyph (charter rows often annotate "✓ (wired by ...)").
var ucTableRowRe = regexp.MustCompile(`^\|\s*(UC-\d+)\s*\|\s*(.+?)\s*\|\s*(core|security|qol|cosmetic)\s*\|\s*([✓✗—-]?)[^|]*\|`)

// subsectionHeaderRe matches H4 (`####`) or H5 (`#####`) markdown headers,
// capturing the label text (without leading hashes / leading whitespace).
// Used to tag each UC with its grouping subsection in ParseUseCases.
var subsectionHeaderRe = regexp.MustCompile(`^#{4,5}\s+(.+?)\s*$`)

// ParseUseCases scans the Use Cases section for UC table rows. Returns
// every UC with its title and severity.
//
// Matches the section header at either `## Use Cases` (H2 — used in some
// charters) or `### Use Cases` (H3 — used in the tasks charter, where it
// nests under `## Specifications`). The boundary is the next H2 header.
func ParseUseCases(body string) []*UCInfo {
	var ucs []*UCInfo

	start := -1
	for _, header := range []string{"### Use Cases", "## Use Cases"} {
		if i := strings.Index(body, header); i >= 0 {
			start = i
			break
		}
	}
	if start < 0 {
		return ucs
	}
	// Stop at next H2 header.
	end := indexAfterHeader(body, start, "## ")
	section := body[start:end]

	currentGroup := ""
	for _, line := range strings.Split(section, "\n") {
		// Track the most recent H4/H5 subsection header so each UC gets
		// tagged with its enclosing group label.
		if h := subsectionHeaderRe.FindStringSubmatch(line); h != nil {
			currentGroup = h[1]
			continue
		}
		m := ucTableRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ucs = append(ucs, &UCInfo{
			ID:       m[1],
			Title:    strings.TrimSpace(m[2]),
			Severity: m[3],
			DoDOK:    m[4] == "✓",
			Group:    currentGroup,
		})
	}
	return ucs
}

// indexAfterHeader returns the offset of the next markdown header at or below
// `marker` (e.g. "## "), starting from after the header at `start`. Used to
// scope a section.
func indexAfterHeader(body string, start int, marker string) int {
	// Skip past the header line itself.
	nl := strings.Index(body[start:], "\n")
	if nl < 0 {
		return len(body)
	}
	tail := body[start+nl+1:]
	idx := strings.Index(tail, "\n"+marker)
	if idx < 0 {
		return len(body)
	}
	return start + nl + 1 + idx
}

// ── Test plan parser ───────────────────────────────────────────────────

// testNameRe matches Go test names — by convention `Test...` or `Benchmark...`,
// case-sensitive. Brackets/parens and surrounding text are excluded by the
// `\b` word boundary.
var testNameRe = regexp.MustCompile(`\bTest[A-Z][A-Za-z0-9_]*`)

// tasktestPathRe matches `.tasktest` file paths in L2 cells.
var tasktestPathRe = regexp.MustCompile(`\S*tests/L2/\S+\.tasktest`)

// testPlanRowRe matches a Test plan row's leading `| UC-... |`. The body of
// the row is parsed by splitOnPipes after the lead is recognised.
var testPlanRowRe = regexp.MustCompile(`^\|\s*((?:UC-\d+\s*/?\s*)+)\s*\|`)

// ParseTestPlan scans the `## Test plan` section for UC rows and extracts
// L1 test names + L2 scenario paths. L3 cell is recorded verbatim (no parser
// since L3 is task-name-based and Phase B doesn't run L3).
func ParseTestPlan(body string) []*UCRow {
	var rows []*UCRow

	start := strings.Index(body, "## Test plan")
	if start < 0 {
		return rows
	}
	end := indexAfterHeader(body, start, "## ")
	section := body[start:end]

	for _, line := range strings.Split(section, "\n") {
		if !testPlanRowRe.MatchString(line) {
			continue
		}
		row := parseTestPlanLine(line)
		if row != nil && len(row.IDs) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

// parseTestPlanLine breaks a single Test plan row into its UC IDs + per-cell
// extracted entities. Returns nil if the line doesn't look like a row.
func parseTestPlanLine(line string) *UCRow {
	cells := splitOnPipes(line)
	if len(cells) < 4 {
		return nil
	}

	ids := ucIDRe.FindAllString(cells[0], -1)
	if len(ids) == 0 {
		return nil
	}

	row := &UCRow{IDs: ids}

	// L1 — extract test names.
	if !cellIsEmpty(cells[1]) {
		seen := map[string]bool{}
		for _, name := range testNameRe.FindAllString(cells[1], -1) {
			if !seen[name] {
				seen[name] = true
				row.L1Tests = append(row.L1Tests, name)
			}
		}
	}

	// L2 — extract .tasktest paths. The captured slice may include leading
	// markdown decoration (backticks) because `\S*` is greedy across
	// non-whitespace; strip them so downstream callers see clean paths.
	if len(cells) >= 3 && !cellIsEmpty(cells[2]) {
		raw := tasktestPathRe.FindAllString(cells[2], -1)
		row.L2Scenarios = make([]string, 0, len(raw))
		for _, p := range raw {
			row.L2Scenarios = append(row.L2Scenarios, strings.Trim(p, "`'\""))
		}
	}

	// L3 — record verbatim if present.
	if len(cells) >= 4 && !cellIsEmpty(cells[3]) {
		row.L3Scenarios = []string{strings.TrimSpace(cells[3])}
	}

	// Trailing notes — e.g. "(closed by tier-1-...)"
	if len(cells) >= 5 {
		row.Notes = strings.TrimSpace(cells[4])
	}

	return row
}

// splitOnPipes splits a markdown table row on `|` boundaries. Trims the
// leading/trailing empty pieces produced by the wrapping pipes.
func splitOnPipes(line string) []string {
	parts := strings.Split(line, "|")
	if len(parts) <= 2 {
		return nil
	}
	// Drop the empty leading and (often empty) trailing slot.
	parts = parts[1:]
	if strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// cellIsEmpty returns true when a Test plan cell is the markdown placeholder
// for "no entry" (`—`, `-`, or empty).
func cellIsEmpty(cell string) bool {
	c := strings.TrimSpace(cell)
	return c == "" || c == "—" || c == "-"
}
