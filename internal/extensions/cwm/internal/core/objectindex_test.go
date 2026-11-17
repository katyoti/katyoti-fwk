// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmevents "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// obj builds a test object with an optional parent.
func obj(id, typ, parent string) api.Object {
	caps := map[string]map[string]any{
		"want": {"reachability": "closable"},
	}
	return api.Object{ID: id, Type: typ, Name: id, Parent: parent, Capabilities: caps}
}

func newSvc(t *testing.T) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc, root
}

// TestObjectCreateThenRead is THE test for why the index exists.
//
// p1d (migrating consumers off Node*/Item*) was blocked because ObjectCreate
// writes a FILE while reads answered from the store — so a consumer would write
// an object and then fail to find it. gocode's methods_uc.go does exactly this:
// ItemCreate then ItemGet. This pins that the pattern now works.
func TestObjectCreateThenRead(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.a", "feature", "")}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Read it back through the INDEX (List), not through Get-by-path.
	out, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "cwm"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if out.Total != 1 || out.Objects[0].ID != "cwm.a" {
		t.Fatalf("create-then-read failed: got %d objects %+v", out.Total, out.Objects)
	}
}

// TestObjectListFilters pins that the filters AND together and that an empty
// input lists everything.
func TestObjectListFilters(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()
	for _, o := range []api.Object{
		obj("cwm.root", "module", ""),
		obj("cwm.g1", "feature", "cwm.root"),
		obj("cwm.g2", "feature", "cwm.root"),
		obj("cwm.p1", "phase", "cwm.root"),
	} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}

	all, _ := svc.ObjectList(ctx, api.ObjectListInput{})
	if all.Total != 4 {
		t.Errorf("empty filter = %d, want 4", all.Total)
	}
	byType, _ := svc.ObjectList(ctx, api.ObjectListInput{Type: "feature"})
	if byType.Total != 2 {
		t.Errorf("type filter = %d, want 2", byType.Total)
	}
	byParent, _ := svc.ObjectList(ctx, api.ObjectListInput{Parent: "cwm.root"})
	if byParent.Total != 3 {
		t.Errorf("parent filter = %d, want 3", byParent.Total)
	}
	both, _ := svc.ObjectList(ctx, api.ObjectListInput{Type: "feature", Parent: "cwm.root"})
	if both.Total != 2 {
		t.Errorf("AND filter = %d, want 2", both.Total)
	}
	none, _ := svc.ObjectList(ctx, api.ObjectListInput{Module: "nope"})
	if none.Total != 0 {
		t.Errorf("unknown module = %d, want 0", none.Total)
	}
}

// TestObjectTreeNests pins that the tree comes back as nested DATA in one call —
// the G5 promise (no per-object round trip).
func TestObjectTreeNests(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()
	for _, o := range []api.Object{
		obj("cwm.root", "module", ""),
		obj("cwm.p1", "phase", "cwm.root"),
		obj("cwm.p1a", "feature", "cwm.p1"),
	} {
		_, _ = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o})
	}

	tr, err := svc.ObjectTree(ctx, api.ObjectTreeInput{Root: "cwm.root"})
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if len(tr.Roots) != 1 || tr.Roots[0].Object.ID != "cwm.root" {
		t.Fatalf("roots = %+v", tr.Roots)
	}
	p1 := tr.Roots[0].Children
	if len(p1) != 1 || p1[0].Object.ID != "cwm.p1" {
		t.Fatalf("children of root = %+v", p1)
	}
	if len(p1[0].Children) != 1 || p1[0].Children[0].Object.ID != "cwm.p1a" {
		t.Fatalf("grandchildren = %+v", p1[0].Children)
	}
	if tr.Total != 3 {
		t.Errorf("total = %d, want 3", tr.Total)
	}

	// Depth limits the descent.
	shallow, _ := svc.ObjectTree(ctx, api.ObjectTreeInput{Root: "cwm.root", Depth: 1})
	if len(shallow.Roots[0].Children) != 1 || len(shallow.Roots[0].Children[0].Children) != 0 {
		t.Errorf("depth=1 should stop after one level: %+v", shallow.Roots[0].Children)
	}

	if _, err := svc.ObjectTree(ctx, api.ObjectTreeInput{Root: "cwm.ghost"}); err == nil {
		t.Error("tree of a missing root should error, not return empty")
	}
}

