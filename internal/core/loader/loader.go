// SPDX-License-Identifier: Apache-2.0

package loader

import "sync"

// Loader is an opaque source of named files.
// Implementations differ in where they read from (local FS, embed.FS, combined).
// Callers interact only through this interface — no filesystem paths are exposed.
//
// Coordinate system: all paths are forward-slash-separated paths relative to the
// loader's root (e.g. "queries/factapp.yaml", "settings.yaml").
type Loader interface {
	// ReadFile returns the raw bytes for an exact path.
	// Returns nil, nil if the file does not exist.
	ReadFile(path string) ([]byte, error)

	// List returns the full relative paths of all files matching a glob pattern.
	// Pattern uses path.Match syntax, e.g. "queries/*.yaml" or "fieldsets/*".
	// Returns nil if no matches are found.
	List(pattern string) []string

	// Walk returns the full relative paths of every file under root, recursively.
	// Directories are not included. Order is unspecified. Used by discovery code
	// that needs to classify files by structural relationship rather than glob
	// (e.g. task discovery accepting both `<name>/TASK.md` folder tasks and
	// `<name>.md` flat tasks; the classifier needs to see whole subtrees).
	Walk() []string

	// Exists reports whether the exact path exists.
	Exists(path string) bool

	// Reset evicts the exact path from any internal cache
	// so the next call re-reads from the source.
	Reset(path string)
}

// WritableLoader extends Loader with write and delete operations.
// Only FSLoader implements this — embed and compound loaders are read-only.
type WritableLoader interface {
	Loader

	// WriteFile writes data to an exact path (creates parent dirs as needed).
	WriteFile(path string, data []byte) error

	// DeleteFile removes the file at the exact path.
	// Returns nil if the file does not exist.
	DeleteFile(path string) error
}

// baseLoader provides optional byte-level caching for Loader implementations.
// Embed it into a concrete loader and call cached/store/evict instead of
// managing sync.RWMutex and map[string][]byte directly.
//
// A nil-valued entry in the cache means "file was looked up and does not exist"
// (negative cache), preventing repeated disk/FS lookups for missing files.
type baseLoader struct {
	useCache bool
	mu       sync.RWMutex
	cache    map[string][]byte
}

// newBaseLoader constructs a baseLoader. Pass useCache=true to enable the cache.
//
// Returns a composite literal rather than filling in a local: a local of this
// type holds a sync.RWMutex, and returning it copies the lock — harmless while
// the lock is still zero-valued, which is exactly the kind of "harmless today"
// `go vet`'s copylocks check exists to stop becoming load-bearing.
func newBaseLoader(useCache bool) baseLoader {
	if !useCache {
		return baseLoader{}
	}
	return baseLoader{useCache: true, cache: make(map[string][]byte)}
}

// cached returns (data, true) on a cache hit; (nil, false) on a miss or when
// caching is disabled. A nil data + true means the file is known-missing.
func (b *baseLoader) cached(name string) ([]byte, bool) {
	if !b.useCache {
		return nil, false
	}
	b.mu.RLock()
	data, ok := b.cache[name]
	b.mu.RUnlock()
	return data, ok
}

// store writes data into the cache under name. Pass nil to record a miss.
// No-op when caching is disabled.
func (b *baseLoader) store(name string, data []byte) {
	if !b.useCache {
		return
	}
	b.mu.Lock()
	b.cache[name] = data
	b.mu.Unlock()
}

// evict removes name from the cache so the next call re-reads from the source.
// No-op when caching is disabled.
func (b *baseLoader) evict(name string) {
	if !b.useCache {
		return
	}
	b.mu.Lock()
	delete(b.cache, name)
	b.mu.Unlock()
}

// ImmutableSource is implemented by a loader that can state whether its content
// can change while the process runs.
//
// The answer drives every cache downstream: an immutable source is read, decoded
// and validated once and never revalidated; a mutable one must be re-derived
// when it changes. A loader that does NOT implement this is treated as mutable,
// because assuming immutability serves stale content for exactly the sources
// designed to be updated — and that failure is silent.
type ImmutableSource interface {
	// Immutable reports whether this loader's content is fixed for the
	// lifetime of the process.
	Immutable() bool
}

// IsImmutable reports whether l is a provably fixed source. Unknown means
// mutable: the safe answer is the one that re-reads.
func IsImmutable(l Loader) bool {
	if is, ok := l.(ImmutableSource); ok {
		return is.Immutable()
	}
	return false
}
