// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// scanWithWriteInside runs one scan with write landing between its read of the
// files and its swap — the window a concurrent put or drop can hit.
func scanWithWriteInside(t *testing.T, svc *Service, write func()) api.ObjectScanOutput {
	t.Helper()
	ix := svc.index()
	if err := ix.ensureLoaded(); err != nil {
		t.Fatal(err)
	}
	ix.afterRead = write
	defer func() { ix.afterRead = nil }()
	out, err := ix.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

// TestScanKeepsAPutMadeWhileItReads pins that a create and an update landing
// between a scan's read and its swap survive the swap.
func TestScanKeepsAPutMadeWhileItReads(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()
	a := obj("cwm.a", "feature", "")
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: a}); err != nil {
		t.Fatal(err)
	}
	scanWithWriteInside(t, svc, func() {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.b", "feature", "cwm.a")}); err != nil {
			t.Errorf("create during scan: %v", err)
		}
		a.Name = "renamed during the scan"
		if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: a}); err != nil {
			t.Errorf("update during scan: %v", err)
		}
	})
	ix := svc.index()
	if _, ok := ix.get("cwm.b"); !ok {
		t.Fatal("an object created while the scan read is lost from the index")
	}
	if got, _ := ix.get("cwm.a"); got.Name != "renamed during the scan" {
		t.Fatalf("an update made while the scan read is reverted: name = %q", got.Name)
	}
	if kids := ix.subtree("cwm.a"); !kids["cwm.b"] {
		t.Fatal("the tree lost the child created while the scan read")
	}
	if hits := ix.fulltext("renamed"); len(hits) == 0 {
		t.Fatal("the full-text index reverted the update made while the scan read")
	}
}

// TestScanKeepsADropMadeWhileItReads pins that a delete landing between a
// scan's read and its swap stays deleted.
func TestScanKeepsADropMadeWhileItReads(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()
	for _, id := range []string{"cwm.a", "cwm.b"} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj(id, "feature", "")}); err != nil {
			t.Fatal(err)
		}
	}
	scanWithWriteInside(t, svc, func() {
		if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "cwm", ID: "cwm.b"}); err != nil {
			t.Errorf("delete during scan: %v", err)
		}
	})
	if _, ok := svc.index().get("cwm.b"); ok {
		t.Fatal("an object deleted while the scan read is back in the index")
	}
	if _, ok := svc.index().get("cwm.a"); !ok {
		t.Fatal("the scan lost an untouched object")
	}
}

// TestEmptyScanRecomputesNoProblems pins that a scan finding nothing changed
// leaves the problems as they are — and that a scan that does find a change
// still marks everything.
func TestEmptyScanRecomputesNoProblems(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.a", "feature", "")}); err != nil {
		t.Fatal(err)
	}
	ix := svc.index()
	svc.ensureProblems() // takes every pending mark
	gen := ix.generation()

	if out, err := ix.scan(); err != nil || len(out.Added)+len(out.Modified)+len(out.Removed) != 0 {
		t.Fatalf("unchanged rescan: %+v, %v", out, err)
	}
	if all, dirty := ix.takeDirty(); all || len(dirty) != 0 {
		t.Fatalf("an empty scan marked problems stale: all=%v dirty=%v", all, dirty)
	}
	if ix.generation() != gen {
		t.Fatal("an empty scan moved the module objects' generation")
	}

	path, _ := ix.pathOf("cwm.a")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte("# edited outside\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _ := ix.scan(); len(out.Modified) != 1 {
		t.Fatalf("modified = %v, want [cwm.a]", out.Modified)
	}
	if all, _ := ix.takeDirty(); !all {
		t.Fatal("a scan that found a change did not mark the problems stale")
	}
}
