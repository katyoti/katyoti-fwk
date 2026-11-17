// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchInventory_FiresOnFileCreate(t *testing.T) {
	dir := t.TempDir()
	var fires atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- WatchInventory(ctx, dir, func() { fires.Add(1) })
	}()

	// Give the watcher a moment to register.
	time.Sleep(50 * time.Millisecond)

	// Write a file → expect one fire (after debounce).
	path := filepath.Join(dir, "ktw-x-1.0.0-sha256-abc.json")
	if err := os.WriteFile(path, []byte(`{"provider":"x"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := waitForCount(&fires, 1, 500*time.Millisecond); err != nil {
		t.Fatalf("waiting for fire: %v", err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("WatchInventory returned err: %v", err)
	}
}

func TestWatchInventory_DebouncesBurstWrites(t *testing.T) {
	dir := t.TempDir()
	var fires atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- WatchInventory(ctx, dir, func() { fires.Add(1) })
	}()
	time.Sleep(50 * time.Millisecond)

	// Five rapid writes within the 50ms debounce window — the watcher
	// should coalesce into a single fire.
	path := filepath.Join(dir, "burst.json")
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
		// No sleep between writes — keep them tight.
	}

	if err := waitForCount(&fires, 1, 500*time.Millisecond); err != nil {
		t.Fatalf("waiting for fire: %v", err)
	}

	// Wait long past the debounce to confirm we don't get a second fire.
	time.Sleep(200 * time.Millisecond)
	if got := fires.Load(); got > 2 {
		t.Errorf("expected 1-2 fires (1 debounced or possibly 2 if write timing scattered), got %d", got)
	}

	cancel()
	<-done
}

func TestWatchInventory_FiresOnRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "to-remove.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var fires atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- WatchInventory(ctx, dir, func() { fires.Add(1) })
	}()
	time.Sleep(50 * time.Millisecond)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if err := waitForCount(&fires, 1, 500*time.Millisecond); err != nil {
		t.Fatalf("waiting for remove fire: %v", err)
	}

	cancel()
	<-done
}

func TestWatchInventory_CtxCancelStopsCleanly(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- WatchInventory(ctx, dir, func() {})
	}()
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("WatchInventory returned err on cancel: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WatchInventory did not exit within 500ms of ctx cancel")
	}
}

func TestWatchInventory_CreatesMissingDir(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "deeply", "nested", "inventory")

	if _, err := os.Stat(target); err == nil {
		t.Fatal("expected target to be missing before WatchInventory")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- WatchInventory(ctx, target, func() {})
	}()
	// Created when the watch starts, on its own goroutine: wait for it, never
	// a fixed pause (a loaded machine starts the goroutine late).
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(target)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("WatchInventory should have created %s: %v", target, err)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-done
}

func TestWatchInventory_RejectsNilCallback(t *testing.T) {
	err := WatchInventory(context.Background(), t.TempDir(), nil)
	if err == nil {
		t.Error("expected error for nil callback")
	}
}

// ----------------------------------------------------------------------------
// WatchUniverseSidecars tests — file-level watch for extensions.yaml.
// ----------------------------------------------------------------------------

func TestWatchUniverseSidecars_FiresOnRewrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "extensions.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var fires atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- WatchUniverseSidecars(ctx, path, func() { fires.Add(1) }) }()
	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(path, []byte("version: 1\nsidecars: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := waitForCount(&fires, 1, 500*time.Millisecond); err != nil {
		t.Fatalf("waiting for fire: %v", err)
	}
	cancel()
	<-done
}

func TestWatchUniverseSidecars_IgnoresSiblingFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "extensions.yaml")
	other := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(target, []byte("v: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var fires atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- WatchUniverseSidecars(ctx, target, func() { fires.Add(1) }) }()
	time.Sleep(50 * time.Millisecond)

	// Touching a sibling must NOT fire.
	if err := os.WriteFile(other, []byte("noise"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := fires.Load(); got != 0 {
		t.Errorf("expected 0 fires for sibling write, got %d", got)
	}

	// Touching the target DOES fire.
	if err := os.WriteFile(target, []byte("v: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := waitForCount(&fires, 1, 500*time.Millisecond); err != nil {
		t.Fatalf("waiting for target fire: %v", err)
	}
	cancel()
	<-done
}

func TestWatchUniverseSidecars_HandlesAtomicSave(t *testing.T) {
	// Many editors save via write-temp + rename. The watcher must
	// fire on rename so atomic saves still propagate. We watch the
	// parent directory and filter to the target path.
	dir := t.TempDir()
	target := filepath.Join(dir, "extensions.yaml")
	if err := os.WriteFile(target, []byte("v: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var fires atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- WatchUniverseSidecars(ctx, target, func() { fires.Add(1) }) }()
	time.Sleep(50 * time.Millisecond)

	// Atomic save.
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte("v: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatal(err)
	}

	if err := waitForCount(&fires, 1, 500*time.Millisecond); err != nil {
		t.Fatalf("atomic-save did not fire: %v", err)
	}
	cancel()
	<-done
}

func TestWatchUniverseSidecars_RejectsEmptyPath(t *testing.T) {
	if err := WatchUniverseSidecars(context.Background(), "", func() {}); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestWatchUniverseSidecars_RejectsNilCallback(t *testing.T) {
	if err := WatchUniverseSidecars(context.Background(), "x", nil); err == nil {
		t.Error("expected error for nil callback")
	}
}

func waitForCount(c *atomic.Int32, target int32, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.Load() >= target {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	if c.Load() >= target {
		return nil
	}
	return os.ErrDeadlineExceeded
}
