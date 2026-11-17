// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// A REACTION'S CHANGE SET: an atomic reaction's writes are staged
// while its set is open, written together at the commit or not at all. The
// runner here is a fake: it opens the set, runs the action under the
// reaction's cause, and commits — or discards when the action fails.

const atomicReaction = "type:watched#on_create#x:react/create@e-1"

func reactionCtx(ctx stdctx.Context, reaction string) stdctx.Context {
	return cause.WithCause(ctx, cause.Cause{Kind: cause.KindReaction, ID: reaction, Depth: 1})
}

// runAtomic is the events runner's part, faked: begin, the action, commit (or
// discard on the action's failure).
func runAtomic(t *testing.T, svc *Service, ctx stdctx.Context, reaction string, action func(rctx stdctx.Context) error) (api.ChangeSetCommitOutput, error) {
	t.Helper()
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: reaction}); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := action(reactionCtx(ctx, reaction)); err != nil {
		if _, derr := svc.ChangeSetDiscard(ctx, api.ChangeSetDiscardInput{Reaction: reaction}); derr != nil {
			t.Fatalf("discard: %v", derr)
		}
		return api.ChangeSetCommitOutput{}, err
	}
	return svc.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Reaction: reaction})
}

// objectEvents counts the created and updated events of ids on bus.
type objectEvents struct{ created, updated captured }

func watchObjects(bus *eventbus.MemoryEventBus, ids ...string) *objectEvents {
	w := &objectEvents{}
	for _, id := range ids {
		_ = bus.Subscribe(api.ObjectCreatedSubject("t", "u", "w", api.SubjectToken(id)), w.created.on)
		_ = bus.Subscribe(api.ObjectUpdatedSubject("t", "u", "w", api.SubjectToken(id)), w.updated.on)
	}
	return w
}

func (w *objectEvents) count() int { return len(w.created.all()) + len(w.updated.all()) }

func fileOf(t *testing.T, cfg *CWMServiceConfig, id string) string {
	t.Helper()
	return filepath.Join(cfg.ProjectRoot, "modules", "billing", "objects", strings.TrimPrefix(id, "billing.")+ObjectFileSuffix)
}

func onDisk(t *testing.T, cfg *CWMServiceConfig, id string) (api.Object, bool) {
	t.Helper()
	obj, err := ReadObjectFile(fileOf(t, cfg, id))
	if err != nil {
		return api.Object{}, false
	}
	return obj, true
}

// changeSetFixture: billing.b exists (written with no cause); the `chained`
// type's async entries accept caused writes, so a reaction's writes stage
// matches in its outbox.
func changeSetFixture(t *testing.T) (*Service, stdctx.Context, *eventbus.MemoryEventBus, *CWMServiceConfig) {
	t.Helper()
	cfg := asyncConfig(t)
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{Name: "about", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"}}
	svc, ctx, bus := openAsync(t, cfg, newDurableActions())
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.b", "chained")}); err != nil {
		t.Fatal(err)
	}
	return svc, ctx, bus, cfg
}

// Two objects staged — a create and an update — then discarded: neither is
// written, no event goes out, nothing is staged in the outbox.
func TestAChangeSetDiscardedWritesNothing(t *testing.T) {
	svc, ctx, bus, cfg := changeSetFixture(t)
	ev := watchObjects(bus, "billing.a", "billing.b")
	_, err := runAtomic(t, svc, ctx, atomicReaction, func(rctx stdctx.Context) error {
		if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.a", "chained")}); err != nil {
			t.Fatalf("stage create: %v", err)
		}
		b := typed("billing.b", "chained")
		b.Description = "changed by the reaction"
		if _, err := svc.ObjectUpdate(rctx, api.ObjectUpdateInput{Module: "billing", Object: b}); err != nil {
			t.Fatalf("stage update: %v", err)
		}
		return errors.New("the action failed")
	})
	if err == nil {
		t.Fatal("the failed action was committed")
	}
	if _, ok := onDisk(t, cfg, "billing.a"); ok {
		t.Error("a discarded create was written")
	}
	if b, _ := onDisk(t, cfg, "billing.b"); b.Description != "" {
		t.Errorf("a discarded update was written: %q", b.Description)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.a"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("get of a discarded create = %v, want not found", err)
	}
	time.Sleep(20 * time.Millisecond)
	if n := ev.count(); n != 0 {
		t.Errorf("%d object event(s) went out for a discarded set", n)
	}
	if boxes := svc.outbox().list(); len(boxes) != 0 {
		t.Errorf("outbox = %+v, want nothing staged by a discarded set", boxes)
	}
	if sets, _ := svc.ChangeSetList(ctx, api.ChangeSetListInput{}); len(sets.Sets) != 0 {
		t.Errorf("sets after the discard = %+v", sets.Sets)
	}
}

