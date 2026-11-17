// SPDX-License-Identifier: Apache-2.0

// Package projection renders the coverage views of a module from its use
// cases and acceptance criteria, statused by the latest test-run verdicts:
//
//   - the charter's "## Test plan" section (per-UC, per-AC status table) —
//     a section of the charter the charter publish renders whole
//   - the manifest.json "coverage" block (counters, per-layer histogram,
//     severity×status, functional rollup) — a field of the manifest the
//     manifest write renders whole
//   - the browsable use-cases index (use-cases/README.md, + the opt-in docs
//     page with --docs-dir), written by `module project`
//
// Nothing here splices into a file: each file has one writer, which renders
// it whole. All output is deterministic and byte-stable: regenerating with
// unchanged inputs produces identical bytes (the `module project --check` CI
// guard relies on it).
package projection

import (
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
)

// statusEmoji maps the ucfile 5-state to the display glyphs used across
// charter / verify / docs.
func statusEmoji(status string) string {
	switch status {
	case ucfile.StatusPass:
		return "🟢"
	case ucfile.StatusFail:
		return "🔴"
	case ucfile.StatusFailDeferred:
		return "🟠"
	case ucfile.StatusNoTest:
		return "⚪"
	case ucfile.StatusSkipped:
		return "⏸"
	}
	return "❓"
}

// sortedByID returns the files ordered by UC number (stable display order).
func sortedByID(files []*ucfile.UCFile) []*ucfile.UCFile {
	out := append([]*ucfile.UCFile(nil), files...)
	sort.Slice(out, func(i, j int) bool {
		var a, b int
		fmt.Sscanf(out[i].Meta.ID, "UC-%d", &a)
		fmt.Sscanf(out[j].Meta.ID, "UC-%d", &b)
		return a < b
	})
	return out
}

// layerCell renders one Test-plan layer cell: each AC's ref + status glyph,
// anti-criteria marked. Empty cell renders the markdown placeholder.
func layerCell(acs []ucfile.AC, layer string) string {
	var parts []string
	for _, ac := range acs {
		if ac.Layer != layer {
			continue
		}
		ref := ac.TestRef
		if ref == "" {
			ref = "(no probe)"
		} else {
			ref = "`" + ref + "`"
		}
		entry := ref + " " + statusEmoji(ac.Status)
		if ac.IsAnti() {
			entry += " [anti]"
		}
		parts = append(parts, entry)
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}

// RenderTestPlanSection renders the full generated `## Test plan` section
// body (heading included) for a module's charter.
func RenderTestPlanSection(module string, files []*ucfile.UCFile) string {
	var b strings.Builder
	b.WriteString("## Test plan\n\n")
	b.WriteString("> **GENERATED** from [`use-cases/`](use-cases/) by `ktw-code module project " + module + "` — do not hand-edit this section. ")
	b.WriteString("UC authoring happens in the per-UC files; `module verify` writes the statuses; this table is a projection.\n\n")
	// Column layout is PINNED to verify's legacy Test-plan parser
	// (UC | L1 | L2 | L3 — parseTestPlanLine indexes cells by offset).
	// No extra columns: the UC rollup status lives in use-cases/README.md
	// + manifest coverage; per-AC emojis ride inside the layer cells.
	// TestRenderTestPlan_ParserRoundTrip enforces this.
	b.WriteString("| UC ID | L1 (Go unit) | L2 (CLI black-box) | L3 (self-hosted — user-request only) |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, f := range sortedByID(files) {
		m := f.Meta
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			m.ID,
			layerCell(m.AcceptanceCriteria, "L1"),
			layerCell(m.AcceptanceCriteria, "L2"),
			layerCell(m.AcceptanceCriteria, "L3"),
		)
	}
	return b.String()
}

// RenderDocsUseCases renders the docs-module use-cases page as a projection
// of the per-UC files.
func RenderDocsUseCases(module, title string, files []*ucfile.UCFile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — Use Cases\n\n", title)
	fmt.Fprintf(&b, "> **GENERATED** from `modules/%s/use-cases/` by `ktw-code module project %s` — do not hand-edit. ", module, module)
	b.WriteString("One file per UC is the source of truth; statuses are written by `module verify`.\n")
	for _, f := range sortedByID(files) {
		m := f.Meta
		fmt.Fprintf(&b, "\n### %s. %s %s\n\n", m.ID, m.Title, statusEmoji(m.StatusCache))
		if d := strings.TrimSpace(m.Description); d != "" {
			fmt.Fprintf(&b, "%s\n\n", d)
		}
		fmt.Fprintf(&b, "**Severity**: %s", m.Severity)
		if m.FailDeferredBy != "" {
			fmt.Fprintf(&b, " · **Deferred by**: %s", m.FailDeferredBy)
		}
		if len(m.Realizes) > 0 {
			fmt.Fprintf(&b, " · **Realizes**: %s", strings.Join(m.Realizes, ", "))
		}
		b.WriteString("\n")
		for _, ac := range m.AcceptanceCriteria {
			kind := ""
			if ac.IsAnti() {
				kind = " *(anti)*"
			}
			ref := ac.TestRef
			if ref == "" {
				ref = "no probe"
			}
			fmt.Fprintf(&b, "- %s **%s** [%s]%s — %s (`%s`)\n",
				statusEmoji(ac.Status), ac.ID, ac.Layer, kind, ac.Claim, ref)
		}
	}
	return b.String()
}
