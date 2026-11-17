// SPDX-License-Identifier: Apache-2.0

// bench-regression-check parses benchstat CSV output (`-format=csv`) and
// exits non-zero when any benchmark regresses beyond the configured
// threshold AND the change is statistically significant.
//
// Usage:
//
//	bench-regression-check -threshold=10 < benchstat.csv
//	bench-regression-check -threshold=10 -alpha=0.05 -file=cmp.csv
//
// Reads the multi-table CSV emitted by `benchstat -format=csv base new`.
// Each table has columns: name, base-metric, base-CI, new-metric, new-CI,
// "vs base", "P". The `vs base` column is either `~` (noise — pass),
// `+X.XX%` (regression — gated by threshold + alpha), or `-X.XX%`
// (improvement — always pass).
//
// Exits 0 if no regression > threshold with p < alpha; exits 1 otherwise.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	threshold := flag.Float64("threshold", 10.0, "fail if any benchmark regresses by more than this percentage")
	alpha := flag.Float64("alpha", 0.05, "only fail if the change is statistically significant at this level")
	file := flag.String("file", "", "read benchstat CSV from this file (default stdin)")
	flag.Parse()

	var r io.Reader = os.Stdin
	if *file != "" {
		f, err := os.Open(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(2)
		}
		defer f.Close()
		r = f
	}

	regressions, err := parseRegressions(r, *threshold, *alpha)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(2)
	}

	if len(regressions) == 0 {
		fmt.Println("bench-regression-check: PASS — no significant regression > " +
			strconv.FormatFloat(*threshold, 'f', 1, 64) + "%")
		return
	}

	fmt.Fprintf(os.Stderr, "bench-regression-check: FAIL — %d regression(s):\n", len(regressions))
	for _, reg := range regressions {
		fmt.Fprintf(os.Stderr, "  %s: %s (%s)\n", reg.name, reg.deltaStr, reg.pValue)
	}
	os.Exit(1)
}

type regression struct {
	name     string
	deltaStr string
	pValue   string
}

func parseRegressions(r io.Reader, threshold, alpha float64) ([]regression, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}

	var regressions []regression

	// Walk the rows. Each "table" starts with a header row whose 2nd cell is
	// a path (the base file). Within a table, data rows have benchmark name
	// in col 0 and the "vs base" column at index 5, "P" at index 6.
	const (
		colName  = 0
		colDelta = 5
		colP     = 6
	)

	inTable := false
	for _, row := range rows {
		if len(row) < 7 {
			inTable = false
			continue
		}
		// A header row's first cell is empty and the row contains `vs base`.
		if row[colName] == "" {
			if containsField(row, "vs base") {
				inTable = true
			} else {
				inTable = false
			}
			continue
		}
		if !inTable {
			continue
		}
		// Skip the geomean summary row.
		if strings.EqualFold(row[colName], "geomean") {
			continue
		}
		delta := strings.TrimSpace(row[colDelta])
		if delta == "" || delta == "~" {
			continue // noise — benchstat says not significant
		}
		// delta is like "+3.42%" or "-1.10%"
		deltaStr := strings.TrimSuffix(delta, "%")
		deltaStr = strings.TrimPrefix(deltaStr, "+")
		pct, err := strconv.ParseFloat(deltaStr, 64)
		if err != nil {
			continue
		}
		if pct <= threshold {
			continue
		}
		// p-value column: "p=0.001 n=10" — parse the leading p=...
		p := parsePValue(row[colP])
		if p > alpha {
			continue
		}
		regressions = append(regressions, regression{
			name:     row[colName],
			deltaStr: delta,
			pValue:   row[colP],
		})
	}
	return regressions, nil
}

func containsField(row []string, want string) bool {
	for _, cell := range row {
		if strings.EqualFold(strings.TrimSpace(cell), want) {
			return true
		}
	}
	return false
}

func parsePValue(s string) float64 {
	// e.g. "p=0.001 n=10"
	idx := strings.Index(s, "p=")
	if idx < 0 {
		return 1.0
	}
	rest := s[idx+2:]
	end := strings.IndexAny(rest, " \t")
	if end > 0 {
		rest = rest[:end]
	}
	v, err := strconv.ParseFloat(rest, 64)
	if err != nil {
		return 1.0
	}
	return v
}
