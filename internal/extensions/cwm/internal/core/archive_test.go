// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// AN ARCHIVED OBJECT HAS ENDED, NOT VANISHED. ObjectArchive renames its file,
// unchanged, into its module's `.ktw/_history/<yyyy-mm>/` and drops it from
// the index; every link to it stays valid (no dangling link, accepted on
// write, shown as ended), ObjectGet says where it went, and its id is never
// minted again (ended.go, methods_object_archive.go).

// archiveFixture: module m, requests and (minted, filed in
// .ktw/requests), a note m.n that serves; error mode.
func archiveFixture(t *testing.T) (*Service, *eventbus.MemoryEventBus, string) {
	t.Helper()
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: project, ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "module"},
		{Name: "note"},
		{Name: "request", IDPrefix: "RQ", Folder: ".ktw/requests"},
	}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{
		{Name: "serves", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	ctx := stdctx.Background()
	seedModule(t, svc, "m")
	for _, title := range []string{"First request", "Second request"} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{Type: "request", Name: title, Parent: "m"}}); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}
	n := api.Object{ID: "m.n", Type: "note", Name: "n", Parent: "m", Edges: []api.ObjectEdge{{Kind: "serves", To: "m.RQ-001"}}}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: n}); err != nil {
		t.Fatalf("create m.n: %v", err)
	}
	return svc, bus, project
}

func archive(t *testing.T, svc *Service, id string) api.ObjectArchiveOutput {
	t.Helper()
	out, err := svc.ObjectArchive(stdctx.Background(), api.ObjectArchiveInput{ID: id})
	if err != nil {
		t.Fatalf("ObjectArchive %s: %v", id, err)
	}
	return out
}

// 1. A link to an archived object is no dangling link.
func TestAnArchivedTargetIsNoDanglingLink(t *testing.T) {
	svc, _, _ := archiveFixture(t)
	archive(t, svc, "m.RQ-001")
	if errs := linkErrors(svc, "m.n"); len(errs) != 0 {
		t.Fatalf("m.n links to an archived object: %+v — want no dangling link", errs)
	}
	list, err := svc.ObjectErrorList(stdctx.Background(), api.ObjectErrorListInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range list.Objects {
		if o.ID == "m.n" {
			t.Errorf("ObjectErrorList lists m.n: %+v", o)
		}
	}
	// The same after a full rescan: the ended set is read back from disk.
	if _, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	if errs := linkErrors(svc, "m.n"); len(errs) != 0 {
		t.Errorf("after a rescan: %+v — want no dangling link", errs)
	}
}

// 2. A write naming an archived id is accepted; a missing id is still refused.
func TestAnEdgeToAnArchivedIDIsAcceptedOnWrite(t *testing.T) {
	svc, _, _ := archiveFixture(t)
	ctx := stdctx.Background()
	archive(t, svc, "m.RQ-001")
	if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "m", To: "m.RQ-001", Kind: "serves"}); err != nil {
		t.Errorf("EdgeCreate to an archived id: %v — want accepted", err)
	}
	o := api.Object{ID: "m.n2", Type: "note", Name: "n2", Parent: "m", Edges: []api.ObjectEdge{{Kind: "serves", To: "m.RQ-001"}}}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Errorf("ObjectCreate with an edge to an archived id: %v — want accepted", err)
	}
	if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "m", To: "m.ghost", Kind: "serves"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("EdgeCreate to a missing id: %v — want refused", err)
	}
}