// The commit writes both; the events go out and the outbox is staged only
// then; the outbox is not released while the set is open.
func TestAChangeSetCommitWritesEveryObjectThenSendsTheirEvents(t *testing.T) {
	svc, ctx, bus, cfg := changeSetFixture(t)
	ev := watchObjects(bus, "billing.a", "billing.b")
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: atomicReaction}); err != nil {
		t.Fatal(err)
	}
	rctx := reactionCtx(ctx, atomicReaction)
	if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.a", "chained")}); err != nil {
		t.Fatal(err)
	}
	b := typed("billing.b", "chained")
	b.Description = "changed by the reaction"
	if _, err := svc.ObjectUpdate(rctx, api.ObjectUpdateInput{Module: "billing", Object: b}); err != nil {
		t.Fatal(err)
	}
	if _, ok := onDisk(t, cfg, "billing.a"); ok {
		t.Fatal("a staged create is on disk before the commit")
	}
	if got, _ := onDisk(t, cfg, "billing.b"); got.Description != "" {
		t.Fatal("a staged update is on disk before the commit")
	}
	if _, err := svc.OutboxRelease(ctx, api.OutboxReleaseInput{Reaction: atomicReaction}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("release with the set open = %v, want refused", err)
	}
	time.Sleep(20 * time.Millisecond)
	if n := ev.count(); n != 0 || len(svc.outbox().list()) != 0 {
		t.Fatalf("before the commit: %d event(s), outbox %+v — want none", n, svc.outbox().list())
	}
	sets, _ := svc.ChangeSetList(ctx, api.ChangeSetListInput{})
	if len(sets.Sets) != 1 || len(sets.Sets[0].Writes) != 2 {
		t.Fatalf("open sets = %+v, want one holding two writes", sets.Sets)
	}

	out, err := svc.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Reaction: atomicReaction})
	if err != nil || len(out.Writes) != 2 {
		t.Fatalf("commit = %+v, %v", out, err)
	}
	if _, ok := onDisk(t, cfg, "billing.a"); !ok {
		t.Error("the committed create is not on disk")
	}
	if got, _ := onDisk(t, cfg, "billing.b"); got.Description != "changed by the reaction" {
		t.Errorf("the committed update is not on disk: %q", got.Description)
	}
	if got, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.a"}); err != nil || got.Revision != out.Writes[0].Revision {
		t.Errorf("get after the commit = %v (revision %s), want the committed revision %s", err, got.Revision, out.Writes[0].Revision)
	}
	if len(ev.created.all()) != 1 || len(ev.updated.all()) != 1 {
		t.Errorf("events after the commit: %d created, %d updated — want one each", len(ev.created.all()), len(ev.updated.all()))
	}
	boxes := svc.outbox().list()
	if len(boxes) != 1 || boxes[0].Reaction != atomicReaction || boxes[0].Writes != 2 || boxes[0].Matches != 2 {
		t.Fatalf("outbox after the commit = %+v, want the reaction's two writes and their matches", boxes)
	}
	if _, err := svc.OutboxRelease(ctx, api.OutboxReleaseInput{Reaction: atomicReaction}); err != nil {
		t.Errorf("release after the commit: %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Join(storage.NewPaths(cfg.WorkspaceRoot).CWMDataDir(), "changesets")); len(ents) != 0 {
		t.Errorf("%d change-set folder(s) left after the commit", len(ents))
	}
}

