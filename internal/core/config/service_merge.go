// SPDX-License-Identifier: Apache-2.0

package config

import "strings"

// dotGet navigates a nested map by dot-separated key.
// Returns the value and true if found, or nil and false.
func dotGet(m map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")

	var current any = m
	for _, part := range parts {
		cm, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = cm[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// deepMerge copies all keys from src into dst recursively.
// Existing keys in dst are overwritten unless both values are maps,
// in which case they are merged recursively.
func deepMerge(dst, src map[string]any) {
	for k, srcVal := range src {
		dstVal, exists := dst[k]
		if !exists {
			dst[k] = srcVal
			continue
		}

		// If both are maps, merge recursively.
		dstMap, dstOk := dstVal.(map[string]any)
		srcMap, srcOk := srcVal.(map[string]any)
		if dstOk && srcOk {
			deepMerge(dstMap, srcMap)
			continue
		}

		// Otherwise, src wins.
		dst[k] = srcVal
	}
}