// TestObjectTreeCycleTerminates pins the guard. Ids are AUTHORED, so a parent
// cycle is a real authoring mistake — it must not hang the reader.
func TestObjectTreeCycleTerminates(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()
	// a -> b -> a
	_, _ = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.a", "x", "cwm.b")})
	_, _ = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.b", "x", "cwm.a")})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.ObjectTree(ctx, api.ObjectTreeInput{Root: "cwm.a"})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ObjectTree did not terminate on a parent cycle")
	}
}

// TestObjectScanDetectsChange pins the G4 basis: added / modified / removed,
// keyed by the file's content hash.
func TestObjectScanDetectsChange(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	// First scan over an empty tree, then add a file behind the service's back —
	// that is the real case: a human edits a file in the repo.
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	dir := filepath.Join(root, "modules", "cwm", "objects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hand.cwm.yaml")
	if err := os.WriteFile(path, []byte("id: cwm.hand\ntype: feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ObjectScan(ctx, api.ObjectScanInput{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got.Added) != 1 || got.Added[0] != "cwm.hand" {
		t.Fatalf("added = %v, want [cwm.hand]", got.Added)
	}

	// An unchanged rescan reports nothing — the hash is the basis.
	if again, _ := svc.ObjectScan(ctx, api.ObjectScanInput{}); len(again.Added)+len(again.Modified)+len(again.Removed) != 0 {
		t.Errorf("unchanged rescan should be quiet, got %+v", again)
	}

	// Edit it → modified.
	if err := os.WriteFile(path, []byte("id: cwm.hand\ntype: phase\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mod, _ := svc.ObjectScan(ctx, api.ObjectScanInput{})
	if len(mod.Modified) != 1 || mod.Modified[0] != "cwm.hand" {
		t.Errorf("modified = %v, want [cwm.hand]", mod.Modified)
	}

	// Delete it → removed.
	_ = os.Remove(path)
	rm, _ := svc.ObjectScan(ctx, api.ObjectScanInput{})
	if len(rm.Removed) != 1 || rm.Removed[0] != "cwm.hand" {
		t.Errorf("removed = %v, want [cwm.hand]", rm.Removed)
	}
}

// TestObjectScanReportsCorruptFile pins that a bad file is REPORTED — neither
// skipped (it would read as "removed", a lie about the repo) nor fatal (one bad
// file must not make the project unopenable).
func TestObjectScanReportsCorruptFile(t *testing.T) {
	svc, root := newSvc(t)
	dir := filepath.Join(root, "modules", "cwm", "objects")
	_ = os.MkdirAll(dir, 0o755)
	// Valid YAML, but no id — the anonymous-object case.
	if err := os.WriteFile(filepath.Join(dir, "bad.cwm.yaml"), []byte("type: feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := stdctx.Background()
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
		t.Fatalf("one bad file made the scan fail: %v", err)
	}
	errs, err := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs.Unreadable) != 1 || !strings.Contains(errs.Unreadable[0].Path, "bad.cwm.yaml") ||
		!strings.HasPrefix(errs.Unreadable[0].Problems[0], api.DiagnosticUnreadable) {
		t.Fatalf("the bad file is not reported: %+v", errs.Unreadable)
	}
}

// TestObjectDeleteDropsFromIndex pins the other half of write-through.
func TestObjectDeleteDropsFromIndex(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()
	o := obj("cwm.tmp", "feature", "")
	_, _ = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o})
	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "cwm", ID: o.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	out, _ := svc.ObjectList(ctx, api.ObjectListInput{})
	if out.Total != 0 {
		t.Errorf("deleted object still in index: %+v", out.Objects)
	}
}

// TestObjectWriteEmitsLifecycleEvent is the anti-hang test.
//
// The inbox handshake suspends a task on `cwm:!workspace/object/<id>/updated` and
// is woken by that event and nothing else. It pins BOTH halves of the current
// truth: a subject-safe id emits, and a module-qualified authored id does NOT —
// which is exactly why the inbox is not migrated yet.
func TestObjectWriteEmitsLifecycleEvent(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()

	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)

	var mu sync.Mutex
	var got []string
	record := func(name string) func(string, any) {
		return func(string, any) {
			mu.Lock()
			got = append(got, name)
			mu.Unlock()
		}
	}

	// A MODULE-QUALIFIED authored id — the case that used to emit nothing.
	// The subscriber derives the same token from the same id; they never
	// exchange it, which is the whole contract.
	safe := obj("cwm.approval-1", "approval-request", "")
	tok := cwmevents.SubjectToken(safe.ID)
	_ = bus.Subscribe(cwmevents.ObjectCreatedSubject("t", "u", "w", tok), record("created"))
	_ = bus.Subscribe(cwmevents.ObjectUpdatedSubject("t", "u", "w", tok), record("updated"))

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: safe}); err != nil {
		t.Fatalf("create: %v", err)
	}
	safe.Capabilities["content"] = map[string]any{"status": "approved"}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: safe}); err != nil {
		t.Fatalf("update: %v", err)
	}

	mu.Lock()
	if len(got) != 2 || got[0] != "created" || got[1] != "updated" {
		t.Errorf("lifecycle events = %v, want [created updated]", got)
	}
	mu.Unlock()
}

