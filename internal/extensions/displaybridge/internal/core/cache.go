// SPDX-License-Identifier: Apache-2.0

package core

import (
	"container/list"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// schemaCacheCapacity bounds the JSON Schema cache. 256 fits ~all
// realistic per-universe schema counts (single-digit hundreds is the
// expected steady state) while keeping memory predictable. When exceeded
// the LRU entry is evicted; the next ValidateValue against that schema
// re-fetches via GenerateJSONSchema.
const schemaCacheCapacity = 256

// schemaCacheEntry is one cached compiled schema together with the bun
// version it came from. version is informational here (Phase 1) — the
// SSE schema-changed channel (Subtask 5) will use it for selective
// invalidation; until then, version stays as whatever bun returned at
// fetch time (often 0 if bun does not surface it yet).
type schemaCacheEntry struct {
	name     string
	version  int
	compiled *jsonschema.Schema
}

// schemaCache is a bounded LRU cache of compiled JSON Schemas keyed by
// schema name. Safe for concurrent use.
//
// Why name-only keys despite the task plan saying "(schemaName,
// schemaVersion)": at lookup time the caller has only the name (the
// validation request input has no version field). Storing version inside
// the entry lets us invalidate by name AND match SSE events by
// (name, version) — the version is metadata, not a lookup dimension.
type schemaCache struct {
	mu       sync.Mutex
	items    map[string]*list.Element // name → list element (carries *schemaCacheEntry)
	order    *list.List               // back = LRU, front = MRU
	capacity int
}

// newSchemaCache builds an empty cache with the given capacity. capacity
// ≤ 0 falls back to schemaCacheCapacity.
func newSchemaCache(capacity int) *schemaCache {
	if capacity <= 0 {
		capacity = schemaCacheCapacity
	}
	return &schemaCache{
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
		capacity: capacity,
	}
}

// Get returns the cached compiled schema for name, marking it as most
// recently used. ok=false on miss.
func (c *schemaCache) Get(name string) (compiled *jsonschema.Schema, version int, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, found := c.items[name]
	if !found {
		return nil, 0, false
	}
	c.order.MoveToFront(el)
	e := el.Value.(*schemaCacheEntry)
	return e.compiled, e.version, true
}

// Put inserts or refreshes the cache entry for name. If the cache is at
// capacity, the LRU entry is evicted first.
func (c *schemaCache) Put(name string, version int, compiled *jsonschema.Schema) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, found := c.items[name]; found {
		// Refresh existing entry in place.
		entry := el.Value.(*schemaCacheEntry)
		entry.version = version
		entry.compiled = compiled
		c.order.MoveToFront(el)
		return
	}

	entry := &schemaCacheEntry{name: name, version: version, compiled: compiled}
	el := c.order.PushFront(entry)
	c.items[name] = el

	// Evict if over capacity.
	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			oldEntry := oldest.Value.(*schemaCacheEntry)
			delete(c.items, oldEntry.name)
			c.order.Remove(oldest)
		}
	}
}

// Invalidate removes the entry for name (no-op if absent). Used by the
// SSE schema-changed handler in Subtask 5.
func (c *schemaCache) Invalidate(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, found := c.items[name]
	if !found {
		return
	}
	delete(c.items, name)
	c.order.Remove(el)
}

// Clear drops all entries. Used on bun supervisor restart and on SSE
// connection drop (safer than serving potentially stale data).
func (c *schemaCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element, c.capacity)
	c.order = list.New()
}

// Size reports the current number of cached entries. Test/inspection helper.
func (c *schemaCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
