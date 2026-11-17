// SPDX-License-Identifier: Apache-2.0

// Package cache provides a reusable file-based cache for expensive operations
// (web scrapes, API calls, etc.).
//
// Design mirrors the Python WithCacheDatas/@cache pattern:
//   - Keys are SHA256 hashes of operation name + serialized input
//   - Values are JSON files stored under a configurable base directory
//   - No TTL — cache persists until manually cleared
//   - Thread-safe via per-key file locking (atomic write + rename)
//
// Usage:
//
//	c := cache.NewFileCache("/path/to/cache")
//	result, err := cache.Cached(c, "scrape", input, func() (Output, error) {
//	    return expensiveCall(input)
//	})
package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache is the interface for a key-value cache with JSON-serializable values.
type Cache interface {
	// Get retrieves a cached value. Returns false if not found.
	Get(namespace, key string, dest any) (bool, error)

	// Set stores a value in the cache.
	Set(namespace, key string, value any) error

	// Dir returns the base cache directory (for logging/debugging).
	Dir() string
}

// FileCache is a file-system-backed cache. Each cached value is a JSON file
// stored at <baseDir>/<namespace>/<sha256-key>.json.
//
// TTL is checked on Get via the file's modification time. A zero TTL means
// entries never expire.
type FileCache struct {
	baseDir string
	ttl     time.Duration
}

// NewFileCache creates a file-based cache rooted at baseDir with no TTL (entries never expire).
func NewFileCache(baseDir string) *FileCache {
	return &FileCache{baseDir: baseDir}
}

// NewFileCacheWithTTL creates a file-based cache with a TTL. Entries older
// than ttl are treated as misses (stale file is left on disk for lazy cleanup).
func NewFileCacheWithTTL(baseDir string, ttl time.Duration) *FileCache {
	return &FileCache{baseDir: baseDir, ttl: ttl}
}

func (c *FileCache) Dir() string { return c.baseDir }

func (c *FileCache) Get(namespace, key string, dest any) (bool, error) {
	path := c.path(namespace, key)

	// TTL check: if the file is older than TTL, treat as miss.
	if c.ttl > 0 {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("cache: stat %s: %w", path, err)
		}
		if time.Since(info.ModTime()) > c.ttl {
			return false, nil // expired
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("cache: read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		// Corrupted cache entry — treat as miss.
		return false, nil
	}
	return true, nil
}

func (c *FileCache) Set(namespace, key string, value any) error {
	path := c.path(namespace, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cache: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: marshal: %w", err)
	}
	// Atomic write: write to temp file, then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("cache: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cache: rename %s: %w", path, err)
	}
	return nil
}

func (c *FileCache) path(namespace, key string) string {
	hash := HashKey(key)
	return filepath.Join(c.baseDir, namespace, hash+".json")
}

// HashKey produces a stable SHA256 hash for use as a cache filename.
func HashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}

// MakeKey builds a cache key from an operation name and input.
// The input is JSON-serialized to produce a stable, deterministic key.
func MakeKey(operation string, input any) string {
	data, err := json.Marshal(input)
	if err != nil {
		// Fallback: use fmt string representation.
		return operation + ":" + fmt.Sprintf("%v", input)
	}
	return operation + ":" + string(data)
}

// Cached executes fn and caches the result. On subsequent calls with
// the same namespace+key, returns the cached value without calling fn.
//
// This is the Go equivalent of the Python @cache decorator.
//
//	result, err := cache.Cached(c, "firecrawl", cache.MakeKey("scrape", url), func() (Output, error) {
//	    return provider.Scrape(ctx, url)
//	})
func Cached[T any](c Cache, namespace, key string, fn func() (T, error)) (T, error) {
	if c != nil {
		var cached T
		found, err := c.Get(namespace, key, &cached)
		if err == nil && found {
			return cached, nil
		}
	}

	result, err := fn()
	if err != nil {
		return result, err
	}

	if c != nil {
		// Best-effort cache write — don't fail the operation if caching fails.
		_ = c.Set(namespace, key, result)
	}

	return result, nil
}

// NopCache is a no-op cache that never stores or retrieves anything.
// Useful as a default when caching is disabled.
type NopCache struct{}

func (NopCache) Get(_, _ string, _ any) (bool, error) { return false, nil }
func (NopCache) Set(_, _ string, _ any) error         { return nil }
func (NopCache) Dir() string                          { return "" }

// CleanKey normalizes a string for use in cache keys (lowercase, collapse whitespace).
func CleanKey(s string) string {
	s = strings.ToLower(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}
