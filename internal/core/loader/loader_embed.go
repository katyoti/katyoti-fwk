// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

// EmbedFSLoader is a Loader backed by an fs.FS (typically embed.FS).
// dir is the root directory prefix within the FS — files are read as {dir}/{path}.
// Caching is disabled by default because embed.FS is immutable; use
// NewEmbedFSLoaderCached if you want a cache (e.g. to avoid repeated path.Join work).
type EmbedFSLoader struct {
	// immutable is set by MarkImmutable for sources that are fixed but not
	// detectable as such.
	immutable bool

	baseLoader
	fsys fs.FS
	dir  string
}

// NewEmbedFSLoader returns a loader over fsys rooted at dir.
//
// Reads are cached automatically when the source is provably fixed (an
// embed.FS): re-reading content compiled into the binary cannot return anything
// different, so the only effect of not caching is cost. A mutable source is not
// cached here — a stale read is worse than a slow one, and invalidation is the
// caller's business.
func NewEmbedFSLoader(fsys fs.FS, dir string) *EmbedFSLoader {
	_, fixed := fsys.(embed.FS)
	return &EmbedFSLoader{baseLoader: newBaseLoader(fixed), fsys: fsys, dir: dir}
}

// NewEmbedFSLoaderCached creates a caching Loader backed by fsys rooted at dir.
// Useful when the same files are read many times in a hot path.
func NewEmbedFSLoaderCached(fsys fs.FS, dir string) *EmbedFSLoader {
	return &EmbedFSLoader{baseLoader: newBaseLoader(true), fsys: fsys, dir: dir}
}

// ReadFile returns the raw bytes for the exact path from the embedded FS.
// Returns nil, nil if the file does not exist.
func (l *EmbedFSLoader) ReadFile(name string) ([]byte, error) {
	if data, ok := l.cached(name); ok {
		return data, nil
	}

	p := path.Join(l.dir, name)
	data, err := fs.ReadFile(l.fsys, p)
	if err != nil {
		l.store(name, nil) // cache the miss
		return nil, nil    // not found — not an error
	}

	l.store(name, data)
	return data, nil
}

// List returns the full relative paths of files matching the glob pattern.
// Pattern is relative to dir (e.g. "queries/*.yaml").
func (l *EmbedFSLoader) List(pattern string) []string {
	matches, err := fs.Glob(l.fsys, path.Join(l.dir, pattern))
	if err != nil {
		return nil
	}
	prefix := ""
	if l.dir != "" {
		prefix = l.dir + "/"
	}
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		result = append(result, strings.TrimPrefix(m, prefix))
	}
	return result
}

// Walk returns every file under dir, recursively. Paths are returned as
// forward-slash-relative to dir. Directories are not included.
func (l *EmbedFSLoader) Walk() []string {
	var out []string
	root := l.dir
	if root == "" {
		root = "."
	}
	prefix := ""
	if l.dir != "" {
		prefix = l.dir + "/"
	}
	_ = fs.WalkDir(l.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		out = append(out, strings.TrimPrefix(p, prefix))
		return nil
	})
	return out
}

// Exists reports whether the exact path is present in the embedded FS.
func (l *EmbedFSLoader) Exists(p string) bool {
	f, err := l.fsys.Open(path.Join(l.dir, p))
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// Reset evicts the exact path from the cache (no-op when caching is disabled).
func (l *EmbedFSLoader) Reset(p string) {
	l.evict(p)
}

// Immutable reports whether the underlying filesystem is fixed.
//
// Detected rather than declared where possible: an `embed.FS` is compiled into
// the binary and cannot change. Anything else — a directory opened as an fs.FS,
// a sub-filesystem, a test double — is treated as mutable, because this
// constructor accepts any fs.FS and a wrong "immutable" is served stale forever.
// Callers that know better can say so with MarkImmutable.
func (l *EmbedFSLoader) Immutable() bool {
	if l.immutable {
		return true
	}
	_, isEmbed := l.fsys.(embed.FS)
	return isEmbed
}

// MarkImmutable declares this loader's content fixed for the process lifetime.
// Use it when the source is provably stable but not an embed.FS — a sub-FS of
// one, say. Wrong here means stale content with no signal, so declare it only
// when the source genuinely cannot change.
func (l *EmbedFSLoader) MarkImmutable() *EmbedFSLoader {
	l.immutable = true
	return l
}
