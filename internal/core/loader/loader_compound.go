// SPDX-License-Identifier: Apache-2.0

package loader

// CompoundLoader combines multiple Loaders with last-wins priority.
// The last Loader in the slice has the highest priority: if it has a file,
// its version is returned without consulting earlier loaders.
// This models the "user overrides embedded defaults" pattern naturally:
//
//	NewCompoundLoader(embeddedLoader, userLoader)
//	// → userLoader wins if the file exists there; falls back to embeddedLoader
type CompoundLoader struct {
	loaders []Loader
}

// NewCompoundLoader creates a Loader that combines multiple sub-loaders with last-wins priority.
// Later loaders override earlier ones (last wins).
//
// As a convenience, if exactly one loader is provided it is returned directly —
// no CompoundLoader wrapper is created. This means callers can always write:
//
//	loader := NewCompoundLoader(loaders...)
//
// without a special-case for the single-loader scenario.
func NewCompoundLoader(loaders ...Loader) Loader {
	if len(loaders) == 1 {
		return loaders[0]
	}
	return &CompoundLoader{loaders: loaders}
}

// ReadFile returns the raw bytes from the last loader that has the file.
// Loaders are checked in reverse order (last = highest priority).
// Returns nil, nil if no loader has the file.
func (c *CompoundLoader) ReadFile(name string) ([]byte, error) {
	for i := len(c.loaders) - 1; i >= 0; i-- {
		data, err := c.loaders[i].ReadFile(name)
		if err != nil {
			return nil, err
		}
		if data != nil {
			return data, nil
		}
	}
	return nil, nil
}

// List returns the deduplicated union of full paths from all loaders for the given pattern.
// Duplicates are collapsed — the last loader's version is returned by ReadFile/LoadFile.
func (c *CompoundLoader) List(pattern string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, l := range c.loaders {
		for _, p := range l.List(pattern) {
			if _, exists := seen[p]; !exists {
				seen[p] = struct{}{}
				result = append(result, p)
			}
		}
	}
	return result
}

// Walk returns the deduplicated union of file paths from every sub-loader.
// Order follows sub-loader order; the first occurrence wins (later loaders
// don't shift positions). Used by callers that need to enumerate the full
// merged file set rather than match a glob pattern.
func (c *CompoundLoader) Walk() []string {
	seen := make(map[string]struct{})
	var result []string
	for _, l := range c.loaders {
		for _, p := range l.Walk() {
			if _, exists := seen[p]; !exists {
				seen[p] = struct{}{}
				result = append(result, p)
			}
		}
	}
	return result
}

// Exists reports whether any loader has the file (checked last-first).
func (c *CompoundLoader) Exists(name string) bool {
	for i := len(c.loaders) - 1; i >= 0; i-- {
		if c.loaders[i].Exists(name) {
			return true
		}
	}
	return false
}

// Reset propagates to all sub-loaders.
func (c *CompoundLoader) Reset(name string) {
	for _, l := range c.loaders {
		l.Reset(name)
	}
}
