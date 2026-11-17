// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// knownExts is the ordered list of extensions tried by name-based loading.
var knownExts = []string{".yaml", ".yml", ".json"}

// UseConfig loads a config file for T from the application's config directory.
// The file name equals T's type name: Plugins → "Plugins.yaml", Settings → "Settings.yaml".
// Missing file returns zero value (no error).
func UseConfig[T any](cs *ConfigService) (T, error) {
	name := ConfigName[T]()
	item := new(T)
	cs.LoadConfig(name, item)
	return *item, nil
}

// LoadFrom loads a named config from a loader into T.
// The name is derived from T's type name, with optional path segments prepended.
// Tries .yaml, .yml, .json extensions in order.
// Missing file is not an error — returns the zero value of T.
func LoadFrom[T any](l loader.Loader, paths ...string) (T, error) {
	name := ConfigName[T]()
	if len(paths) > 0 {
		parts := append(paths, name)
		name = filepath.Join(parts...)
	}
	item := new(T)
	err := LoadInto(l, name, item)
	return *item, err
}

// LoadByPath loads a config from a loader by explicit path.
// Detects format from path extension.
func LoadByPath[T any](l loader.Loader, path string) (T, error) {
	var zero T
	data, err := l.ReadFile(path)
	if err != nil {
		return zero, err
	}
	if data == nil {
		return zero, nil
	}
	format, ok := marshal.FormatFor(path)
	if !ok {
		return zero, fmt.Errorf("config: unknown format for %q", path)
	}
	return Decode[T](data, format, path)
}

// LoadFromBytes decodes bytes in the given format into T.
func LoadFromBytes[T any](data []byte, format marshal.Format) (T, error) {
	return Decode[T](data, format, "bytes")
}

// LoadFromFile loads a file by absolute path, detects format from extension.
func LoadFromFile[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	format, ok := marshal.FormatFor(path)
	if !ok {
		return zero, fmt.Errorf("config: unknown format for %q", path)
	}
	return Decode[T](data, format, path)
}

// LoadInto loads a named config from a loader into an existing target (pointer).
// Tries .yaml, .yml, .json extensions.
// Missing file is not an error — target keeps its defaults.
func LoadInto(l loader.Loader, name string, target any) error {
	for _, ext := range knownExts {
		fullPath := name + ext
		data, err := l.ReadFile(fullPath)
		if err != nil {
			return err
		}
		if data != nil {
			format, _ := marshal.FormatFor(ext)
			if err := DecodeBytes(data, format, target); err != nil {
				return fmt.Errorf("config %q: %w", fullPath, err)
			}
			return nil
		}
	}
	return nil
}

// LoadFileInto reads the exact path from l and decodes it into target (pointer).
// Detects format from path extension.
// Missing file is not an error — target keeps its defaults.
func LoadFileInto(l loader.Loader, path string, target any) error {
	data, err := l.ReadFile(path)
	if err != nil || data == nil {
		return err
	}
	format, ok := marshal.FormatFor(path)
	if !ok {
		return fmt.Errorf("config: unknown format for %q", path)
	}
	if err := DecodeBytes(data, format, target); err != nil {
		return fmt.Errorf("config %q: %w", path, err)
	}
	return nil
}
