// SPDX-License-Identifier: Apache-2.0

package application

import (
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// Library[T] is a typed, read-only repository of named resources.
// It combines one or more [loader.Loader]s (last-wins via [loader.CompoundLoader]) and
// provides typed List/Load/LoadAll operations over a named prefix directory.
//
// The parse function controls format handling; defaults to YAML + ProcessConfig.
// Use [NewLibraryWithParser] to supply a custom parser (e.g. for domain-specific formats).
type Library[T any] struct {
	ldr     loader.Loader
	prefix  string
	pattern string
	parse   func(string, []byte) (*T, error)
}

// NewLibrary creates a Library over prefix/ using YAML as the default format.
// Multiple loaders are combined with [loader.CompoundLoader] (last-wins priority).
func NewLibrary[T any](prefix string, loaders ...loader.Loader) *Library[T] {
	return &Library[T]{
		ldr:     resolveLoader(loaders),
		prefix:  prefix,
		pattern: "*.yaml",
		parse:   defaultParse[T],
	}
}

// NewLibraryWithParser creates a Library with a custom parser.
// pattern is the glob used to enumerate files (e.g. "*.yaml", "*.json", "*").
func NewLibraryWithParser[T any](prefix, pattern string, parse func(string, []byte) (*T, error), loaders ...loader.Loader) *Library[T] {
	if pattern == "" {
		pattern = "*"
	}
	return &Library[T]{
		ldr:     resolveLoader(loaders),
		prefix:  prefix,
		pattern: pattern,
		parse:   parse,
	}
}

func defaultParse[T any](path string, data []byte) (*T, error) {
	var t T
	if err := config.DecodeBytes(data, marshal.YAML, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// List returns the logical names (no prefix, no extension) of available items.
func (l *Library[T]) List() []string {
	paths := l.ldr.List(l.prefix + "/" + l.pattern)
	pfx := l.prefix + "/"
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		name := strings.TrimPrefix(p, pfx)
		if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
			name = name[:dot]
		}
		result = append(result, name)
	}
	return result
}

// Load loads the item named name (logical name, no extension).
// Returns nil, nil when the item does not exist.
func (l *Library[T]) Load(name string) (*T, error) {
	ext := strings.TrimPrefix(l.pattern, "*")
	fullPath := l.prefix + "/" + name + ext

	data, err := l.ldr.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return l.parse(fullPath, data)
}

// LoadAll loads every available item and returns the full slice.
func (l *Library[T]) LoadAll() ([]*T, error) {
	names := l.List()
	result := make([]*T, 0, len(names))
	for _, name := range names {
		t, err := l.Load(name)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", l.prefix, name, err)
		}
		if t != nil {
			result = append(result, t)
		}
	}
	return result, nil
}

// --- CRUD operations (require WritableLoader, e.g., FSLoader) ---

// Writable returns the loader as WritableLoader, or an error if read-only.
func (l *Library[T]) Writable() (loader.WritableLoader, error) {
	wl, ok := l.ldr.(loader.WritableLoader)
	if !ok {
		return nil, fmt.Errorf("library %s: loader is read-only", l.prefix)
	}
	return wl, nil
}

// ext returns the file extension from the pattern.
func (l *Library[T]) ext() string {
	ext := strings.TrimPrefix(l.pattern, "*")
	if ext == "" {
		return ".yaml"
	}
	return ext
}

// Save writes an item to the library as a YAML file.
func (l *Library[T]) Save(name string, item T) error {
	wl, err := l.Writable()
	if err != nil {
		return err
	}
	data, err := marshal.Marshal(item, marshal.YAML)
	if err != nil {
		return fmt.Errorf("marshal %s/%s: %w", l.prefix, name, err)
	}
	return wl.WriteFile(l.prefix+"/"+name+l.ext(), data)
}

// SaveWithHeader writes an item with a YAML comment header prepended.
func (l *Library[T]) SaveWithHeader(name string, item T, header string) error {
	wl, err := l.Writable()
	if err != nil {
		return err
	}
	data, err := marshal.Marshal(item, marshal.YAML)
	if err != nil {
		return fmt.Errorf("marshal %s/%s: %w", l.prefix, name, err)
	}
	return wl.WriteFile(l.prefix+"/"+name+l.ext(), append([]byte(header), data...))
}

// Delete removes an item from the library.
func (l *Library[T]) Delete(name string) error {
	wl, err := l.Writable()
	if err != nil {
		return err
	}
	return wl.DeleteFile(l.prefix + "/" + name + l.ext())
}

// Exists checks if an item exists in the library.
func (l *Library[T]) Exists(name string) bool {
	data, _ := l.ldr.ReadFile(l.prefix + "/" + name + l.ext())
	return data != nil
}

// LoadMap loads an item as a raw map[string]any (for dynamic access).
func (l *Library[T]) LoadMap(name string) (map[string]any, error) {
	data, err := l.ldr.ReadFile(l.prefix + "/" + name + l.ext())
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return marshal.UnmarshalMap(data, marshal.YAML)
}

// SaveMap writes a raw map as YAML with an optional header.
func (l *Library[T]) SaveMap(name string, m map[string]any, header string) error {
	wl, err := l.Writable()
	if err != nil {
		return err
	}
	data, err := marshal.Marshal(m, marshal.YAML)
	if err != nil {
		return fmt.Errorf("marshal %s/%s: %w", l.prefix, name, err)
	}
	return wl.WriteFile(l.prefix+"/"+name+l.ext(), append([]byte(header), data...))
}

// resolveLoader collapses a slice of loaders into one.
func resolveLoader(loaders []loader.Loader) loader.Loader {
	switch len(loaders) {
	case 0:
		panic("application: Library requires at least one Loader")
	case 1:
		return loaders[0]
	default:
		return loader.NewCompoundLoader(loaders...)
	}
}
