// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// lentPanel is a panel a run lends: the progress the work reports into it,
// in order, as « label current/total ».
func lentPanel(t *testing.T, ctx context.Context) (context.Context, *panel.Handle, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var steps []string
	h, err := panel.Resume(panel.Ref{ID: panel.NewID(), Seq: 1, Tenant: "local", Universe: "test"}, func(_ context.Context, _ string, payload []byte) error {
		e, err := panel.DecodeEvent(payload)
		if err != nil {
			return err
		}
		if e.Progress != nil {
			mu.Lock()
			steps = append(steps, fmt.Sprintf("%s %d/%d", e.Progress.Label, e.Progress.Current, e.Progress.Total))
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return panel.Lend(ctx, h), h, func() []string {
		mu.Lock()
		defer mu.Unlock()
		got := steps
		steps = nil
		return got
	}
}

// Sync and reindex are long methods: declared long, so the
// action surface answers their run and its panel at once, and each reports
// its steps into the panel the run lends it - a sync-all each source then the
// reindex, never the steps of the sync it calls; a connector called within
// carries the panel, so a long one reports there and answers its value.
func TestSyncAndReindexAreLongAndReportIntoTheirPanel(t *testing.T) {
	for _, m := range []string{"SourceSync", "SyncAll", "Reindex"} {
		if !api.SourcesServiceDef.Methods[m].LongRunning {
			t.Errorf("%s is not declared long", m)
		}
	}

	calls := &[]actionsapi.RunInput{}
	ctx, cwm, svc, root, _, _ := knowledgeWorldCalls(t, calls)
	ext := filepath.Join(root, "ext")
	_ = os.MkdirAll(ext, 0o755)
	for _, n := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(ext, n), []byte("# "+n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	create := func(local string, fields map[string]any) string {
		t.Helper()
		obj := cwmapi.NewContainerObject("m", local, api.InformationSourceType, "m", local, "")
		raw, _ := json.Marshal(fields)
		obj.SetFieldsJSON(raw)
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
		if err != nil {
			t.Fatalf("create %s: %v", local, err)
		}
		return out.Object.ID
	}
	folder := create("specs", map[string]any{"connector": api.FolderConnector, "location": map[string]any{"path": "ext"}})
	create("wiki", map[string]any{"connector": "wiki-page", "location": map[string]any{"page": "home"}})
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(of.KBPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(of.KBPath, "KnowledgeBase.yaml"), []byte("name: m\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A source synced alone: read by its connector, each item anchored, the
	// source recorded.
	lent, h, steps := lentPanel(t, ctx)
	if res, err := svc.SourceSync(lent, api.SourceSyncInput{ID: folder}); err != nil || !res.OK {
		t.Fatalf("source-sync: %v %+v", err, res)
	}
	want := "Lecture de la source par son connecteur 0/0 | Ancrage des éléments lus 0/2 | Ancrage des éléments lus 1/2 | Enregistrement de la source 2/2"
	if got := strings.Join(steps(), " | "); got != want {
		t.Errorf("source-sync's steps:\n got %s\nwant %s", got, want)
	}

	// Sync-all: each source, then the reindex - the steps of each sync it
	// calls stay out of its bar.
	*calls = nil
	if res, err := svc.SyncAll(lent, api.SyncAllInput{ID: "m"}); err != nil || !res.OK || !res.Reindexed {
		t.Fatalf("sync-all: %v %+v", err, res)
	}
	want = "Synchronisation de specs 0/3 | Synchronisation de wiki 1/3 | Réindexation de la base de connaissance 2/3 | Base de connaissance réindexée 3/3"
	if got := strings.Join(steps(), " | "); got != want {
		t.Errorf("sync-all's steps:\n got %s\nwant %s", got, want)
	}
	// The connectors and the engine it called carry the lent panel (the
	// save gate CWM runs is its own call).
	carried := 0
	for _, c := range *calls {
		if c.URI == "ktw-agents:sources/source-check" {
			continue
		}
		if c.Context["panel"] != h.ID() {
			t.Errorf("%s was called without the run's panel: %v", c.URI, c.Context)
		}
		carried++
	}
	if carried != 3 {
		t.Errorf("sync-all made %d calls, want the two connectors and kb/run", carried)
	}

	// Reindex alone: one step.
	if res, err := svc.Reindex(lent, api.ReindexInput{ID: "m"}); err != nil || !res.OK {
		t.Fatalf("reindex: %v %+v", err, res)
	}
	want = "Réindexation de la base de connaissance 0/1 | Base de connaissance réindexée 1/1"
	if got := strings.Join(steps(), " | "); got != want {
		t.Errorf("reindex's steps:\n got %s\nwant %s", got, want)
	}

	// Outside a run (a timer, a deposit) nothing is reported and a call
	// carries no panel.
	*calls = nil
	if res, err := svc.SyncAll(ctx, api.SyncAllInput{ID: "m"}); err != nil || !res.OK {
		t.Fatalf("sync-all outside a run: %v %+v", err, res)
	}
	for _, c := range *calls {
		if c.Context != nil {
			t.Errorf("%s called outside a run carries %v", c.URI, c.Context)
		}
	}
}
