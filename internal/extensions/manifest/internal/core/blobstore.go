// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// blobstore.go — the bulk plane: content of arbitrary size, out of band.
//
// # Hash is identity, locator is address
//
// Two copies of the same content have the same hash and ARE the same content,
// wherever they sit — so they deduplicate, and a consumer that already has the
// hash never fetches again. The locator is merely where to go and get it this
// time: it is minted when a fetch is about to happen and is never written into
// the index. An index that stored locators would be an index that expires, and
// would not survive being moved between environments.
//
// # No filesystem in the contract
//
// Nothing in BlobStore says "file". Components do not share a filesystem — they
// are separate processes, and in a cluster separate machines — so a contract
// phrased in paths would be a contract that only works on one box. Swapping the
// filesystem backend for an object store is then a swap, not a rewrite.

// ErrBlobNotFound is returned when a hash is not in the store.
var ErrBlobNotFound = errors.New("blob: not found")

// BlobStore holds content addressed by hash.
//
// Every method takes a hash, never a URI. A contribution's URI can change, be
// re-pointed or be served by a different provider; its content's hash cannot.
// Keying on the hash is what makes eviction safe and deduplication automatic.
type BlobStore interface {
	// Put stores content under `hash`. Idempotent: content already present
	// is left alone, which is what makes two providers shipping identical
	// content cost one copy.
	Put(ctx context.Context, hash string, r io.Reader) error

	// Locate mints an address for fetching this hash. Called at fetch time
	// and never persisted: an address may be short-lived, scoped or signed,
	// and storing one would put an expiring value in a durable place.
	Locate(ctx context.Context, hash string) (string, error)

	// Open returns a reader over a byte range. `offset` starts the read;
	// `length` <= 0 means "to the end". Range support is what makes a large
	// fetch resumable — an interrupted transfer continues instead of
	// starting over.
	Open(ctx context.Context, hash string, offset, length int64) (io.ReadCloser, error)

	// Stat reports the size of stored content, and whether it is present at
	// all, without reading it.
	Stat(ctx context.Context, hash string) (size int64, ok bool)

	// Evict removes content BY HASH. Never by URI: a URI names a
	// contribution, and several contributions may share one blob, so
	// evicting by URI would delete content still referenced elsewhere.
	//
	// Safe to call while a reader is open. A backend that cannot remove
	// content in use reports success and reclaims it later — eviction is
	// housekeeping on a cache, so being unable to reclaim immediately is
	// never an error a caller can act on.
	Evict(ctx context.Context, hash string) error
}

// FileBlobStore is the filesystem backend, and the one local development runs
// against. It mints `file://` locators, so the fetch path that will later
// receive `https://` addresses from an object store is exercised now, with the
// same code — the backend changes, the consumer does not.
type FileBlobStore struct {
	root string

	// evicted is what makes eviction mean something the filesystem cannot
	// always deliver. A reader holding content open prevents its removal —
	// on Windows, outright — so removal alone cannot be what "evicted"
	// means. Addressability is the STORE's concern: a tombstoned hash stops
	// resolving at once, and the bytes are reclaimed by Sweep once the
	// holder lets go. Readers already open are untouched, because evicting
	// content must not corrupt a read in flight.
	mu      sync.Mutex
	evicted map[string]bool
}

// NewFileBlobStore roots a store at a directory. It is created on first write;
// reads tolerate its absence as a clean miss.
func NewFileBlobStore(root string) *FileBlobStore {
	return &FileBlobStore{root: root, evicted: map[string]bool{}}
}

// isEvicted reports whether this name has been tombstoned.
func (s *FileBlobStore) isEvicted(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evicted[name]
}

// Root returns the directory this store is rooted at.
func (s *FileBlobStore) Root() string { return s.root }

// blobName maps a hash to its file name: the bare hex digest, with any
// algorithm prefix stripped. Rejects anything carrying a path separator, so a
// hash can never address a file outside the store.
func blobName(hash string) (string, error) {
	h := strings.TrimSpace(hash)
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[i+1:]
	}
	if h == "" || strings.ContainsAny(h, `/\.`) {
		return "", fmt.Errorf("blob: invalid hash %q", hash)
	}
	return h, nil
}