// 3. The file moves, bytes unchanged, into .ktw/_history/<yyyy-mm>/ and leaves
// list, search and full-text.
func TestArchiveMovesTheFileUnchangedOutOfEveryRead(t *testing.T) {
	svc, _, project := archiveFixture(t)
	ctx := stdctx.Background()
	src, ok := svc.index().pathOf("m.RQ-001")
	if !ok {
		t.Fatal("m.RQ-001 not indexed")
	}
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	out := archive(t, svc, "m.RQ-001")
	month := time.Now().UTC().Format("2006-01")
	want := filepath.Join(project, "modules", "m", ".ktw", "_history", month, filepath.Base(src))
	after, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("archived file not at %s: %v (out %+v)", want, err, out)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the archived bytes changed")
	}
	if out.Path != filepath.ToSlash(filepath.Join("modules", "m", ".ktw", "_history", month, filepath.Base(src))) {
		t.Errorf("out.Path = %q", out.Path)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("the file is still at %s: %v", src, err)
	}
	list, _ := svc.ObjectList(ctx, api.ObjectListInput{Module: "m"})
	for _, o := range list.Objects {
		if o.ID == "m.RQ-001" {
			t.Errorf("ObjectList still has m.RQ-001")
		}
	}
	for _, mode := range []string{"substring", "fulltext"} {
		got, err := svc.ObjectSearch(ctx, api.ObjectSearchInput{Query: "First", Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range got.Objects {
			if o.ID == "m.RQ-001" {
				t.Errorf("ObjectSearch (%s) still finds m.RQ-001", mode)
			}
		}
	}
	// Its outbound edges left with it: nothing lists an edge from it.
	edges, _ := svc.EdgeList(ctx, api.EdgeListInput{})
	for _, e := range edges.Edges {
		if e.From == "m.RQ-001" {
			t.Errorf("an edge from the archived object is still listed: %+v", e)
		}
	}
	// Into names another folder of the history only.
	if _, err := svc.ObjectArchive(ctx, api.ObjectArchiveInput{ID: "m.RQ-002", Into: "elsewhere"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("Into outside .ktw/_history: %v — want refused", err)
	}
	got, err := svc.ObjectArchive(ctx, api.ObjectArchiveInput{ID: "m.RQ-002", Into: ".ktw/_history/old"})
	if err != nil || !strings.Contains(got.Path, "/.ktw/_history/old/") {
		t.Errorf("Into .ktw/_history/old: %+v, %v", got, err)
	}
}

// 4. Archiving twice: AlreadyArchived, one event.
func TestArchivingTwiceIsOneArchive(t *testing.T) {
	svc, bus, _ := archiveFixture(t)
	var c captured
	_ = bus.Subscribe(api.ObjectArchivedSubject("t", "u", "w", api.SubjectToken("m.RQ-001")), c.on)
	first := archive(t, svc, "m.RQ-001")
	second := archive(t, svc, "m.RQ-001")
	if first.AlreadyArchived || !second.AlreadyArchived || second.Path != first.Path {
		t.Errorf("first %+v, second %+v — want the second AlreadyArchived at the same path", first, second)
	}
	got := c.all()
	if len(got) != 1 {
		t.Fatalf("archived events = %d, want 1", len(got))
	}
	p, ok := got[0].(events.ObjectArchivedPayload)
	if !ok || p.ObjectID != "m.RQ-001" || p.Module != "m" || p.Type != "request" || p.Parent != "m" || p.Path != first.Path || p.EventID == "" || p.Revision == "" {
		t.Errorf("event = %+v", got[0])
	}
	if _, err := svc.ObjectArchive(stdctx.Background(), api.ObjectArchiveInput{ID: "m.never"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("archive of a missing id: %v — want not found", err)
	}
}

// 5. An object with children is refused, naming them.
func TestAnObjectWithChildrenIsNotArchived(t *testing.T) {
	svc, _, _ := archiveFixture(t)
	ctx := stdctx.Background()
	kid := api.Object{ID: "m.kid", Type: "note", Name: "kid", Parent: "m.RQ-001"}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: kid}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ObjectArchive(ctx, api.ObjectArchiveInput{ID: "m.RQ-001"})
	if !errors.Is(err, api.ErrReferenced) || !strings.Contains(err.Error(), "m.kid") {
		t.Fatalf("archive with a child: %v — want refused naming m.kid", err)
	}
	if _, ok := svc.index().get("m.RQ-001"); !ok {
		t.Errorf("the refused archive moved the object")
	}
}

// 6. The mint after archiving the highest id gives the next one, never it.
func TestTheMintNeverReusesAnArchivedID(t *testing.T) {
	svc, _, _ := archiveFixture(t)
	ctx := stdctx.Background()
	archive(t, svc, "m.RQ-002")
	got, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{Type: "request", Name: "Third", Parent: "m"}})
	if err != nil || got.Object.ID != "m.RQ-003" {
		t.Fatalf("minted %q, %v — want m.RQ-003", got.Object.ID, err)
	}
	// Read back from disk after a scan. A dated id is read from the file: its
	// name's prefix is not the id.
	archive(t, svc, "m.RQ-003")
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{Type: "request", Name: "Fourth", Parent: "m"}})
	if err != nil || got.Object.ID != "m.RQ-004" {
		t.Fatalf("after a rescan minted %q, %v — want m.RQ-004", got.Object.ID, err)
	}
	if id := endedIDOf("m", "", "RQ-007-a-slug.cwm.yaml"); id != "m.RQ-007" {
		t.Errorf("endedIDOf a slugged name = %q", id)
	}
	dated := filepath.Join(t.TempDir(), "TD-20261009-104526-8d3c.cwm.yaml")
	if err := os.WriteFile(dated, []byte("id: m.TD-20261009-104526-8d3c\ntype: note\nname: td\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if id := endedIDOf("m", dated, filepath.Base(dated)); id != "m.TD-20261009-104526-8d3c" {
		t.Errorf("endedIDOf a dated name = %q � want the file's id", id)
	}
}

