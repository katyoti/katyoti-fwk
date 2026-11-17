// SPDX-License-Identifier: Apache-2.0

package storeset

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// THE JOINED INDEX STAYS IN ITS BOUND. This repository's own
// modules, dealt one in two to two projects of one repository - each served
// by a store of its own and joined by the store set: the heap
// both indexes keep, once scanned, per object of the joined index, stays
// within core.IndexHeapPerObjectBound - two projects cost no more per object
// than one. The trigger is logged for the index the set serves.
func TestTheJoinedIndexOfTwoProjectsStaysInItsBound(t *testing.T) {
	long.Test(t)
	ctx := context.Background()
	source := filepath.Join("..", "..", "..", "..", "..", "modules") // the repository's modules, as the core corpus tests reach them
	modules, err := os.ReadDir(source)
	if err != nil {
		t.Skipf("no modules folder to read: %v", err)
	}
	repo := t.TempDir()
	alphaDir, betaDir := filepath.Join(repo, "alpha"), filepath.Join(repo, "beta")
	// The project objects first, on empty folders: the index a project-init
	// builds is of nothing, so the measure below is the scan's.
	alpha, beta := projectStore(t, alphaDir, "alpha"), projectStore(t, betaDir, "beta")
	set, err := New(alpha, beta)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var names []string
	for _, m := range modules {
		if m.IsDir() && !strings.HasPrefix(m.Name(), ".") {
			names = append(names, m.Name())
		}
	}
	sort.Strings(names)
	for i, m := range names {
		dest := alphaDir
		if i%2 == 1 {
			dest = betaDir
		}
		copyObjectFiles(t, filepath.Join(source, m), filepath.Join(dest, "modules", m))
	}

	var scanned int
	var scanDur time.Duration
	retained := retainedHeap(func() {
		t0 := time.Now()
		for _, st := range []*core.Service{alpha, beta} {
			out, err := st.ObjectScan(ctx, api.ObjectScanInput{Force: true})
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			scanned += out.Total
		}
		scanDur = time.Since(t0)
	})
	joined, err := set.ObjectList(ctx, api.ObjectListInput{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if joined.Total < 100 || joined.Total != scanned {
		t.Fatalf("the joined index lists %d objects, the two scans %d - want the same, past 100", joined.Total, scanned)
	}
	perObject := int(retained) / joined.Total
	t.Logf("joined index of two projects: %d objects, %d KB of heap kept (%d B/object, bound %d B)",
		joined.Total, retained/1024, perObject, core.IndexHeapPerObjectBound)
	t.Logf("trigger (the joined index): %s", core.IndexTrigger(retained, scanDur))
	if perObject > core.IndexHeapPerObjectBound {
		t.Errorf("the joined index keeps %d B of heap per object, past its bound of %d B: an object costs more in the index - find what grew, or raise the bound with its new measure",
			perObject, core.IndexHeapPerObjectBound)
	}
	runtime.KeepAlive(set)
}

// copyObjectFiles copies the object files under from (*.cwm.yaml, *.cwm.md)
// to the same places under to.
func copyObjectFiles(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !(strings.HasSuffix(d.Name(), ".cwm.yaml") || strings.HasSuffix(d.Name(), ".cwm.md")) {
			return nil
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, raw, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s: %v", from, err)
	}
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
