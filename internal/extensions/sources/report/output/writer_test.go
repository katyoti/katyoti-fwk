// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/report"
	"github.com/xuri/excelize/v2"
)

func TestWriteCSV(t *testing.T) {
	cols := []report.Column{
		{ID: "id", Label: "ID"},
		{ID: "displayName", Label: "Name"},
	}
	rows := []map[string]string{
		{"id": "1", "displayName": "App One"},
		{"id": "2", "displayName": "App Two"},
	}

	var buf bytes.Buffer
	err := WriteCSV(rows, cols, &buf)
	if err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d", len(lines))
	}
	if lines[0] != "ID,Name" {
		t.Errorf("header = %q, want %q", lines[0], "ID,Name")
	}
	if lines[1] != "1,App One" {
		t.Errorf("row 1 = %q, want %q", lines[1], "1,App One")
	}
}

func TestWriteNLJSON(t *testing.T) {
	rows := []map[string]string{
		{"id": "1", "name": "App One"},
		{"id": "2", "name": "App Two"},
	}

	var buf bytes.Buffer
	err := WriteNLJSON(rows, &buf)
	if err != nil {
		t.Fatalf("WriteNLJSON: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"id":"1"`) {
		t.Errorf("line 0 missing id: %s", lines[0])
	}
}

func TestWriteDispatch(t *testing.T) {
	cols := []report.Column{
		{ID: "id", Label: "ID"},
	}
	rows := []map[string]string{{"id": "1"}}

	var buf bytes.Buffer
	if err := Write("csv", rows, cols, &buf); err != nil {
		t.Errorf("Write csv: %v", err)
	}

	buf.Reset()
	if err := Write("nljson", rows, cols, &buf); err != nil {
		t.Errorf("Write nljson: %v", err)
	}

	buf.Reset()
	if err := Write("table", rows, cols, &buf); err != nil || !strings.Contains(buf.String(), `"id": "1"`) {
		t.Errorf("Write table: %v %s", err, buf.String())
	}

	buf.Reset()
	if err := Write("xml", rows, cols, &buf); err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestWriteMultiSheetXLSX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xlsx")

	sheets := []SheetData{
		{
			Name:    "Applications",
			Headers: []string{"ID", "Name"},
			IDs:     []string{"id", "name"},
			Rows: []map[string]string{
				{"id": "1", "name": "App A"},
				{"id": "2", "name": "App B"},
			},
		},
		{
			Name:    "Interfaces",
			Headers: []string{"ID", "Display Name", "Supplier"},
			IDs:     []string{"id", "displayName", "supplier"},
			Rows: []map[string]string{
				{"id": "i1", "displayName": "Iface 1", "supplier": "App A"},
			},
		},
		{
			Name:    "Empty Sheet",
			Headers: nil,
			IDs:     nil,
			Rows:    nil,
		},
	}

	if err := WriteMultiSheetXLSX(path, sheets); err != nil {
		t.Fatalf("WriteMultiSheetXLSX: %v", err)
	}

	// Verify file exists and can be opened
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer f.Close()

	sheetNames := f.GetSheetList()
	if len(sheetNames) != 3 {
		t.Fatalf("sheets = %d, want 3: %v", len(sheetNames), sheetNames)
	}
	if sheetNames[0] != "Applications" {
		t.Errorf("sheet[0] = %q, want Applications", sheetNames[0])
	}
	if sheetNames[1] != "Interfaces" {
		t.Errorf("sheet[1] = %q, want Interfaces", sheetNames[1])
	}

	// Verify data in first sheet
	rows, err := f.GetRows("Applications")
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	if len(rows) != 3 { // header + 2 data rows
		t.Errorf("Applications rows = %d, want 3", len(rows))
	}
	if rows[0][0] != "ID" || rows[0][1] != "Name" {
		t.Errorf("header = %v", rows[0])
	}

	// Verify second sheet
	rows, err = f.GetRows("Interfaces")
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	if len(rows) != 2 { // header + 1 data row
		t.Errorf("Interfaces rows = %d, want 2", len(rows))
	}

	// Cleanup
	os.Remove(path)
}

func TestWriteXLSX(t *testing.T) {
	path := filepath.Join(t.TempDir(), "one.xlsx")
	cols := []report.Column{{ID: "id", Label: "ID"}, {ID: "name"}}
	rows := []map[string]string{{"id": "1", "name": "line one\nline two"}}
	if err := WriteXLSX(path, rows, cols); err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer f.Close()
	got, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	if len(got) != 2 || got[0][0] != "ID" || got[0][1] != "name" || got[1][1] != "line one\nline two" {
		t.Errorf("rows = %q, want the header (label, else id) and the row", got)
	}
}

func TestWorkbookSheets(t *testing.T) {
	results := map[string]*report.StepResult{
		"items": {
			Rows:    []map[string]string{{"id": "1", "name": "One"}},
			Columns: []report.Column{{ID: "id", Label: "ID"}, {ID: "name", Label: "Name"}},
			SubOutputs: map[string]*report.StepResult{
				"rest": {Rows: []map[string]string{{"b": "2", "a": "1"}}},
			},
		},
	}
	def := &report.XLSXOutputDef{Sheets: []report.SheetDef{{Name: "Items", Data: "items"}, {Name: "Rest", Data: "items.rest"}}}
	sheets, err := WorkbookSheets(def, results)
	if err != nil {
		t.Fatalf("WorkbookSheets: %v", err)
	}
	if len(sheets) != 2 || sheets[0].Headers[1] != "Name" || sheets[0].IDs[1] != "name" {
		t.Fatalf("sheets = %+v", sheets)
	}
	if sheets[1].IDs[0] != "a" || sheets[1].IDs[1] != "b" {
		t.Errorf("sheet from rows ids = %v, want a then b", sheets[1].IDs)
	}
	if _, err := WorkbookSheets(&report.XLSXOutputDef{Sheets: []report.SheetDef{{Name: "X", Data: "missing"}}}, results); err == nil {
		t.Error("expected an error for a missing step")
	}
	if _, err := WorkbookSheets(nil, results); err == nil {
		t.Error("expected an error with no xlsx output")
	}
}
