// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"testing"
)

func TestParseChart(t *testing.T) {
	c, err := ParseChart("bar", []string{"Year, Build, Run", "2024, 120, 40", "", "2025, 90.5, 60"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Categories) != 2 || c.Series[1] != "Run" || c.Values[1][0] != 90.5 {
		t.Fatalf("chart = %+v", c)
	}
	for name, tc := range map[string]struct {
		kind  string
		lines []string
	}{
		"unknown kind":  {"radar", []string{"A, B", "x, 1"}},
		"no data":       {"bar", []string{"A, B"}},
		"one column":    {"line", []string{"A", "x"}},
		"ragged line":   {"bar", []string{"A, B, C", "x, 1"}},
		"not a number":  {"bar", []string{"A, B", "x, many"}},
		"pie of two":    {"pie", []string{"A, B, C", "x, 1, 2"}},
		"decimal comma": {"bar", []string{"A, B", "x, 1,5"}},
	} {
		if _, err := ParseChart(tc.kind, tc.lines); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The lint names a chart whose data cannot be drawn, at its fence line, and
// leaves other code blocks alone.
func TestLintChecksCharts(t *testing.T) {
	src := "---\ntitle: T\n---\n\n## Costs\n```chart bar\nYear, Cost\n2024, lots\n```\n\n---\n\n## Code\n```go\nx := 1\n```\n"
	var found []Finding
	for _, f := range Lint(src, Grammar{}) {
		if f.Rule == "chart-invalid" {
			found = append(found, f)
		}
	}
	if len(found) != 1 || found[0].Line != 6 || !strings.Contains(found[0].Message, "not a number") {
		t.Fatalf("findings = %+v", found)
	}
	ok := "---\ntitle: T\n---\n\n## Costs\n```chart pie\nPart, Share\nBuild, 60\nRun, 40\n```\n"
	for _, f := range Lint(ok, Grammar{}) {
		if f.Rule == "chart-invalid" {
			t.Errorf("a valid chart: %+v", f)
		}
	}
}
