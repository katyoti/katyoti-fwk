// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"reflect"
	"sync"
)

// decode_cache.go — decode each body once per process, not once per consumer.
//
// The key is `(body hash, target type)`, and that pair is enough on its own:
// the hash IS the content's identity, so the same pair cannot name two
// different values. Nothing needs invalidating, because an edit produces
// different bytes, a different hash, and therefore a different key. A provider
// epoch would be a coarser version of the same guarantee.
//
// # Decoded bodies are read-only
//
// A hit returns the SAME value every consumer holds. Go copies the struct on
// return, so setting a field is safe; a slice, map or pointer INSIDE the body
// is shared, and writing through one corrupts every other holder. Treat a body
// from QueryTyped or GetOne as immutable, and copy what you intend to change.
//
// That sharing is the point — it is what makes twenty agents reading the same
// skill cost one decode rather than twenty.

type decodeKey struct {
	hash string
	typ  reflect.Type
}

// decodeCacheMax bounds the number of retained values. Distinct keys are
// naturally bounded by the catalog's size times the number of decode targets;
// the cap only matters for a long-lived process watching sources that change,
// where hashes churn. Over the cap the map is dropped whole — a cache flush
// costs re-decoding, never correctness, so the policy stays trivial.
const decodeCacheMax = 8192

var (
	decodeMu    sync.RWMutex
	decodeCache = map[decodeKey]any{}
)

// decodeCached decodes body into T, reusing a previous decode of the same
// content into the same type.
//
// An empty hash means the content is not addressed — the entry is decoded
// without being cached, since there is no key that can safely identify it.
func decodeCached[T any](hash string, body []byte) (T, error) {
	var zero T
	if len(body) == 0 {
		return zero, nil
	}
	if hash == "" {
		var v T
		if err := json.Unmarshal(body, &v); err != nil {
			return zero, err
		}
		return v, nil
	}

	key := decodeKey{hash: hash, typ: reflect.TypeFor[T]()}
	decodeMu.RLock()
	hit, ok := decodeCache[key]
	decodeMu.RUnlock()
	if ok {
		return hit.(T), nil
	}

	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return zero, err
	}

	decodeMu.Lock()
	if len(decodeCache) >= decodeCacheMax {
		decodeCache = make(map[decodeKey]any, decodeCacheMax)
	}
	decodeCache[key] = v
	decodeMu.Unlock()
	return v, nil
}
