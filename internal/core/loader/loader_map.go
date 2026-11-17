// SPDX-License-Identifier: Apache-2.0

package loader

import "path"

// MapLoader is a [Loader] backed by an in-memory map.
// Intended for unit tests — inject full-path → content pairs without touching disk.
// Keys are full relative paths including extension (e.g. "queries/factapp.yaml", "settings.yaml").
//
//	loader := NewMapLoader(map[string]string{
//	    "leanix.yaml":           `domain: tenant.example`,
//	    "queries/factapp.yaml":  `fstype: Application`,
//	})
//
// Reset is a no-op — mutate the underlying map if needed.
type MapLoader struct {
	m map[string]string
}

// NewMapLoader creates a MapLoader from path → content pairs.
func NewMapLoader(m map[string]string) *MapLoader {
	return &MapLoader{m: m}
}

// ReadFile returns the raw bytes for the exact path.
// Returns nil, nil if not found.
func (l *MapLoader) ReadFile(name string) ([]byte, error) {
	if s, ok := l.m[name]; ok {
		return []byte(s), nil
	}
	return nil, nil
}

// List returns the full paths of all keys matching the glob pattern.
func (l *MapLoader) List(pattern string) []string {
	var result []string
	for k := range l.m {
		if matched, _ := path.Match(pattern, k); matched {
			result = append(result, k)
		}
	}
	return result
}

// Walk returns every key in the map (i.e. all "files" the loader holds).
// Order is map-iteration order, which Go does not guarantee — callers that
// need a stable order should sort the result.
func (l *MapLoader) Walk() []string {
	out := make([]string, 0, len(l.m))
	for k := range l.m {
		out = append(out, k)
	}
	return out
}

// Exists reports whether the exact path exists in the map.
func (l *MapLoader) Exists(name string) bool {
	_, ok := l.m[name]
	return ok
}

func (l *MapLoader) Reset(_ string) {}
