// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"regexp"
	"strings"
)

// tcidUCPattern extracts the UC slug from a test-case id following
// the canonical convention `L<layer>-uc<digits>-<slug>`:
//
//	L1-uc001-executor          → UC-001
//	L2-uc053-spawn-resume      → UC-053
//	L3-uc150-end-to-end-magentic → UC-150
//
// Matching is case-insensitive; capture is the digit run.
var tcidUCPattern = regexp.MustCompile(`(?i)^L[123]-uc(\d{1,5})(?:[-_].*)?$`)

// UCIDFromTCID extracts the canonical UC-NNN string from a
// test-case id. Returns ("", false) if the TCID doesn't follow the
// convention.
//
// Example:
//
//	UCIDFromTCID("L1-uc001-executor") → ("UC-001", true)
//	UCIDFromTCID("freeform-name")     → ("", false)
//
// The padding mirrors the modules/tasks/charter.md convention:
// UC-001 (3 digits) for the first 999, UC-1000+ are full-width.
// We always emit a minimum 3-digit pad so equality comparisons
// against the canonical UC-NNN form work regardless of how the
// TCID was authored.
func UCIDFromTCID(tcid string) (string, bool) {
	matches := tcidUCPattern.FindStringSubmatch(tcid)
	if len(matches) != 2 {
		return "", false
	}
	digits := matches[1]
	// Pad to minimum 3 digits to match UC-001 style canonical form.
	for len(digits) < 3 {
		digits = "0" + digits
	}
	return "UC-" + digits, true
}

// GroupTestCasesByUC indexes a flat slice of test-cases by the UC
// they claim to verify (parsed from TCID). Test-cases whose TCID
// doesn't match the convention land in the unmatched bucket — the
// caller should surface them as orphan-test-case lint signals.
//
// Returns:
//
//   - byUC:     UC-NNN → []indices into the input slice
//   - orphans:  indices of test-cases with non-conforming TCID
func GroupTestCasesByUC[T any](items []T, tcidOf func(T) string) (byUC map[string][]int, orphans []int) {
	byUC = map[string][]int{}
	for i, item := range items {
		tcid := tcidOf(item)
		ucID, ok := UCIDFromTCID(tcid)
		if !ok {
			orphans = append(orphans, i)
			continue
		}
		byUC[ucID] = append(byUC[ucID], i)
	}
	return byUC, orphans
}

// NormalizeUCID accepts any reasonable form of a UC id (UC-001,
// uc-001, uc1, UC-1, UC-0001) and returns the canonical UC-NNN
// (minimum 3 digits). Returns ("", false) if the input doesn't
// contain a digit run.
//
// Used by callers that index by UC id but can't trust how the id
// was authored.
func NormalizeUCID(in string) (string, bool) {
	trimmed := strings.TrimSpace(in)
	if trimmed == "" {
		return "", false
	}
	// Strip prefix variants (uc, UC, uc-, UC-)
	low := strings.ToLower(trimmed)
	low = strings.TrimPrefix(low, "uc-")
	low = strings.TrimPrefix(low, "uc")
	low = strings.TrimPrefix(low, "-")
	if low == "" {
		return "", false
	}
	// Verify it's pure digits
	for _, r := range low {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	for len(low) < 3 {
		low = "0" + low
	}
	return "UC-" + low, true
}