// 7. A bare archived id keeps its module (qualifyTarget, no code of its own).
func TestABareArchivedIDKeepsItsModule(t *testing.T) {
	svc, _, _ := archiveFixture(t)
	archive(t, svc, "m.RQ-001")
	got, err := svc.EdgeCreate(stdctx.Background(), api.EdgeCreateInput{From: "m", To: "RQ-001", Kind: "serves"})
	if err != nil || got.Edge.To != "m.RQ-001" {
		t.Fatalf("EdgeCreate m -> RQ-001: %+v, %v — want m.RQ-001", got.Edge, err)
	}
}

// 8. EdgeList marks an edge to an archived object Ended, and only that one.
func TestAnEdgeToAnArchivedObjectIsEnded(t *testing.T) {
	svc, _, _ := archiveFixture(t)
	ctx := stdctx.Background()
	if err := withEdge(svc, t, "m.n", "serves", "m.RQ-002"); err != nil {
		t.Fatal(err)
	}
	archive(t, svc, "m.RQ-001")
	out, err := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: "m.n", Direction: api.DirectionOut})
	if err != nil {
		t.Fatal(err)
	}
	ended := map[string]bool{}
	for _, e := range out.Edges {
		ended[e.To] = e.Ended
	}
	if !ended["m.RQ-001"] || ended["m.RQ-002"] {
		t.Errorf("ended = %v — want m.RQ-001 ended, m.RQ-002 not", ended)
	}
}

// 9. ObjectGet on an archived id: cwm.objectArchived, a not-found, saying where.
func TestObjectGetOnAnArchivedIDSaysItEnded(t *testing.T) {
	svc, _, _ := archiveFixture(t)
	out := archive(t, svc, "m.RQ-001")
	for _, in := range []api.ObjectGetInput{{ID: "m.RQ-001"}, {Module: "m", ID: "m.RQ-001"}} {
		_, err := svc.ObjectGet(stdctx.Background(), in)
		if !errors.Is(err, api.ErrNotFound) || !usermsg.Is(err, "cwm.objectArchived") || !strings.Contains(err.Error(), out.Path) {
			t.Errorf("ObjectGet %+v: %v — want cwm.objectArchived (not found) naming %s", in, err, out.Path)
		}
	}
	if _, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{ID: "m.never"}); usermsg.Is(err, "cwm.objectArchived") || !errors.Is(err, api.ErrNotFound) {
		t.Errorf("ObjectGet of a missing id: %v — want a plain not found", err)
	}
}
