// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/xuri/excelize/v2"
)

// ExcelExtractor converts XLSX/XLS files to markdown tables (or CSV for large sheets).
type ExcelExtractor struct{}

func (e *ExcelExtractor) Name() string      { return "excelize-md" }
func (e *ExcelExtractor) Formats() []string { return []string{"xlsx", "xls"} }

func (e *ExcelExtractor) Extract(_ context.Context, inputPath string, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	f, err := excelize.OpenFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("open excel: %w", err)
	}
	defer f.Close()

	maxMDRows := 50
	maxRows := 0
	if config != nil {
		if v, ok := config["max_md_rows"].(int); ok {
			maxMDRows = v
		}
	}
	if limits != nil && limits.MaxRows > 0 {
		maxRows = limits.MaxRows
	}

	var sb strings.Builder
	sheets := f.GetSheetList()

	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		rows = dropEmptyRows(rows)
		if len(rows) == 0 {
			continue
		}

		// Apply row limit
		if maxRows > 0 && len(rows) > maxRows+1 { // +1 for header
			rows = rows[:maxRows+1]
		}

		sb.WriteString(fmt.Sprintf("## Sheet: %s\n\n", sheet))

		if len(rows) <= maxMDRows+1 {
			// Markdown table
			sb.WriteString(rowsToMarkdown(rows))
		} else {
			// CSV block for large sheets
			sb.WriteString("```csv\n")
			sb.WriteString(rowsToCSV(rows))
			sb.WriteString("```\n")
		}
		sb.WriteString("\n")
	}

	contentFile := "content.md"
	outPath := filepath.Join(outputDir, contentFile)
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write content: %w", err)
	}

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: contentFile, Part: "whole", Index: 0},
		},
		TotalParts: 1,
	}, nil
}

func dropEmptyRows(rows [][]string) [][]string {
	var out [][]string
	for _, row := range rows {
		empty := true
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				empty = false
				break
			}
		}
		if !empty {
			out = append(out, row)
		}
	}
	return out
}

func rowsToMarkdown(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	cols := maxCols(rows)
	var sb strings.Builder

	// Header
	sb.WriteString("| " + joinPadded(rows[0], cols) + " |\n")
	sb.WriteString("| " + strings.Repeat("--- | ", cols) + "\n")

	// Data rows
	for _, row := range rows[1:] {
		sb.WriteString("| " + joinPadded(row, cols) + " |\n")
	}
	return sb.String()
}

func rowsToCSV(rows [][]string) string {
	var sb strings.Builder
	for _, row := range rows {
		quoted := make([]string, len(row))
		for i, cell := range row {
			if strings.ContainsAny(cell, ",\"\n") {
				cell = `"` + strings.ReplaceAll(cell, `"`, `""`) + `"`
			}
			quoted[i] = cell
		}
		sb.WriteString(strings.Join(quoted, ",") + "\n")
	}
	return sb.String()
}

func joinPadded(row []string, cols int) string {
	padded := make([]string, cols)
	for i := range padded {
		if i < len(row) {
			padded[i] = row[i]
		}
	}
	return strings.Join(padded, " | ")
}

func maxCols(rows [][]string) int {
	max := 0
	for _, row := range rows {
		if len(row) > max {
			max = len(row)
		}
	}
	return max
}
