// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a process that lost the store's write lease stops writing -
// its next write is refused, and nothing reaches the store.
func TestAStoreWhoseLeaseWasTakenRefusesTheNextWrite(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.a", "feature", "")}); err != nil {
		t.Fatalf("a write while holding the lease: %v", err)
	}
	// Another live process takes the lease over (this process's parent).
	rec, _ := json.Marshal(map[string]any{"pid": os.Getppid(), "started_at": time.Now().UTC(), "lease_ttl": "30s", "heartbeat_at": time.Now().UTC()})
	if err := os.WriteFile(filepath.Join(root, ".lock"), rec, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.b", "feature", "")})
	if !errors.Is(err, api.ErrLockHeldByOtherProcess) {
		t.Fatalf("a write after losing the lease = %v, want ErrLockHeldByOtherProcess", err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: "cwm.a"})
	upd := got.Object
	upd.Name = "changed"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: upd}); !errors.Is(err, api.ErrLockHeldByOtherProcess) {
		t.Fatalf("an update after losing the lease = %v", err)
	}
	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "cwm", ID: "cwm.a"}); !errors.Is(err, api.ErrLockHeldByOtherProcess) {
		t.Fatalf("a delete after losing the lease = %v", err)
	}
	if again, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: "cwm.a"}); err != nil || again.Object.Name == "changed" {
		t.Fatalf("the store was written after the lease was lost: %+v, %v", again.Object, err)
	}
}

// (AC-011): a store whose workspace lease was taken over at a
// higher epoch - another store on the same root, in this same process -
// refuses its next create, update and delete and writes nothing, while the
// taker writes. The PID alone would not fence it: both stores are this
// process.
func TestAStoreWhoseLeaseEpochMovedRefusesItsNextWrite(t *testing.T) {
	first, root := newSvc(t)
	ctx := stdctx.Background()
	if _, err := first.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.a", "feature", "")}); err != nil {
		t.Fatalf("a write while holding the lease: %v", err)
	}
	taker, err := New(ctx, &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("the second store taking the lease over: %v", err)
	}
	t.Cleanup(func() { _ = taker.Close() })

	if _, err := first.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.b", "feature", "")}); !errors.Is(err, api.ErrLockHeldByOtherProcess) {
		t.Fatalf("a create after the epoch moved = %v, want ErrLockHeldByOtherProcess", err)
	}
	got, err := first.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: "cwm.a"})
	if err != nil {
		t.Fatal(err)
	}
	upd := got.Object
	upd.Name = "changed"
	if _, err := first.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: upd}); !errors.Is(err, api.ErrLockHeldByOtherProcess) {
		t.Fatalf("an update after the epoch moved = %v", err)
	}
	if _, err := first.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "cwm", ID: "cwm.a"}); !errors.Is(err, api.ErrLockHeldByOtherProcess) {
		t.Fatalf("a delete after the epoch moved = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".lock")); err != nil {
		t.Fatalf("the taker's lease is gone: %v", err)
	}
	if _, err := taker.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.c", "feature", "")}); err != nil {
		t.Fatalf("the taker's write: %v", err)
	}
	if _, err := taker.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: "cwm.b"}); err == nil {
		t.Fatal("the store was written by the holder that lost the lease")
	}
	if again, err := taker.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: "cwm.a"}); err != nil || again.Object.Name == "changed" {
		t.Fatalf("the store was updated by the holder that lost the lease: %+v, %v", again.Object, err)
	}
}