// B changed by someone else after the reaction staged it: the commit is
// ErrConflict and A is not written either; a rerun on the current state
// commits. A revision already stale when it is staged conflicts at once.
func TestACommitOverAnObjectChangedMeanwhileWritesNothing(t *testing.T) {
	svc, ctx, _, cfg := changeSetFixture(t)
	read, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.b"})
	if err != nil {
		t.Fatal(err)
	}
	attempt := func(rctx stdctx.Context, rev string) error {
		if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.a", "chained")}); err != nil {
			return err
		}
		b := read.Object
		b.Description = "the reaction's"
		_, err := svc.ObjectUpdate(rctx, api.ObjectUpdateInput{Module: "billing", Object: b, Revision: rev})
		return err
	}
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: atomicReaction}); err != nil {
		t.Fatal(err)
	}
	if err := attempt(reactionCtx(ctx, atomicReaction), read.Revision); err != nil {
		t.Fatal(err)
	}
	// A person edits B meanwhile.
	edited := read.Object
	edited.Description = "the person's"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: edited}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Reaction: atomicReaction}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("commit over a moved revision = %v, want ErrConflict", err)
	}
	if _, ok := onDisk(t, cfg, "billing.a"); ok {
		t.Error("A was written by a commit that conflicted on B")
	}
	if got, _ := onDisk(t, cfg, "billing.b"); got.Description != "the person's" {
		t.Errorf("B = %q, want the person's write kept", got.Description)
	}
	if sets, _ := svc.ChangeSetList(ctx, api.ChangeSetListInput{}); len(sets.Sets) != 0 {
		t.Errorf("a conflicting commit left its set: %+v", sets.Sets)
	}
	// The rerun, on the current state: a revision stale at staging conflicts
	// at once; the current one commits.
	_, err = runAtomic(t, svc, ctx, atomicReaction, func(rctx stdctx.Context) error { return attempt(rctx, read.Revision) })
	if !errors.Is(err, api.ErrConflict) {
		t.Fatalf("staging over a stale revision = %v, want ErrConflict at once", err)
	}
	if read, err = svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runAtomic(t, svc, ctx, atomicReaction, func(rctx stdctx.Context) error { return attempt(rctx, read.Revision) }); err != nil {
		t.Fatalf("the rerun on the current state: %v", err)
	}
	if _, ok := onDisk(t, cfg, "billing.a"); !ok {
		t.Error("the rerun did not write A")
	}
}

// A process that stops after the redo log was written: the next start
// finishes the commit — both objects written, the index knows them, what they
// trigger staged in the reaction's outbox.
func TestACommitStoppedAfterItsRedoLogIsFinishedAtTheNextStart(t *testing.T) {
	cfg := asyncConfig(t)
	svc1, ctx1, _ := openAsync(t, cfg, newDurableActions())
	if _, err := svc1.ObjectCreate(ctx1, api.ObjectCreateInput{Module: "billing", Object: typed("billing.b", "chained")}); err != nil {
		t.Fatal(err)
	}
	svc1.changeSetStopAfterLog = func() bool { return true }
	_, err := runAtomic(t, svc1, ctx1, atomicReaction, func(rctx stdctx.Context) error {
		if _, err := svc1.ObjectCreate(rctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.a", "chained")}); err != nil {
			return err
		}
		b := typed("billing.b", "chained")
		b.Description = "the reaction's"
		_, err := svc1.ObjectUpdate(rctx, api.ObjectUpdateInput{Module: "billing", Object: b})
		return err
	})
	if err == nil {
		t.Fatal("the stopped commit reported success")
	}
	if _, ok := onDisk(t, cfg, "billing.a"); ok {
		t.Fatal("the stop came after the apply: the test proves nothing")
	}
	_ = svc1.Close()

	svc2, ctx2, bus := openAsync(t, cfg, newDurableActions())
	defer svc2.Close()
	ev := watchObjects(bus, "billing.a", "billing.b")
	svc2.RunBootstrapHooks(ctx2, BootstrapOwn) // the start
	if _, ok := onDisk(t, cfg, "billing.a"); !ok {
		t.Error("the next start did not write A")
	}
	if got, _ := onDisk(t, cfg, "billing.b"); got.Description != "the reaction's" {
		t.Errorf("the next start did not write B: %q", got.Description)
	}
	if _, err := svc2.ObjectGet(ctx2, api.ObjectGetInput{ID: "billing.a"}); err != nil {
		t.Errorf("the index does not know the finished create: %v", err)
	}
	if n := ev.count(); n != 2 {
		t.Errorf("%d event(s) at the finish, want 2", n)
	}
	boxes := svc2.outbox().list()
	if len(boxes) != 1 || boxes[0].Reaction != atomicReaction || boxes[0].Writes != 2 {
		t.Errorf("outbox after the finish = %+v, want the reaction's two writes staged", boxes)
	}
	if ents, _ := os.ReadDir(filepath.Join(storage.NewPaths(cfg.WorkspaceRoot).CWMDataDir(), "changesets")); len(ents) != 0 {
		t.Errorf("%d change-set folder(s) left after the finish", len(ents))
	}
}

