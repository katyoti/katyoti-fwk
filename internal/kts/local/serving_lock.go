// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
)

// ONE SERVING PROCESS — a provider serves a universe once: `ktsp nats` takes
// the universe's serving lock for its provider before it subscribes, and holds
// it for its life. The lock is the operating system's (oslock): held while the
// process lives — asleep or not —, released when it dies. A second process for
// the same provider and universe waits a while (a predecessor draining), then
// refuses to serve: two would split the provider's requests (its slots are
// queue groups) and restore each other's runs.

// servingLockWait is how long a starting process waits for its predecessor to
// let the lock go.
var servingLockWait = 30 * time.Second

// servingLockCheck is how often a serving process checks its lock still holds
// its path (see watchServingLock).
var servingLockCheck = 30 * time.Second

// servingLockPath is where provider's serving lock for the universe whose data
// folder is dataDir lives.
func servingLockPath(dataDir, provider string) string {
	return filepath.Join(dataDir, "serving", provider+".lock")
}

// acquireServingLock takes provider's serving lock, waiting up to wait for its
// holder to let it go; refused, the error says which process serves.
func acquireServingLock(ctx context.Context, dataDir, provider string, wait time.Duration) (*oslock.Lock, error) {
	path := servingLockPath(dataDir, provider)
	deadline := time.Now().Add(wait)
	for {
		l, err := oslock.TryLock(path)
		if err == nil {
			_ = l.Note(fmt.Sprintf("pid %d", os.Getpid()))
			return l, nil
		}
		if !errors.Is(err, oslock.ErrLocked) {
			return nil, err
		}
		if time.Now().After(deadline) {
			who := oslock.Note(path)
			if who == "" {
				who = "another process"
			}
			return nil, fmt.Errorf("%s already serves this universe (%s) — one serving process per universe: stop it first", provider, who)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// watchServingLock checks every `every` that lock still holds its path; when
// it no longer does (its file removed or replaced — a lock on a file no longer
// at its path guards nothing), lost is called once and the watch ends.
func watchServingLock(ctx context.Context, lock interface{ StillHeld() bool }, every time.Duration, lost func()) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !lock.StillHeld() {
				slog.Error("sc ktsp nats: the serving lock no longer holds its path — this process stops serving")
				lost()
				return
			}
		}
	}
}

// ServingProviders lists the providers a serving process holds the lock of
// in a universe's data folder, each with its lock's note ("pid N") — what
// `katyoti universe status` reports as the universe's serving members.
func ServingProviders(dataDir string) map[string]string {
	entries, err := os.ReadDir(filepath.Join(dataDir, "serving"))
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".lock") {
			continue
		}
		path := filepath.Join(dataDir, "serving", name)
		if held, _ := oslock.Held(path); held {
			out[strings.TrimSuffix(name, ".lock")] = oslock.Note(path)
		}
	}
	return out
}
