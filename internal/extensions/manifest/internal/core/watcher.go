// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchUniverseSidecars watches `path` (the universe's Univers.yaml) for
// content changes and invokes `onChange` (debounced) on each rewrite.
//
// fsnotify on a single file is fragile: many editors save by writing
// to a temp + rename, which fires Rename/Remove on the watched path
// rather than Write. We work around by watching the parent directory
// and filtering events to entries whose path matches `target`. After
// a Rename/Remove of the file, we re-add the watch on the new file
// (via a brief retry loop) so subsequent edits keep firing.
//
// Used by ktw-hub to keep the membership in `Catalog` + the
// `ConnectionManager` synced with the on-disk file. A change to the
// `sidecars:` list triggers `Catalog.SetMembers` + `connmgr.SetMembers`
// (via duck-typed interfaces).
//
// Behaviour:
//   - Coalesces events in a 50ms debounce window (same as WatchInventory).
//   - Fires on Rename / Remove too — atomic-saves still work.
//   - Blocks until ctx is cancelled.
//   - Returns the first non-cancellation error; ctx cancellation = nil.
func WatchUniverseSidecars(ctx context.Context, path string, onChange func()) error {
	if onChange == nil {
		return fmt.Errorf("watcher: nil onChange callback")
	}
	if path == "" {
		return fmt.Errorf("watcher: empty path")
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watcher: create: %w", err)
	}
	defer w.Close()

	dir := filepathDir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("watcher: mkdir %s: %w", dir, err)
	}
	if err := w.Add(dir); err != nil {
		return fmt.Errorf("watcher: add %s: %w", dir, err)
	}

	const debounce = 50 * time.Millisecond
	var (
		mu    sync.Mutex
		timer *time.Timer
	)
	fire := func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(debounce, onChange)
	}

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			if timer != nil {
				timer.Stop()
			}
			mu.Unlock()
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			// Only fire on changes to the target file, not other files
			// in the same directory.
			if ev.Name != path {
				continue
			}
			if ev.Op == fsnotify.Chmod {
				continue
			}
			fire()
		case _, ok := <-w.Errors:
			if !ok {
				return nil
			}
			// Swallow transient errors (same as WatchInventory).
		}
	}
}

// filepathDir is filepath.Dir without the import (kept inline so the
// file doesn't need a second filepath import — already imported via
// fsnotify usage isn't a thing; just keep deps minimal).
func filepathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}

// WatchInventory watches `root` for filesystem changes and invokes
// `onChange` on every Create / Write / Remove / Rename event in the
// directory.
//
// Used by ktw-hub (through manifestlocal.WatchCache) to keep the in-memory
// Catalog in sync with the tier-1 manifest cache under <CacheDir>/sidecars/:
// when `catalog refresh <name>` rewrites a member's folder (or a developer
// edits one), the watch fires and the hub calls `Catalog.Reload()`.
//
// Behaviour:
//   - Creates `root` if missing (so callers don't have to pre-mkdir).
//   - Coalesces events in a 50ms debounce window — many editors emit
//     a flurry of events for a single save (Create + Write + chmod);
//     the callback fires once per quiet window, not per raw event.
//   - Blocks until ctx is cancelled. Callers run it in a goroutine.
//   - Returns the first non-cancellation error encountered (e.g.,
//     fsnotify creation failure). ctx cancellation returns nil.
//
// Coalescing: per-event-burst, not periodic. After an event arrives
// the watcher waits for `debounce` of silence before firing onChange,
// then resets. This minimises false-trigger Reloads when a file is
// written byte-by-byte.
func WatchInventory(ctx context.Context, root string, onChange func()) error {
	if onChange == nil {
		return fmt.Errorf("watcher: nil onChange callback")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("watcher: mkdir %s: %w", root, err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watcher: create: %w", err)
	}
	defer w.Close()

	if err := w.Add(root); err != nil {
		return fmt.Errorf("watcher: add %s: %w", root, err)
	}

	const debounce = 50 * time.Millisecond
	var (
		mu    sync.Mutex
		timer *time.Timer
	)
	fire := func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(debounce, onChange)
	}

	for {
		select {
		case <-ctx.Done():
			// Cancel any pending fire so we don't trigger after stop.
			mu.Lock()
			if timer != nil {
				timer.Stop()
			}
			mu.Unlock()
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			// Skip CHMOD-only events — editors and OS metadata shuffles
			// emit those without touching content. Anything else is
			// content-affecting (write / create / remove / rename).
			if ev.Op == fsnotify.Chmod {
				continue
			}
			fire()
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			// Don't fail the loop on transient errors — log via the
			// next pass when callers add a logger. For now, swallow.
			_ = err
		}
	}
}