// The reaction reads its own staged writes: Get, an update on the staged
// revision, an edge to a staged create; nobody else sees them.
func TestAReactionReadsItsOwnStagedWrites(t *testing.T) {
	svc, ctx, _, cfg := changeSetFixture(t)
	// The precondition the edge check below relies on: a link to an object
	// that does not exist is refused.
	dangling := typed("billing.x", "chained")
	dangling.Edges = []api.ObjectEdge{{Kind: "about", To: "billing.nowhere"}}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: dangling}); err == nil {
		t.Fatal("a dangling link is accepted: the edge check below proves nothing")
	}
	_, err := runAtomic(t, svc, ctx, atomicReaction, func(rctx stdctx.Context) error {
		created, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.a", "chained")})
		if err != nil {
			return err
		}
		got, err := svc.ObjectGet(rctx, api.ObjectGetInput{ID: "billing.a"})
		if err != nil || got.Revision != created.Revision {
			t.Errorf("the reaction's get of its staged create = %v (revision %s, want %s)", err, got.Revision, created.Revision)
		}
		if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.a"}); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("another reader sees a staged create: %v", err)
		}
		a := got.Object
		a.Description = "again"
		upd, err := svc.ObjectUpdate(rctx, api.ObjectUpdateInput{Module: "billing", Object: a, Revision: created.Revision})
		if err != nil {
			t.Fatalf("an update on the staged revision: %v", err)
		}
		if _, err := svc.ObjectUpdate(rctx, api.ObjectUpdateInput{Module: "billing", Object: a, Revision: created.Revision}); !errors.Is(err, api.ErrConflict) {
			t.Errorf("an update on a staged revision since replaced = %v, want ErrConflict", err)
		}
		_ = upd
		linker := typed("billing.c", "chained")
		linker.Edges = []api.ObjectEdge{{Kind: "about", To: "billing.a"}}
		if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "billing", Object: linker}); err != nil {
			t.Errorf("an edge to a staged create: %v", err)
		}
		if _, err := svc.EdgeCreate(rctx, api.EdgeCreateInput{From: "billing.a", To: "billing.b", Kind: "about"}); err != nil {
			t.Errorf("an edge from a staged create: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	a, ok := onDisk(t, cfg, "billing.a")
	if !ok || a.Description != "again" || len(a.Edges) != 1 {
		t.Errorf("committed A = %+v (%v), want its last staged state with its edge", a, ok)
	}
	if _, ok := onDisk(t, cfg, "billing.c"); !ok {
		t.Error("the linking create was not committed")
	}
}

// A write with no cause, and a reaction with no open set, are written at once
// as they always were — even while another reaction's set is open; a write
// with no cause never opens the change-set store.
func TestWritesOutsideAnOpenSetAreWrittenAtOnce(t *testing.T) {
	svc, ctx, _, cfg := changeSetFixture(t)
	if svc.changeSetStore != nil {
		t.Fatal("writes with no cause opened the change-set store")
	}
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: atomicReaction}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.u", "chained")}); err != nil {
		t.Fatal(err)
	}
	other := "type:watched#on_create#x:react/create@e-2" // not atomic: no set
	if _, err := svc.ObjectCreate(reactionCtx(ctx, other), api.ObjectCreateInput{Module: "billing", Object: typed("billing.r", "chained")}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"billing.u", "billing.r"} {
		if _, ok := onDisk(t, cfg, id); !ok {
			t.Errorf("%s was not written at once", id)
		}
	}
	sets, _ := svc.ChangeSetList(ctx, api.ChangeSetListInput{})
	if len(sets.Sets) != 1 || len(sets.Sets[0].Writes) != 0 {
		t.Errorf("open sets = %+v, want the atomic reaction's, empty", sets.Sets)
	}
	if boxes := svc.outbox().list(); len(boxes) != 1 || boxes[0].Reaction != other {
		t.Errorf("outbox = %+v, want the non-atomic reaction's write staged as before", boxes)
	}
}

