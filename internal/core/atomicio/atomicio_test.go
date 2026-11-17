// SPDX-License-Identifier: Apache-2.0

package atomicio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestWriteAtomicNoTornWrite hammers a single path with concurrent
// writers and a concurrent reader. The reader must always see either
// (a) the file missing or (b) a fully valid YAML doc — never a
// half-written tempfile or partial bytes.
//
// Each writer writes a different payload of varying size to maximize
// the chance of catching a torn write if the atomicity invariant
// regressed.
func TestWriteAtomicNoTornWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "target.yaml")

	// Enough concurrency to catch tearing, not so much that the workload stops
	// resembling anything.
	//
	// It was 100 writers, each also reading 5 times. On Windows a rename fails
	// while ANY handle is open on the destination, so those readers block the
	// writers: under load only 18 of 100 ever published, and the run proved
	// nothing about tearing because almost nothing was published to tear. That
	// is also not a workload the system produces — CWM holds a
	// single-writer-per-workspace lock, so concurrent writers to one path are a
	// stress case, while concurrent READERS are real.
	//
	// So: fewer writers, same readers each. The invariant under test is
	// unchanged and the run actually reaches it.
	const writers = 24
	const readsPerWriter = 5

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		wroteOK   int
		readOK    int
		contended int
	)
	wg.Add(writers)

	for i := 0; i < writers; i++ {
		i := i
		go func() {
			defer wg.Done()
			body := fmt.Sprintf("writer: %d\npayload: %s\n", i, strings.Repeat("x", i*64))
			// A write or read that could not complete is CONTENTION, not a torn
			// write, and this test's subject is tearing. 100 goroutines renaming
			// onto one path is a deliberate worst case; on Windows, where a
			// reader — or a virus scanner — holding the destination makes rename
			// fail, some legitimately lose. Counting those separately stops a
			// loaded machine reporting a correctness failure, while the floors
			// below stop the test passing on a run where almost nothing
			// happened.
			if err := WriteAtomic(path, []byte(body), 0o644); err != nil {
				mu.Lock()
				contended++
				mu.Unlock()
			} else {
				mu.Lock()
				wroteOK++
				mu.Unlock()
			}
			for r := 0; r < readsPerWriter; r++ {
				data, err := ReadAtomic(path)
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					mu.Lock()
					contended++
					mu.Unlock()
					continue
				}
				var v map[string]any
				if err := yaml.Unmarshal(data, &v); err != nil {
					// THE invariant: a read that SUCCEEDS must parse. Anything
					// else means a reader saw a half-published file.
					t.Errorf("torn write detected — writer %d read %d: %v\nbytes: %q",
						i, r, err, data)
					continue
				}
				mu.Lock()
				readOK++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Floors, so contention can never turn this into a test that checks
	// nothing: a run where every writer lost would otherwise report success
	// having observed no publish at all.
	if wroteOK < writers/2 {
		t.Errorf("only %d of %d writers published — too contended to have tested tearing", wroteOK, writers)
	}
	if readOK < writers*readsPerWriter/2 {
		t.Errorf("only %d of %d reads observed a document — too few to have tested tearing",
			readOK, writers*readsPerWriter)
	}
	if contended > 0 {
		t.Logf("contention: %d write/read attempts lost the race (expected on Windows under load)", contended)
	}

	// Final state must be readable and valid.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	var v map[string]any
	if err := yaml.Unmarshal(data, &v); err != nil {
		t.Fatalf("final parse: %v\nbytes: %q", err, data)
	}
	if _, ok := v["writer"]; !ok {
		t.Fatalf("final doc missing writer key: %v", v)
	}

	// No leftover tempfiles after the dust settles.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover tempfile: %s", e.Name())
		}
	}
}

// TestWriteAtomicCreatesParentDir confirms parent directories are
// created on demand — every higher layer relies on this.
func TestWriteAtomicCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.yaml")
	if err := WriteAtomic(path, []byte("k: v\n"), 0o644); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	}
}

// TestReadAtomicMissingFileReturnsErrNotExist confirms the
// fast-path for absent files — callers commonly use
// errors.Is(err, os.ErrNotExist) for control flow and shouldn't pay
// the retry cost.
func TestReadAtomicMissingFileReturnsErrNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadAtomic(filepath.Join(dir, "absent.yaml"))
	if !os.IsNotExist(err) {
		t.Errorf("ReadAtomic on absent: err = %v, want os.IsNotExist", err)
	}
}
