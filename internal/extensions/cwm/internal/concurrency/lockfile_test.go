// SPDX-License-Identifier: Apache-2.0

package concurrency

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The workspace lock is a process lease (core/proclease, tested there): a
// fresh live holder is refused with CWM's own error; a free workspace is
// taken and released.
func TestTheWorkspaceLockRefusesALiveHolderWithCWMsError(t *testing.T) {
	root := t.TempDir()
	rec := fmt.Sprintf(`{"pid":%d,"started_at":%q,"lease_ttl":"30s","heartbeat_at":%q}`,
		os.Getppid(), time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(root, lockfileBaseName), []byte(rec), 0o644); err != nil {
		t.Fatal(err)
	}
	err := NewProcessLockfile(root).Acquire(context.Background())
	if !errors.Is(err, api.ErrLockHeldByOtherProcess) {
		t.Fatalf("Acquire over a live fresh holder: %v, want ErrLockHeldByOtherProcess", err)
	}
	// The refusal names the holder: a member that cannot open the workspace
	// says which process keeps it.
	if want := fmt.Sprintf("pid %d", os.Getppid()); !strings.Contains(err.Error(), want) {
		t.Fatalf("the refusal %q does not name the holder (%s)", err, want)
	}

	free := NewProcessLockfile(t.TempDir())
	if err := free.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire of a free workspace: %v", err)
	}
	if !free.IsHeldBy() {
		t.Error("IsHeldBy = false after Acquire")
	}
	if err := free.Release(); err != nil {
		t.Fatal(err)
	}
}