// What a change set cannot stage is refused inside one.
func TestVerbsAChangeSetCannotStageAreRefused(t *testing.T) {
	svc, ctx, _, _ := changeSetFixture(t)
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: atomicReaction}); err != nil {
		t.Fatal(err)
	}
	rctx := reactionCtx(ctx, atomicReaction)
	calls := map[string]error{}
	_, calls["move"] = svc.ObjectMove(rctx, api.ObjectMoveInput{ID: "billing.b", NewLocalID: "bb", Reason: "rename"})
	_, calls["split"] = svc.ObjectSplit(rctx, api.ObjectSplitInput{ID: "billing.b"})
	_, calls["file write"] = svc.ObjectFileWrite(rctx, api.ObjectFileWriteInput{ID: "billing.b", Path: "x.txt", Content: []byte("x")})
	_, calls["file delete"] = svc.ObjectFileDelete(rctx, api.ObjectFileDeleteInput{ID: "billing.b", Path: "x.txt"})
	_, calls["file attach"] = svc.ObjectFileAttach(rctx, api.ObjectFileAttachInput{ID: "billing.b"})
	_, calls["named file write"] = svc.ObjectNamedFileWrite(rctx, api.ObjectNamedFileWriteInput{ID: "billing.b"})
	_, calls["named file declare"] = svc.ObjectNamedFileDeclare(rctx, api.ObjectNamedFileDeclareInput{ID: "billing.b"})
	_, calls["delete with detach"] = svc.ObjectDelete(rctx, api.ObjectDeleteInput{ID: "billing.b", Detach: true})
	for verb, err := range calls {
		if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "not inside a change set") {
			t.Errorf("%s inside a change set = %v, want refused", verb, err)
		}
	}
	// A delete without detach is staged.
	if _, err := svc.ObjectDelete(rctx, api.ObjectDeleteInput{ID: "billing.b"}); err != nil {
		t.Fatalf("a staged delete: %v", err)
	}
	if _, err := svc.ObjectGet(rctx, api.ObjectGetInput{ID: "billing.b"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("the reaction still reads its staged delete's object: %v", err)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.b"}); err != nil {
		t.Errorf("a staged delete is visible to others: %v", err)
	}
	if _, err := svc.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Reaction: atomicReaction}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.b"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("the committed delete left the object: %v", err)
	}
}

// A transition inside a set is staged with its event: the state and the
// transitioned event land at the commit, and the commit does not refuse the
// state change it carries.
func TestATransitionInsideAChangeSetLandsAtTheCommit(t *testing.T) {
	svc, ctx, bus := transitionFixture(t, nil)
	var moved captured
	_ = bus.Subscribe(api.ObjectTransitionedSubject("t", "u", "w", api.SubjectToken("billing.g1")), moved.on)
	if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: atomicReaction}); err != nil {
		t.Fatal(err)
	}
	if _, err := move(t, svc, reactionCtx(ctx, atomicReaction), "proposed", agent); err != nil {
		t.Fatalf("a staged transition: %v", err)
	}
	stored, _ := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.g1"})
	if st := lifecycleState(stored.Object); st == "proposed" || len(moved.all()) != 0 {
		t.Fatalf("before the commit: state %q, %d transitioned event(s)", st, len(moved.all()))
	}
	if _, err := svc.ChangeSetCommit(ctx, api.ChangeSetCommitInput{Reaction: atomicReaction}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	stored, _ = svc.ObjectGet(ctx, api.ObjectGetInput{ID: "billing.g1"})
	if st := lifecycleState(stored.Object); st != "proposed" || len(moved.all()) != 1 {
		t.Errorf("after the commit: state %q, %d transitioned event(s) — want proposed and one", st, len(moved.all()))
	}
	_ = events.New
}

func lifecycleState(o api.Object) string {
	life, _ := o.Lifecycle()
	return life.State
}
