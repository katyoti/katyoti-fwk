// SPDX-License-Identifier: Apache-2.0

package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/report"
	"github.com/xuri/excelize/v2"
)

// Formats lists the stream formats Write supports (xlsx is written to a file).
var Formats = []string{"csv", "nljson", "table"}

// Write outputs rows in the specified format.
func Write(format string, rows []map[string]string, cols []report.Column, w io.Writer) error {
	switch format {
	case "csv":
		return WriteCSV(rows, cols, w)
	case "nljson":
		return WriteNLJSON(rows, w)
	case "table":
		return WriteTable(rows, cols, w)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// WriteCSV writes rows as CSV with headers from the column labels.
func WriteCSV(rows []map[string]string, cols []report.Column, w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header row
	headers := make([]string, len(cols))
	ids := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.Label
		if headers[i] == "" {
			headers[i] = c.ID
		}
		ids[i] = c.ID
	}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// Data rows
	for _, row := range rows {
		record := make([]string, len(ids))
		for i, id := range ids {
			record[i] = row[id]
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// WriteNLJSON writes rows as newline-delimited JSON (one JSON object per line).
func WriteNLJSON(rows []map[string]string, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

// WriteTable writes rows as a JSON array of flat objects (column ids as keys).
func WriteTable(rows []map[string]string, cols []report.Column, w io.Writer) error {
	ids := make([]string, len(cols))
	for i, c := range cols {
		ids[i] = c.ID
	}

	var objects []map[string]any
	for _, row := range rows {
		obj := make(map[string]any, len(ids))
		for _, id := range ids {
			obj[id] = row[id]
		}
		objects = append(objects, obj)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(objects)
}

// WriteXLSX writes rows as an Excel .xlsx file with headers, auto-filter, and auto-width.
func WriteXLSX(path string, rows []map[string]string, cols []report.Column) error {
	f := excelize.NewFile()
	sheet := "Sheet1"

	headers := make([]string, len(cols))
	ids := make([]string, len(cols))
	colWidths := make([]float64, len(cols))

	// Styles
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Vertical: "top"},
	})
	cellStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top"},
	})
	wrapStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})

	for col, c := range cols {
		label := c.Label
		if label == "" {
			label = c.ID
		}
		headers[col] = label
		ids[col] = c.ID
		colWidths[col] = float64(len(label))

		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheet, cell, label)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	// Data rows
	for rowIdx, row := range rows {
		for col, id := range ids {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowIdx+2)
			val := row[id]
			f.SetCellValue(sheet, cell, val)
			if strings.Contains(val, "\n") {
				f.SetCellStyle(sheet, cell, cell, wrapStyle)
			} else {
				f.SetCellStyle(sheet, cell, cell, cellStyle)
			}
			if w := float64(len(val)); w > colWidths[col] {
				colWidths[col] = w
			}
		}
	}

	// Auto-width columns (capped at 60)
	for col, w := range colWidths {
		colName, _ := excelize.ColumnNumberToName(col + 1)
		width := w + 2
		if width > 60 {
			width = 60
		}
		f.SetColWidth(sheet, colName, colName, width)
	}

	// Auto-filter on header row
	if len(cols) > 0 {
		lastCol, _ := excelize.ColumnNumberToName(len(cols))
		f.AutoFilter(sheet, fmt.Sprintf("A1:%s1", lastCol), nil)
	}

	// Freeze header row
	f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	return f.SaveAs(path)
}

// SheetData describes a single sheet for multi-sheet Excel output.
type SheetData struct {
	Name    string              // Sheet tab name
	Rows    []map[string]string // Data rows
	Headers []string            // Column headers (display labels)
	IDs     []string            // Column IDs (keys in Rows maps)
}

// WriteMultiSheetXLSX writes a multi-sheet Excel workbook.
func WriteMultiSheetXLSX(path string, sheets []SheetData) error {
	f := excelize.NewFile()

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Vertical: "top"},
	})
	cellStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top"},
	})
	wrapStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})

	for i, sd := range sheets {
		sheetName := sd.Name
		if sheetName == "" {
			sheetName = fmt.Sprintf("Sheet%d", i+1)
		}

		if i == 0 {
			// Rename the default sheet
			f.SetSheetName("Sheet1", sheetName)
		} else {
			f.NewSheet(sheetName)
		}

		if len(sd.Headers) == 0 || len(sd.IDs) == 0 {
			continue
		}

		colWidths := make([]float64, len(sd.Headers))

		// Header row
		for col, label := range sd.Headers {
			cell, _ := excelize.CoordinatesToCellName(col+1, 1)
			f.SetCellValue(sheetName, cell, label)
			f.SetCellStyle(sheetName, cell, cell, headerStyle)
			colWidths[col] = float64(len(label))
		}

		// Data rows
		for rowIdx, row := range sd.Rows {
			for col, id := range sd.IDs {
				cell, _ := excelize.CoordinatesToCellName(col+1, rowIdx+2)
				val := row[id]
				f.SetCellValue(sheetName, cell, val)
				if strings.Contains(val, "\n") {
					f.SetCellStyle(sheetName, cell, cell, wrapStyle)
				} else {
					f.SetCellStyle(sheetName, cell, cell, cellStyle)
				}
				if w := float64(len(val)); w > colWidths[col] {
					colWidths[col] = w
				}
			}
		}

		// Auto-width (capped at 60)
		for col, w := range colWidths {
			colName, _ := excelize.ColumnNumberToName(col + 1)
			width := w + 2
			if width > 60 {
				width = 60
			}
			f.SetColWidth(sheetName, colName, colName, width)
		}

		// Auto-filter on header row
		if len(sd.Headers) > 0 {
			lastCol, _ := excelize.ColumnNumberToName(len(sd.Headers))
			f.AutoFilter(sheetName, fmt.Sprintf("A1:%s1", lastCol), nil)
		}

		// Freeze header row
		f.SetPanes(sheetName, &excelize.Panes{
			Freeze:      true,
			Split:       false,
			XSplit:      0,
			YSplit:      1,
			TopLeftCell: "A2",
			ActivePane:  "bottomLeft",
		})
	}

	return f.SaveAs(path)
}

// SheetFor answers the sheet of a step result: its rows headed by its columns
// (report.HeaderColumns).
func SheetFor(name string, sr *report.StepResult) SheetData {
	cols := report.HeaderColumns(sr)
	sd := SheetData{Name: name, Rows: sr.Rows, Headers: make([]string, len(cols)), IDs: make([]string, len(cols))}
	for i, c := range cols {
		sd.Headers[i] = c.Label
		sd.IDs[i] = c.ID
	}
	return sd
}

// WorkbookSheets answers the sheets a report's xlsx output declares, each
// resolved from the step results ("step" or "step.subOutput").
func WorkbookSheets(def *report.XLSXOutputDef, results map[string]*report.StepResult) ([]SheetData, error) {
	if def == nil {
		return nil, fmt.Errorf("report has no xlsx output definition")
	}
	sheets := make([]SheetData, 0, len(def.Sheets))
	for _, sheetDef := range def.Sheets {
		sr, err := report.ResolveOutputData(sheetDef.Data, results)
		if err != nil {
			return nil, fmt.Errorf("resolve sheet %q data: %w", sheetDef.Name, err)
		}
		sheets = append(sheets, SheetFor(sheetDef.Name, sr))
	}
	return sheets, nil
}
