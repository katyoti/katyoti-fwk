// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A sidecar's existing progress reporting feeds its panel's handle
// : the work's progress.Emit calls, on the context ReportTo
// answers, go out as the panel's progress and as a merge of a node's data;
// the work changes nothing of how it reports.
func TestASidecarsProgressReportingFeedsItsPanel(t *testing.T) {
	ctx := context.Background()
	publish, got := recorder(t)
	h, err := OpenPanel(ctx, scope, publish, Open{Type: "ktw-test:/panel-type/sync", Title: "Sync"})
	if err != nil {
		t.Fatal(err)
	}
	// The sidecar's work, as it reports today: progress.Emit on its context.
	work := func(ctx context.Context) {
		progress.Emit(ctx, Progress{Label: "pages", Current: 1, Total: 3})
		progress.Emit(ctx, map[string]any{"pages": []any{"Home"}})
		progress.Emit(ctx, &Progress{Label: "pages", Current: 3, Total: 3})
		progress.Emit(ctx, nil)
	}
	work(ReportTo(ctx, h, "sync:tree"))

	events := got()
	if len(events) != 4 {
		t.Fatalf("sent %d events, want the open and three reports: %+v", len(events), events)
	}
	if p := events[1].event; p.Kind != KindProgress || p.Progress.Current != 1 || p.Progress.Total != 3 || p.Seq != 2 {
		t.Fatalf("the first report is %+v, want the progress 1 of 3 at seq 2", p)
	}
	o := events[2].event
	if o.Kind != KindOps || len(o.Ops) != 1 || o.Ops[0].Op != render.OpMerge || o.Ops[0].Target != "sync:tree" {
		t.Fatalf("a data snapshot went out as %+v, want one merge on sync:tree", o)
	}
	if data, _ := o.Ops[0].Value.(map[string]any)["data"].(map[string]any); data == nil || data["pages"] == nil {
		t.Fatalf("the merge carries %+v, want the snapshot as the node's data", o.Ops[0].Value)
	}
	if p := events[3].event; p.Kind != KindProgress || p.Progress.Current != 3 {
		t.Fatalf("the last report is %+v, want the progress 3 of 3", p)
	}

	// No target: a data snapshot is dropped, a progress still goes.
	publish2, got2 := recorder(t)
	h2, _ := OpenPanel(ctx, scope, publish2, Open{Type: "ktw-test:/panel-type/sync"})
	quiet := ReportTo(ctx, h2, "")
	progress.Emit(quiet, map[string]any{"pages": 1})
	progress.Emit(quiet, Progress{Label: "pages", Current: 1})
	if ev := got2(); len(ev) != 2 || ev[1].event.Kind != KindProgress {
		t.Fatalf("without a target sent %+v, want the open and the progress", ev)
	}

	// The panel ended: a late report is not the work's failure.
	if err := h.Complete(ctx, "ok", nil); err != nil {
		t.Fatal(err)
	}
	progress.Emit(ReportTo(ctx, h, "sync:tree"), Progress{Label: "late"})
	if n := len(got()); n != 5 {
		t.Fatalf("after the end %d events, want the complete and no report", n)
	}
}
