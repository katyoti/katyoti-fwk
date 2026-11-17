// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// write drops a raw object file at a path under the workspace, creating
// intermediate folders. Used to build layouts the API cannot express, which is
// the point — the API has no notion of a subfolder.
func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func objDoc(id string) string {
	return "id: " + id + "\ntype: feature\ncapabilities:\n  want: {title: " + id + "}\n"
}

// An object filed in a SUBFOLDER is found.
//
// Until 2026-09-20 the walk did `if f.IsDir() { continue }` — which did not
// error, it made the object DISAPPEAR. Anyone organising a 47-file folder lost
// work with no signal at all.
func TestObjectsInSubfoldersAreFound(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	write(t, root, "modules/demo/objects/flat.cwm.yaml", objDoc("demo.flat"))
	write(t, root, "modules/demo/objects/goals/g1.cwm.yaml", objDoc("demo.g1"))
	write(t, root, "modules/demo/objects/phases/deep/p1.cwm.yaml", objDoc("demo.p1"))

	out, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "demo"})
	if err != nil {
		t.Fatalf("ObjectList: %v", err)
	}
	got := map[string]bool{}
	for _, o := range out.Objects {
		got[o.ID] = true
	}
	for _, want := range []string{"demo.flat", "demo.g1", "demo.p1"} {
		if !got[want] {
			t.Errorf("%q not found — an object in a subfolder is invisible", want)
		}
	}
}

// THE PATH IS NEVER SEMANTIC. Moving an object between subfolders changes
// nothing: not its id, not its module, not its place in the tree.
//
// This is what keeps grouping a filing decision. The moment behaviour depends on
// the folder, renaming a folder changes meaning and we are back to
// path-as-identity, which this model rejects.
func TestMovingAnObjectBetweenFoldersChangesNothing(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	write(t, root, "modules/demo/objects/a/thing.cwm.yaml", objDoc("demo.thing"))
	before, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "demo", ID: "demo.thing"})
	if err != nil {
		t.Fatalf("get before: %v", err)
	}

	// Move it, and force a rebuild so the index re-reads from disk.
	if err := os.MkdirAll(filepath.Join(root, "modules/demo/objects/b"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Rename(
		filepath.Join(root, "modules/demo/objects/a/thing.cwm.yaml"),
		filepath.Join(root, "modules/demo/objects/b/thing.cwm.yaml"),
	); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	after, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "demo", ID: "demo.thing"})
	if err != nil {
		t.Fatalf("get after the move: %v — the path was load-bearing", err)
	}
	if after.Object.ID != before.Object.ID || after.Object.Type != before.Object.Type {
		t.Errorf("the object changed when its folder did: %+v vs %+v", after.Object, before.Object)
	}
}

// A DUPLICATE ID IS AN ERROR naming both files — never a silent overwrite
// whose winner depends on directory read order, and never a project that
// refuses to open: the first file is loaded, the second is listed as an error
// naming both.
//
// A flat folder made this catchable because filenames mirror ids. Subfolders
// hide it completely, so the guard exists before the folders do.
func TestDuplicateIdIsReportedNamingBothFiles(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	write(t, root, "modules/demo/objects/one/dup.cwm.yaml", objDoc("demo.dup"))
	write(t, root, "modules/demo/objects/two/dup.cwm.yaml", objDoc("demo.dup"))

	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
		t.Fatalf("a duplicate made the project unopenable: %v", err)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "demo", ID: "demo.dup"}); err != nil {
		t.Errorf("the first file was not loaded: %v", err)
	}
	errs, err := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs.Unreadable) != 1 {
		t.Fatalf("unreadable = %+v, want the second file", errs.Unreadable)
	}
	msg := errs.Unreadable[0].Path + " " + strings.Join(errs.Unreadable[0].Problems, " ")
	for _, want := range []string{api.DiagnosticDuplicateID, "demo.dup", "one", "two"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the error does not name %q, so the duplicate cannot be found: %s", want, msg)
		}
	}
}

// `_`-prefixed folders are skipped, matching the existing `_`-prefixed FILE
// convention, so `_archive/` works without a special case.
func TestUnderscoreFoldersAreSkipped(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	write(t, root, "modules/demo/objects/live.cwm.yaml", objDoc("demo.live"))
	write(t, root, "modules/demo/objects/_archive/old.cwm.yaml", objDoc("demo.old"))

	out, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "demo"})
	if err != nil {
		t.Fatalf("ObjectList: %v", err)
	}
	for _, o := range out.Objects {
		if o.ID == "demo.old" {
			t.Error("an object under _archive/ was indexed; the folder convention is not applied to folders")
		}
	}
	found := false
	for _, o := range out.Objects {
		if o.ID == "demo.live" {
			found = true
		}
	}
	if !found {
		t.Error("the live object was lost while skipping _archive/")
	}
}

// The walk is SORTED at every level. Directory order is not guaranteed, and an
// index whose order varies between runs moves hashes for no reason — which has
// already cost this refactor two false census diffs.
func TestWalkOrderIsDeterministic(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	for _, n := range []string{"zeta", "alpha", "mid"} {
		write(t, root, "modules/demo/objects/"+n+"/o.cwm.yaml", objDoc("demo."+n))
	}
	var first []string
	for i := 0; i < 3; i++ {
		if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "demo"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var ids []string
		for _, o := range out.Objects {
			ids = append(ids, o.ID)
		}
		if i == 0 {
			first = ids
			continue
		}
		if strings.Join(ids, ",") != strings.Join(first, ",") {
			t.Fatalf("order varies between runs: %v vs %v", first, ids)
		}
	}
}