// Put streams content to disk and verifies it hashes to what was claimed.
//
// The content is never held whole: it is copied through a fixed buffer while
// being hashed, so storing a multi-gigabyte layer costs the buffer, not the
// layer. Verification happens as it streams, and a mismatch removes the
// partial file rather than leaving content at an address that does not
// describe it.
func (s *FileBlobStore) Put(_ context.Context, hash string, r io.Reader) error {
	name, err := blobName(hash)
	if err != nil {
		return err
	}
	final := filepath.Join(s.root, name)
	// Already present: nothing to do. The content at an address was verified
	// when it was stored, so it is already the right content — re-reading the
	// caller's stream to check it again would cost a full transfer and could
	// not change the outcome. This is what makes two providers shipping
	// identical content cost one copy.
	if _, statErr := os.Stat(final); statErr == nil {
		s.mu.Lock()
		delete(s.evicted, name) // re-put revives a tombstoned address
		s.mu.Unlock()
		return nil
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("blob: mkdir %s: %w", s.root, err)
	}
	tmp, err := os.CreateTemp(s.root, name+".tmp-*")
	if err != nil {
		return fmt.Errorf("blob: create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }

	sum := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, sum), r); err != nil {
		cleanup()
		return fmt.Errorf("blob: write %s: %w", name, err)
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != name {
		cleanup()
		return fmt.Errorf("blob: hash mismatch (claimed %q, actual sha256:%s)", hash, got)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("blob: close temp: %w", err)
	}
	// Temp + rename: the canonical name never names a partial file, so a
	// crash mid-write cannot leave content that fails its own address.
	if err := os.Rename(tmpName, final); err != nil {
		if _, statErr := os.Stat(final); statErr == nil {
			_ = os.Remove(tmpName) // a concurrent writer won; same content
			return nil
		}
		_ = os.Remove(tmpName)
		return fmt.Errorf("blob: rename %s: %w", name, err)
	}
	s.mu.Lock()
	delete(s.evicted, name)
	s.mu.Unlock()
	return nil
}

// Locate returns a `file://` URL for the hash, minted now.
func (s *FileBlobStore) Locate(_ context.Context, hash string) (string, error) {
	name, err := blobName(hash)
	if err != nil {
		return "", err
	}
	if s.isEvicted(name) {
		return "", fmt.Errorf("%w: %s", ErrBlobNotFound, hash)
	}
	p := filepath.Join(s.root, name)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("%w: %s", ErrBlobNotFound, hash)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return fileURL(abs), nil
}

// fileURL renders an absolute path as a file:// URL. Windows paths need the
// extra leading slash and forward separators, so `file://` locators mean the
// same thing on every platform the fetcher runs on.
func fileURL(abs string) string {
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// Open returns a reader over a byte range, seeking rather than reading forward,
// so a resumed transfer costs nothing for the part already held.
func (s *FileBlobStore) Open(_ context.Context, hash string, offset, length int64) (io.ReadCloser, error) {
	name, err := blobName(hash)
	if err != nil {
		return nil, err
	}
	if s.isEvicted(name) {
		return nil, fmt.Errorf("%w: %s", ErrBlobNotFound, hash)
	}
	f, err := os.Open(filepath.Join(s.root, name))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBlobNotFound, hash)
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("blob: seek %s to %d: %w", hash, offset, err)
		}
	}
	if length <= 0 {
		return f, nil
	}
	return &sectionCloser{Reader: io.LimitReader(f, length), closer: f}, nil
}

// sectionCloser bounds a read while keeping the underlying file closable.
type sectionCloser struct {
	io.Reader
	closer io.Closer
}

func (s *sectionCloser) Close() error { return s.closer.Close() }

// Stat reports size and presence without opening.
func (s *FileBlobStore) Stat(_ context.Context, hash string) (int64, bool) {
	name, err := blobName(hash)
	if err != nil {
		return 0, false
	}
	if s.isEvicted(name) {
		return 0, false
	}
	fi, err := os.Stat(filepath.Join(s.root, name))
	if err != nil {
		return 0, false
	}
	return fi.Size(), true
}

// Evict removes content by hash.
//
// A reader may hold the content open — on Windows that makes removal fail
// outright. Rather than report an error a caller cannot act on, the entry is
// renamed out of the way so it stops being addressable immediately, and the
// bytes are reclaimed on the next sweep once the reader lets go. Failing to
// reclaim space is not a failure to evict.
func (s *FileBlobStore) Evict(_ context.Context, hash string) error {
	name, err := blobName(hash)
	if err != nil {
		return err
	}
	// Tombstone FIRST. Whether the bytes can be reclaimed right now depends on
	// who is holding them; whether the hash still resolves does not.
	s.mu.Lock()
	s.evicted[name] = true
	s.mu.Unlock()

	p := filepath.Join(s.root, name)
	if _, statErr := os.Stat(p); statErr != nil {
		return nil // already gone
	}
	if os.Remove(p) == nil {
		s.mu.Lock()
		delete(s.evicted, name) // reclaimed; no tombstone to keep
		s.mu.Unlock()
		return nil
	}
	// A reader holds it. Leave the bytes for Sweep. Failing to reclaim space
	// is not failing to evict, and it is not something a caller could act on.
	return nil
}

// Sweep reclaims the bytes of content that Evict tombstoned but could not
// remove because a reader held it. Returns how many entries it collected.
//
// Safe to call at any time: an entry whose holder has not let go is simply
// skipped and collected by a later sweep.
func (s *FileBlobStore) Sweep() int {
	s.mu.Lock()
	pending := make([]string, 0, len(s.evicted))
	for name := range s.evicted {
		pending = append(pending, name)
	}
	s.mu.Unlock()

	n := 0
	for _, name := range pending {
		if err := os.Remove(filepath.Join(s.root, name)); err == nil || os.IsNotExist(err) {
			s.mu.Lock()
			delete(s.evicted, name)
			s.mu.Unlock()
			n++
		}
	}
	return n
}

var _ BlobStore = (*FileBlobStore)(nil)
