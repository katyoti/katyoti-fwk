// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// One process, several callers finding-or-spawning the same universe's hub at
// once (ktw-gui at a restart, its frontend activating the admin universe from
// several calls): ONE hub is started, the others attach to it.

// fakeSpawn stands in for exec'ing ktw-hub: it counts, takes a moment like a
// real start, then either "starts" a hub (an embedded NATS, announced by the
// lock file the way `ktw-hub serve` announces itself) or fails.
func fakeSpawn(t *testing.T, spawns *int32, fail error) {
	t.Helper()
	bus := startBus(t)
	prev := spawnHub
	spawnHub = func(ctx context.Context, opts ConnectOptions) (*HubClient, error) {
		atomic.AddInt32(spawns, 1)
		// Long enough that every caller started together joins the flight
		// before it ends, even on a loaded machine: a caller arriving after
		// it ended probes afresh, which is right but not what is tested.
		time.Sleep(time.Second)
		if fail != nil {
			return nil, fail
		}
		if err := WriteLockFile(opts.DataDir, LockFile{PID: os.Getpid(), NATSURL: bus.Server.ClientURL()}); err != nil {
			return nil, err
		}
		return connectReuse(ctx, opts)
	}
	t.Cleanup(func() { spawnHub = prev })
}

func findOrSpawnConcurrently(t *testing.T, dataDir string, n int) []error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	errs := make([]error, n)
	var wg, ready sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		ready.Add(1)
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start
			c, err := ConnectToHub(ctx, ConnectOptions{DataDir: dataDir, Mode: ModeFindOrSpawn})
			errs[i] = err
			if c != nil {
				_ = c.Close()
			}
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()
	return errs
}

func TestFindOrSpawn_ConcurrentCallersStartOneHub(t *testing.T) {
	var spawns int32
	fakeSpawn(t, &spawns, nil)
	errs := findOrSpawnConcurrently(t, t.TempDir(), 8)
	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&spawns); got != 1 {
		t.Fatalf("hubs started = %d, want 1", got)
	}
}

// A lead that fails (a hub that never came up, a cancelled start) is not
// retried by every caller that was waiting on it: each of them starting a hub
// of its own is how several hubs ended up running for one universe.
func TestFindOrSpawn_AFailedLeadIsNotRetriedByEveryWaiter(t *testing.T) {
	var spawns int32
	boom := errors.New("hub did not come up")
	fakeSpawn(t, &spawns, boom)
	errs := findOrSpawnConcurrently(t, t.TempDir(), 6)
	for i, err := range errs {
		if err == nil {
			t.Errorf("caller %d: no error, want the lead's failure", i)
		}
	}
	if got := atomic.LoadInt32(&spawns); got != 1 {
		t.Fatalf("hub starts = %d, want 1 (the waiters must not each start one)", got)
	}
}

func TestSpawnFlightKey_OneFolderOneFlight(t *testing.T) {
	d := t.TempDir()
	if spawnFlightKey(d) != spawnFlightKey(d+string(os.PathSeparator)+".") {
		t.Fatalf("two spellings of %s get two flights", d)
	}
}
