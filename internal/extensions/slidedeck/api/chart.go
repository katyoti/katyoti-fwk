// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"strconv"
	"strings"
)

// A chart in a slide: a fenced block whose info string is `chart <kind>`,
// holding comma-separated data — a header (the category column's name, then
// one name per series) and one row per category:
//
//	```chart bar
//	Year, Build, Run
//	2024, 120, 40
//	2025, 90, 60
//	```
//
// The deck component draws it in the theme's colours (chart.tsx); the lint
// checks it here. The two read the same rules.

// ChartKinds are the charts a slide draws.
var ChartKinds = []string{"bar", "line", "pie"}

// Chart is a chart block read.
type Chart struct {
	Kind       string      `json:"kind"`
	Categories []string    `json:"categories"`
	Series     []string    `json:"series"`
	Values     [][]float64 `json:"values"` // per category, one value per series
}

// IsChartFence reports whether a fence line opens a chart block, and its kind
// as written (possibly empty).
func IsChartFence(line string) (kind string, ok bool) {
	t := strings.TrimLeft(strings.TrimSpace(line), "`~")
	fields := strings.Fields(t)
	if len(fields) == 0 || fields[0] != "chart" {
		return "", false
	}
	if len(fields) > 1 {
		kind = fields[1]
	}
	return kind, true
}

// ParseChart reads a chart block's kind and data lines.
func ParseChart(kind string, lines []string) (Chart, error) {
	c := Chart{Kind: kind}
	if !contains(ChartKinds, kind) {
		return c, fmt.Errorf("chart kind %q: write ```chart %s", kind, strings.Join(ChartKinds, "|"))
	}
	var rows [][]string
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		cells := strings.Split(l, ",")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	if len(rows) < 2 {
		return c, fmt.Errorf("a chart needs a header line and at least one data line")
	}
	head := rows[0]
	if len(head) < 2 {
		return c, fmt.Errorf("the header names the category column and at least one series: `Year, Cost`")
	}
	c.Series = head[1:]
	if kind == "pie" && len(c.Series) != 1 {
		return c, fmt.Errorf("a pie chart has one series; this one has %d", len(c.Series))
	}
	for i, r := range rows[1:] {
		if len(r) != len(head) {
			return c, fmt.Errorf("data line %d has %d values; the header has %d columns", i+1, len(r), len(head))
		}
		vals := make([]float64, 0, len(r)-1)
		for _, cell := range r[1:] {
			v, err := strconv.ParseFloat(strings.ReplaceAll(cell, " ", ""), 64)
			if err != nil {
				return c, fmt.Errorf("data line %d: %q is not a number (decimals with a point)", i+1, cell)
			}
			vals = append(vals, v)
		}
		c.Categories = append(c.Categories, r[0])
		c.Values = append(c.Values, vals)
	}
	return c, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
