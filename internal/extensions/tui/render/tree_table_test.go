// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// TestRenderTreeTable renders a confluence-status-shaped node tree and asserts
// the columns, kind cells (version ✓/✎, direction arrows), indentation, and
// header labels all appear.
func TestRenderTreeTable(t *testing.T) {
	cli := &schemaapi.UICLI{
		Mode:        schemaapi.UICLIModeTreeTable,
		IDField:     "id",
		ParentField: "parent_id",
		StateField:  "status",
		TreeColumn:  "title",
		Columns:     []string{"local_version", "direction", "remote_version", "title", "status"},
		ColumnKinds: map[string]string{
			"local_version":  "version",
			"direction":      "direction",
			"remote_version": "version",
		},
		VersionStates: map[string]string{
			"local_version":  "local_state",
			"remote_version": "remote_state",
		},
		ColumnLabels: map[string]string{
			"local_version":  "Local",
			"direction":      "",
			"remote_version": "Remote",
			"title":          "Page",
			"status":         "Status",
		},
	}

	rows := []any{
		map[string]any{
			"id": "p1", "parent_id": "", "title": "ERP Acme — Vues synthétiques",
			"local_version": "v2", "local_state": "updated", "direction": "upload",
			"remote_version": "v2", "remote_state": "clean", "status": "local updated",
		},
		map[string]any{
			"id": "p2", "parent_id": "", "title": "ERP Acme — Odoo",
			"local_version": "v1", "local_state": "clean", "direction": "skip",
			"remote_version": "v1", "remote_state": "clean", "status": "up to date",
		},
		map[string]any{
			"id": "p2a", "parent_id": "p2", "title": "ERP Acme — Odoo — Produit",
			"local_version": "v2", "local_state": "clean", "direction": "skip",
			"remote_version": "v2", "remote_state": "clean", "status": "up to date",
		},
	}

	var buf bytes.Buffer
	if err := renderTreeTable(nil, rows, cli, RenderCtx{Width: 120}, &buf); err != nil {
		t.Fatalf("renderTreeTable: %v", err)
	}
	got := buf.String()

	for _, want := range []string{
		"Local", "Remote", "Page", "Status", // header labels
		"ERP Acme — Vues synthétiques",
		"ERP Acme — Odoo",
		"✓", "✎", // version glyphs (clean vs updated)
		"▶▶", "═", // direction arrows (upload + skip)
		"local updated", "up to date", // status
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}

	// The child row must be indented under its parent.
	lines := strings.Split(got, "\n")
	var childLine string
	for _, l := range lines {
		if strings.Contains(l, "Produit") {
			childLine = l
		}
	}
	if childLine == "" {
		t.Fatal("child row not rendered")
	}
	if !strings.Contains(childLine, "  ERP Acme — Odoo — Produit") {
		t.Errorf("child row not indented: %q", childLine)
	}
}

// TestRenderTreeTable_ModeDispatch verifies the mode is recognized end-to-end.
func TestRenderTreeTable_ModeDispatch(t *testing.T) {
	// ModeOverride must accept "tree-table".
	ctx := RenderCtx{ModeOverride: "tree-table"}
	if got := resolveMode(nil, ctx); got != schemaapi.UICLIModeTreeTable {
		t.Errorf("resolveMode(tree-table override) = %q, want tree-table", got)
	}
}

// TestRenderTreeTableEnvelopeFooter is the regression guard for the bug where a
// sync run that failed every page rendered identically to a read-only listing:
// the rows carry the PLAN, only the envelope says how the run went. Asserts the
// summary headline and the per-row failure detail both reach the output.
func TestRenderTreeTableEnvelopeFooter(t *testing.T) {
	cli := &schemaapi.UICLI{
		Mode:        schemaapi.UICLIModeTreeTable,
		IDField:     "id",
		ParentField: "parent_id",
		StateField:  "status",
		TreeColumn:  "title",
		NodesField:  "tree",
		Columns:     []string{"title", "status"},

		SummaryField: "summary",
		DetailField:  "message",
	}

	envelope := map[string]any{
		"summary": "push: 1 succeeded, 1 failed",
		"failed":  float64(1),
		"tree": []any{
			map[string]any{"id": "p1", "title": "Dossier d'analyse", "status": "failed",
				"message": "convert body for upload: kroki: context deadline exceeded"},
			map[string]any{"id": "p2", "title": "Compte rendu", "status": "uploaded"},
		},
	}

	var buf bytes.Buffer
	if err := renderTreeTable(nil, envelope, cli, RenderCtx{Width: 120}, &buf); err != nil {
		t.Fatalf("renderTreeTable: %v", err)
	}
	got := buf.String()

	for _, want := range []string{
		"push: 1 succeeded, 1 failed",                // headline
		"Dossier d'analyse: convert body for upload", // failure detail
		"failed", // status column
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	// The succeeding row must NOT produce a failure line.
	if strings.Contains(got, "✗ Compte rendu") {
		t.Errorf("non-failed row listed as a failure:\n%s", got)
	}
}

// TestRenderTreeTableNoFooterWhenUndeclared keeps the footer opt-in: a type that
// declares neither field renders exactly as before.
func TestRenderTreeTableNoFooterWhenUndeclared(t *testing.T) {
	cli := &schemaapi.UICLI{
		Mode: schemaapi.UICLIModeTreeTable, IDField: "id", ParentField: "parent_id",
		StateField: "status", TreeColumn: "title", NodesField: "tree",
		Columns: []string{"title", "status"},
	}
	envelope := map[string]any{
		"summary": "push: 1 succeeded, 1 failed",
		"tree": []any{
			map[string]any{"id": "p1", "title": "Dossier", "status": "failed", "message": "boom"},
		},
	}
	var buf bytes.Buffer
	if err := renderTreeTable(nil, envelope, cli, RenderCtx{Width: 120}, &buf); err != nil {
		t.Fatalf("renderTreeTable: %v", err)
	}
	if got := buf.String(); strings.Contains(got, "push: 1 succeeded") || strings.Contains(got, "boom") {
		t.Errorf("footer rendered without being declared:\n%s", got)
	}
}
