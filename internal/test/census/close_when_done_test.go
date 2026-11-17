// SPDX-License-Identifier: Apache-2.0

package census

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
)

// heldLock is a service holding a lock in the isolated data folder, as the
// task run store holds its owner lock: Close gives it up.
type heldLock struct {
	lock   *oslock.Lock
	closed *bool
}

func (h *heldLock) Close() error {
	*h.closed = true
	return h.lock.Unlock()
}

// The census's application is closed when its test ends, before the isolated
// folders go: a service holding a file open in them (the run store's owner
// lock) gives it up, and the folder is removed — on Windows an open file
// cannot be, and the test failed in its TempDir cleanup.
func TestTheApplicationClosesBeforeItsFoldersAreRemoved(t *testing.T) {
	closed := false
	var data string
	t.Run("census", func(t *testing.T) {
		Isolate(t)
		data = os.Getenv("KTW_DATA")
		app := application.New(application.Config())
		lock, err := oslock.TryLock(filepath.Join(data, "runs", "owners", "census.lock"))
		if err != nil {
			t.Fatalf("lock: %v", err)
		}
		application.PutService[*heldLock](app, "default", &heldLock{lock: lock, closed: &closed})
		if err := app.Run(); err != nil {
			t.Fatalf("run: %v", err)
		}
		closeWhenDone(t, app)
	})
	if !closed {
		t.Error("the application was not closed when its test ended: its services keep what they hold")
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Errorf("the isolated data folder %s survives its test (stat: %v)", data, err)
	}
}
