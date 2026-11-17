// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FSLoader is a Loader backed by a local filesystem directory.
// All paths are resolved relative to rootDir.
//
// # A directory is a mutable source, so the cache is stat-gated
//
// Reads are cached, but a cached entry is only used while the file on disk
// still matches the size and mtime it was read at. Without that gate the cache
// had no invalidation for a change made OUTSIDE this process: the loader's own
// WriteFile and DeleteFile evict, so a program editing its own files saw its
// edits, while an author editing a served folder did not. Adding a file worked
// (a new name misses the cache) and editing one did not — silent, partial, and
// the exact shape of failure that is hardest to find.
//
// The gate costs one stat per read instead of zero, and keeps the cache's value
// for the overwhelmingly common case of a file that has not changed.
//
// Bound: a change that preserves both size and mtime is not seen. That is the
// same bound every other staleness check here takes, and the same one the
// filesystem's own timestamp resolution imposes.
type FSLoader struct {
	baseLoader
	rootDir string

	stampMu sync.RWMutex
	stamps  map[string]fileStamp

	// immutable is set by MarkImmutable for a directory that cannot change
	// while the process runs — a sealed pack extracted from an executable.
	// Declared, never inferred: a directory is mutable until someone with the
	// knowledge to say otherwise says so.
	immutable bool
}

// MarkImmutable declares this directory's content fixed for the lifetime of the
// process, which lets reads be cached outright.
//
// For a SEALED source only — content extracted from an artifact that carries
// it, where changing the content means changing the artifact, and that shows up
// in the artifact's own stat. Marking a live folder immutable would serve stale
// content for exactly the source designed to be updated, and do it silently.
func (l *FSLoader) MarkImmutable() *FSLoader {
	l.immutable = true
	return l
}

// Immutable implements ImmutableSource. False unless declared: the safe answer
// is the one that re-reads.
func (l *FSLoader) Immutable() bool { return l.immutable }

// fileStamp is what a cached entry was read at.
type fileStamp struct {
	size    int64
	modNano int64
	missing bool // the file was absent when cached
}

// stampOf reads the current stamp for a relative path.
func (l *FSLoader) stampOf(name string) fileStamp {
	fi, err := os.Stat(filepath.Join(l.rootDir, filepath.FromSlash(name)))
	if err != nil {
		return fileStamp{missing: true}
	}
	return fileStamp{size: fi.Size(), modNano: fi.ModTime().UnixNano()}
}

// fresh reports whether the cached entry for name still matches disk.
func (l *FSLoader) fresh(name string, now fileStamp) bool {
	l.stampMu.RLock()
	was, ok := l.stamps[name]
	l.stampMu.RUnlock()
	return ok && was == now
}

// stamp records what an entry was read at.
func (l *FSLoader) stamp(name string, at fileStamp) {
	l.stampMu.Lock()
	if l.stamps == nil {
		l.stamps = make(map[string]fileStamp)
	}
	l.stamps[name] = at
	l.stampMu.Unlock()
}

// NewFSLoader creates a caching Loader rooted at the given directory.
//
// rootDir is normalized to OS-native separators (filepath.Clean). List and
// Walk strip `rootDir + Separator` from filepath.Glob/Walk output to return
// relative paths; those APIs emit OS-native separators, so an unnormalized
// forward-slash root (e.g. a `--folder C:/foo` CLI arg on Windows) would fail
// the prefix strip and leak absolute paths, breaking every downstream
// ReadFile. Cleaning here keeps the strip prefix in the same separator form.
func NewFSLoader(rootDir string) *FSLoader {
	return &FSLoader{
		baseLoader: newBaseLoader(true),
		rootDir:    filepath.Clean(rootDir),
		stamps:     make(map[string]fileStamp),
	}
}