// TestAuthoredIDReachesItsSubscriber is the anti-hang test, stated as the
// property that actually matters: an object with a MODULE-QUALIFIED authored id
// — the normal case — must reach a subscriber that only knows the id.
//
// Before the derived token this was impossible: the dot in `cwm.approval-1` is
// the NATS separator, the subject builder panicked, events.recoverPublish
// swallowed it, and the suspended task waited forever.
func TestAuthoredIDReachesItsSubscriber(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()

	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)

	woken := make(chan struct{}, 1)
	o := obj("compose.CMP-010", "approval-request", "")

	// The subscriber knows ONLY the id, exactly like inbox_ask.
	_ = bus.Subscribe(
		cwmevents.ObjectUpdatedSubject("t", "u", "w", cwmevents.SubjectToken("compose.CMP-010")),
		func(string, any) { woken <- struct{}{} },
	)

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "compose", Object: o}); err != nil {
		t.Fatalf("create: %v", err)
	}
	o.Capabilities["content"] = map[string]any{"status": "approved"}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "compose", Object: o}); err != nil {
		t.Fatalf("update: %v", err)
	}

	select {
	case <-woken:
	case <-time.After(2 * time.Second):
		t.Fatal("a write to an authored id did not reach its subscriber — a suspended task would wait forever")
	}
}

// TestObjectGetSurvivesFileRename pins the file header's own promise:
// "identity comes from the file CONTENT, never the filename". Get used to
// rebuild the path from the id, so a rename broke it while List and Tree kept
// working — the doctrine was true three lines above code that ignored it.
func TestObjectGetSurvivesFileRename(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.thing", "feature", "")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	dir := filepath.Join(root, "modules", "cwm", "objects")
	if err := os.Rename(filepath.Join(dir, "thing.cwm.yaml"), filepath.Join(dir, "totally-unrelated-name.cwm.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: "cwm.thing"})
	if err != nil {
		t.Fatalf("ObjectGet after rename: %v — the filename is a convention, not identity", err)
	}
	if got.Object.ID != "cwm.thing" {
		t.Errorf("got %q", got.Object.ID)
	}
}
