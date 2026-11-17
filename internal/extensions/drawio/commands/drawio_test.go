// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

func writeDiagram(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "d.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewDrawioCommand()
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

// TestValidate_ReportsErrorsAndFails asserts validate prints each finding
// and exits non-zero when there is an error.
func TestValidate_ReportsErrorsAndFails(t *testing.T) {
	p := writeDiagram(t, `{"stencilSet":"","nodes":[{"id":"a","label":"A","kind":"X"},{"id":"a","label":"B","kind":"X"}]}`)
	out, err := run(t, "validate", "--json", p)
	if err == nil {
		t.Fatalf("want a failing exit, got none; output:\n%s", out)
	}
	var findings []issueJSON
	if jerr := json.Unmarshal([]byte(out), &findings); jerr != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", jerr, out)
	}
	if len(findings) == 0 || findings[0].Severity != "error" {
		t.Fatalf("want error findings, got %+v", findings)
	}
}

// TestLint_WarnsWithoutFailing asserts lint never fails on a warning.
func TestLint_WarnsWithoutFailing(t *testing.T) {
	p := writeDiagram(t, `{"stencilSet":"x:/stencil-set/y","nodes":[{"id":"a","label":"A","kind":"X"}]}`)
	out, err := run(t, "lint", p)
	if err != nil {
		t.Fatalf("lint failed on warnings: %v\n%s", err, out)
	}
	if !strings.Contains(out, "orphan") {
		t.Errorf("want the orphan warning, got:\n%s", out)
	}
}

// TestExport_RejectsUnknownFormat asserts the format is checked before
// draw.io desktop is looked for.
func TestExport_RejectsUnknownFormat(t *testing.T) {
	if _, err := run(t, "export", "x.drawio", "--format", "gif"); err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("want unsupported-format error, got %v", err)
	}
}

// TestRead_PrintsModel asserts read prints the model of a .drawio file.
func TestRead_PrintsModel(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.drawio")
	body := `<mxfile><diagram><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/>` +
		`<mxCell id="a" value="A" vertex="1" parent="1"><mxGeometry x="1" y="2" width="3" height="4" as="geometry"/></mxCell>` +
		`</root></mxGraphModel></diagram></mxfile>`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "read", "--model", p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(out, `"label": "A"`) || !strings.Contains(out, `"kind": "unknown"`) {
		t.Errorf("unexpected model:\n%s", out)
	}
}

func runIn(t *testing.T, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	cmd := NewDrawioCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	return out.String(), err
}

// TestDiscovery_ListsTheShippedViewpointsAndNotations: the viewpoints and
// notations verbs list what the catalog holds, and a notation's kinds.
func TestDiscovery_ListsTheShippedViewpointsAndNotations(t *testing.T) {
	ctx := drawiotesting.WithCatalog(t, drawiotesting.ShippedCatalog(t))
	out, err := runIn(t, ctx, "viewpoints")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"archimate3-layered", "archimate3-application-cooperation", "uml2-class", "uml2-overview"} {
		if !strings.Contains(out, "ktw-docs:/viewpoint/"+id) {
			t.Errorf("viewpoints: %s missing\n%s", id, out)
		}
	}
	out, err = runIn(t, ctx, "notations")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ktw-docs:/notation/archimate3") || !strings.Contains(out, "ktw-docs:/notation/uml2") {
		t.Errorf("notations:\n%s", out)
	}
	out, err = runIn(t, ctx, "notations", "ktw-docs:/notation/archimate3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "BusinessActor") || !strings.Contains(out, "relationship kinds:") {
		t.Errorf("notation kinds:\n%s", out)
	}
}

// TestCheck_FailsOnOverlappingText: `drawio check` exits 1 on a gated
// finding or an unreadable file, passes a drawing whose layering is
// deliberate, and gives each file's findings and counts as JSON.
func TestCheck_FailsOnOverlappingText(t *testing.T) {
	fixture := func(name string) string { return filepath.Join("..", "render", "testdata", "check", name) }
	if out, err := run(t, "check", fixture("overlap.drawio")); err == nil || !strings.Contains(out, "text-overlap") {
		t.Fatalf("want a failing exit naming the text-overlap, got %v:\n%s", err, out)
	}
	if out, err := run(t, "check", fixture("layering.drawio")); err != nil {
		t.Fatalf("layering failed the check: %v\n%s", err, out)
	}
	if _, err := run(t, "check", "--strict", fixture("portfolio-snapshot.drawio")); err != nil {
		t.Fatalf("the snapshot fails --strict: %v", err)
	}
	if _, err := run(t, "check", filepath.Join(t.TempDir(), "missing.drawio")); err == nil {
		t.Fatal("a missing file passed the check")
	}
	out, err := run(t, "check", "--json", fixture("overlap.drawio"), fixture("layering.drawio"))
	if err == nil {
		t.Fatal("want a failing exit")
	}
	var files []checkFileJSON
	if jerr := json.Unmarshal([]byte(out), &files); jerr != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", jerr, out)
	}
	if len(files) != 2 || files[0].Gated != 1 || files[0].Counts["text-overlap"] != 1 || files[1].Gated != 0 {
		t.Fatalf("unexpected JSON: %+v", files)
	}
}
