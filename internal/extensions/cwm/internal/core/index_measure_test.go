// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// THE MEASUREMENT 7b PROMISED: numbers, not adjectives. Over the REAL corpus
// (this repository's modules/), how big is what the index keeps per object,
// how much of the files it leaves on disk, and how long a scan and a full
// drawn tree take. Logged, so the numbers are in the run; asserted where a
// regression would be a design failure: a body found resident, or the heap
// one object keeps in the index past IndexHeapPerObjectBound. The
// trigger - the index past IndexHeapTrigger or a scan past IndexScanTrigger -
// is logged, never failed.
func TestMeasureIndexOnRealCorpus(t *testing.T) {
	long.Test(t)
	repo := filepath.Join("..", "..", "..", "..", "..") // the repository, as containment_corpus_test reaches it
	if _, err := os.Stat(filepath.Join(repo, api.ProjectFileName)); err != nil {
		t.Skipf("no project object at the repo root: %v", err)
	}
	root := freshWorkspace(t)
	// NO BOOTSTRAP: this reads the REAL repository, and the bootstrap WRITES
	// (the system inbox, 2026-09-26) — a read-only corpus test must not.
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: repo})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()

	var scan api.ObjectScanOutput
	var scanDur time.Duration
	retained := retainedHeap(func() {
		t0 := time.Now()
		scan, err = svc.ObjectScan(ctx, api.ObjectScanInput{Force: true})
		scanDur = time.Since(t0)
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if scan.Total < 100 {
		t.Fatalf("only %d objects — the corpus is not being read", scan.Total)
	}
	perObject := int(retained) / scan.Total
	if perObject > IndexHeapPerObjectBound {
		t.Errorf("the index keeps %d B of heap per object (%d KB over %d objects), past its bound of %d B: an object costs more in the index - find what grew, or raise the bound with its new measure",
			perObject, retained/1024, scan.Total, IndexHeapPerObjectBound)
	}
	t.Logf("trigger (one project): %s", IndexTrigger(retained, scanDur))

	// Resident bytes (a JSON proxy for what the index holds) vs file bytes.
	all := svc.index().list(api.ObjectListInput{})
	resident, files := 0, 0
	for _, o := range all {
		b, _ := json.Marshal(o)
		resident += len(b)
		if c, ok := o, true; ok && len(c.FieldsJSON()) != 0 {
			t.Errorf("%s: a body is RESIDENT (%d bytes)", o.ID, len(c.FieldsJSON()))
		}
		if p, ok := svc.index().pathOf(o.ID); ok {
			if fi, err := os.Stat(p); err == nil {
				files += int(fi.Size())
			}
		}
	}

	t1 := time.Now()
	tree, err := svc.ObjectTree(ctx, api.ObjectTreeInput{DrawnOnly: true})
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	treeDur := time.Since(t1)
	t2 := time.Now()
	hits, _ := svc.ObjectSearch(ctx, api.ObjectSearchInput{Query: "capability"})
	searchDur := time.Since(t2)

	t.Logf("corpus: %d objects, %d files bytes; resident (json proxy): %d bytes = %d B/object (%.0f%% of the files)",
		scan.Total, files, resident, resident/scan.Total, 100*float64(resident)/float64(files))
	t.Logf("heap the index keeps after the scan: %d KB (%d B/object, bound %d B)", retained/1024, perObject, IndexHeapPerObjectBound)
	t.Logf("scan: %s; drawn tree (%d nodes, %d roots): %s; search: %s (%d hits)",
		scanDur, tree.Total, len(tree.Roots), treeDur, searchDur, hits.Total)
}

// retainedHeap is the heap fn leaves live: the heap in use after a collection,
// before and after it (0 when it freed more than it kept).
func retainedHeap(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.GC()
	runtime.ReadMemStats(&after)
	if after.HeapAlloc < before.HeapAlloc {
		return 0
	}
	return after.HeapAlloc - before.HeapAlloc
}