// ReadFile returns the bytes for the exact path, re-reading whenever the file
// on disk no longer matches what the cached copy was read at.
// Returns nil, nil if the file does not exist.
func (l *FSLoader) ReadFile(name string) ([]byte, error) {
	// A declared-immutable source skips the stat: re-reading content that
	// cannot change cannot return anything different, so checking is pure cost.
	if l.immutable {
		if data, ok := l.cached(name); ok {
			return data, nil
		}
	} else if now := l.stampOf(name); l.fresh(name, now) {
		if data, ok := l.cached(name); ok {
			return data, nil
		}
	}
	// Stat BEFORE the read. A file rewritten between the two would otherwise be
	// stamped as current while the cache holds the older bytes, and the next
	// read would trust it.
	now := l.stampOf(name)

	data, err := os.ReadFile(filepath.Join(l.rootDir, filepath.FromSlash(name)))
	if err != nil {
		if os.IsNotExist(err) {
			l.store(name, nil) // cache the miss
			l.stamp(name, fileStamp{missing: true})
			return nil, nil
		}
		return nil, fmt.Errorf("config %q: read error: %w", name, err)
	}

	l.store(name, data)
	// Stamp from the stat taken BEFORE the read. A file rewritten between the
	// two would otherwise be stamped as current while holding the older bytes,
	// and the next read would trust it.
	l.stamp(name, now)
	return data, nil
}

// List returns the full relative paths of files matching the glob pattern.
// Pattern is relative to rootDir (e.g. "queries/*.yaml").
func (l *FSLoader) List(pattern string) []string {
	matches, err := filepath.Glob(filepath.Join(l.rootDir, filepath.FromSlash(pattern)))
	if err != nil {
		return nil
	}
	prefix := l.rootDir + string(filepath.Separator)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		rel := strings.TrimPrefix(m, prefix)
		result = append(result, filepath.ToSlash(rel))
	}
	return result
}

// Walk returns every file under rootDir, recursively. Paths are
// forward-slash-relative to rootDir. Directories are not included.
// Returns nil if rootDir doesn't exist or isn't readable.
func (l *FSLoader) Walk() []string {
	var out []string
	prefix := l.rootDir + string(filepath.Separator)
	_ = filepath.Walk(l.rootDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // tolerate transient errors; skip subtree
		}
		if info.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, prefix)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out
}

// Exists checks whether the exact path exists on disk.
func (l *FSLoader) Exists(path string) bool {
	_, err := os.Stat(filepath.Join(l.rootDir, filepath.FromSlash(path)))
	return err == nil
}

// Reset evicts the exact path from the cache so the next call re-reads from disk.
func (l *FSLoader) Reset(path string) {
	l.evict(path)
	l.unstamp(path)
}

// Invalidate drops every cached read, so the next read of any path goes to
// disk. For a caller told that a source changed wholesale, rather than one
// checking a path at a time.
func (l *FSLoader) Invalidate() {
	l.stampMu.Lock()
	l.stamps = make(map[string]fileStamp)
	l.stampMu.Unlock()
	l.mu.Lock()
	if l.cache != nil {
		l.cache = make(map[string][]byte)
	}
	l.mu.Unlock()
}

// unstamp forgets what a path was read at.
func (l *FSLoader) unstamp(name string) {
	l.stampMu.Lock()
	delete(l.stamps, name)
	l.stampMu.Unlock()
}

// WriteFile writes data to an exact path, creating parent directories as needed.
// Evicts the cache entry so the next read sees the new data.
func (l *FSLoader) WriteFile(name string, data []byte) error {
	full := filepath.Join(l.rootDir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create dir for %q: %w", name, err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", name, err)
	}
	l.evict(name)
	l.unstamp(name)
	return nil
}

// DeleteFile removes the file at the exact path.
// Returns nil if the file does not exist.
func (l *FSLoader) DeleteFile(name string) error {
	full := filepath.Join(l.rootDir, filepath.FromSlash(name))
	err := os.Remove(full)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %q: %w", name, err)
	}
	l.evict(name)
	l.unstamp(name)
	return nil
}

// Ensure FSLoader implements WritableLoader.
var _ WritableLoader = (*FSLoader)(nil)
