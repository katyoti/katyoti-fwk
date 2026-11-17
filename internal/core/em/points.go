// SPDX-License-Identifier: Apache-2.0

package em

import (
	"maps"
	"sync"
)

// ListPoint is an append-only list implementation of ExtensionPoint[T].
type ListPoint[T any] struct {
	mu    sync.RWMutex
	items []T
}

// NewListPoint creates a new ListPoint whose ID is derived from T.
func NewListPoint[T any]() *ListPoint[T] {
	return &ListPoint[T]{}
}

func (p *ListPoint[T]) Contribute(item T) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = append(p.items, item)
	return nil
}

func (p *ListPoint[T]) Entries() []T {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]T, len(p.items))
	copy(out, p.items)
	return out
}

// IndexedPointOption configures an IndexedPoint.
type IndexedPointOption func(*indexedPointConfig)

type indexedPointConfig struct {
	allowReplace bool
}

// AllowReplace permits overwriting existing keys in an IndexedPoint.
func AllowReplace() IndexedPointOption {
	return func(c *indexedPointConfig) { c.allowReplace = true }
}

// IndexedPoint is a keyed O(1) lookup implementation of ExtensionPoint[T].
// Items are indexed by a key extracted via keyFn.
type IndexedPoint[T any] struct {
	keyFn        func(T) string
	allowReplace bool
	mu           sync.RWMutex
	items        map[string]T
	order        []string // insertion order for deterministic Entries()
}

// NewIndexedPoint creates a new IndexedPoint whose ID is derived from T.
func NewIndexedPoint[T any](keyFn func(T) string, opts ...IndexedPointOption) *IndexedPoint[T] {
	cfg := &indexedPointConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return &IndexedPoint[T]{
		keyFn:        keyFn,
		allowReplace: cfg.allowReplace,
		items:        make(map[string]T),
	}
}

func (p *IndexedPoint[T]) Contribute(item T) error {
	key := p.keyFn(item)
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.items[key]; exists && !p.allowReplace {
		return ErrDuplicateKey{PointID: PointID[T](), Key: key}
	}
	if _, exists := p.items[key]; !exists {
		p.order = append(p.order, key)
	}
	p.items[key] = item
	return nil
}

func (p *IndexedPoint[T]) Entries() []T {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]T, 0, len(p.order))
	for _, key := range p.order {
		out = append(out, p.items[key])
	}
	return out
}

// EntriesMap returns all contributed items as a map keyed by their key function.
func (p *IndexedPoint[T]) EntriesMap() map[string]T {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]T, len(p.items))
	maps.Copy(out, p.items)
	return out
}

// Get returns the item with the given key, or the zero value if not found.
func (p *IndexedPoint[T]) Get(key string) T {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.items[key]
}

// Has returns true if the given key exists.
func (p *IndexedPoint[T]) Has(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.items[key]
	return ok
}

// Keys returns all keys in insertion order.
func (p *IndexedPoint[T]) Keys() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.order))
	copy(out, p.order)
	return out
}
