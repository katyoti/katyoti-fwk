// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
)

// THE DOCUMENT EXPORT METHODS ARE LONG: Render and
// Diagram declare themselves long - run from the action surface they run as
// a method run owning a display panel - and LayoutDiagram, whose callers need
// its graph at once, does not.
func TestTheDocumentExportMethodsAreDeclaredLong(t *testing.T) {
	for _, m := range []string{"Render", "Diagram"} {
		if !api.DocServiceDef.Methods[m].LongRunning {
			t.Errorf("docs/%s is not declared long: an export waits on the bus's 5 s fallback", m)
		}
	}
	if api.DocServiceDef.Methods["LayoutDiagram"].LongRunning {
		t.Error("docs/LayoutDiagram is declared long: its callers would get a run, not the laid-out graph")
	}
}

// AN EXPORT REPORTS THROUGH THE PANELS' API: a diagram
// export run with a panel lent to it (what the method run does, panel.Lend)
// reports each of its steps into that panel as its progress, and its answer is
// unchanged.
func TestADiagramExportReportsItsStepsIntoTheLentPanel(t *testing.T) {
	kroki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/mermaid/svg") {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
	}))
	defer kroki.Close()

	dir := t.TempDir()
	src := filepath.Join(dir, "flow.mmd")
	if err := os.WriteFile(src, []byte("graph TD; A-->B"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := NewDocService(context.Background(), &DocServiceConfig{KrokiURL: kroki.URL})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	var mu sync.Mutex
	var events []panel.Event
	publish := func(_ context.Context, _ string, payload []byte) error {
		var ev panel.Event
		if err := json.Unmarshal(payload, &ev); err != nil {
			t.Errorf("a panel event is no JSON event: %v", err)
		}
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
		return nil
	}
	h, err := panel.OpenPanel(ctx, panel.Scope{Tenant: "t", Universe: "u"}, publish, panel.Open{Type: "ktw-test:/panel-type/progress", Title: "Export"})
	if err != nil {
		t.Fatal(err)
	}

	out, err := svc.Diagram(panel.Lend(ctx, h), api.DiagramInput{Source: src, Format: "svg"})
	if err != nil {
		t.Fatalf("Diagram: %v", err)
	}
	if b, err := os.ReadFile(out.OutputPath); err != nil || !strings.Contains(string(b), "<svg") {
		t.Fatalf("the export wrote %q (%v), want the SVG kroki answered", out.OutputPath, err)
	}

	mu.Lock()
	defer mu.Unlock()
	var steps []panel.Progress
	for _, ev := range events {
		if ev.Kind == panel.KindProgress && ev.Progress != nil {
			steps = append(steps, *ev.Progress)
		}
	}
	if len(steps) != 3 {
		t.Fatalf("the export reported %d steps (%+v), want three: rendering, writing, written", len(steps), steps)
	}
	if last := steps[len(steps)-1]; last.Current != last.Total || last.Total == 0 {
		t.Fatalf("the last step is %+v, want the whole of the export done", last)
	}
	if !strings.Contains(steps[0].Label, "mermaid") {
		t.Fatalf("the first step is %q, want it to name the diagram rendered", steps[0].Label)
	}
}

// A Render reports its first steps into its panel before the converter
// runs: a conversion that fails still showed where it stood.
func TestARenderReportsItsStepsBeforeTheConversion(t *testing.T) {
	svc, _ := NewDocService(context.Background(), &DocServiceConfig{KrokiURL: "http://127.0.0.1:1"})
	ctx := context.Background()
	var labels []string
	publish := func(_ context.Context, _ string, payload []byte) error {
		var ev panel.Event
		_ = json.Unmarshal(payload, &ev)
		if ev.Kind == panel.KindProgress && ev.Progress != nil {
			labels = append(labels, ev.Progress.Label)
		}
		return nil
	}
	h, err := panel.OpenPanel(ctx, panel.Scope{Tenant: "t", Universe: "u"}, publish, panel.Open{Type: "ktw-test:/panel-type/progress"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_, _ = svc.Render(panel.Lend(ctx, h), api.RenderInput{Input: filepath.Join(dir, "missing.md"), Output: filepath.Join(dir, "out.html"), Format: "html"})
	if len(labels) < 2 || !strings.Contains(labels[1], "Pandoc") {
		t.Fatalf("Render reported %v, want the template step then the Pandoc conversion", labels)
	}
}
