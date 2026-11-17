// SPDX-License-Identifier: Apache-2.0

// Package marshal provides format-aware encoding and decoding of structured data.
//
// It dispatches between YAML, JSON, Frontmatter (Markdown with YAML front matter),
// and plain Text based on a [Format] value. Format detection from file extensions
// is provided by [FormatFor].
//
// New codecs can be registered at init time via [RegisterCodec].
//
// This package is a pure codec layer — no file I/O, no config processing,
// no variable expansion. Higher-level pipelines (config/) compose marshal
// with loaders and post-processing.
package marshal

import (
	"fmt"
	"strings"
	"sync"
)

// Format represents a data serialization format.
type Format string

const (
	YAML        Format = "yaml"
	JSON        Format = "json"
	Frontmatter Format = "frontmatter"
	Text        Format = "text"
)

// Codec defines a symmetric encoder/decoder for a specific format.
type Codec interface {
	Unmarshal(data []byte, target any) error
	Marshal(data any) ([]byte, error)
}

// registry maps Format → Codec and extension → Format.
var (
	mu     sync.RWMutex
	codecs = map[Format]Codec{}
	extMap = map[string]Format{}
)

func init() {
	// Built-in codecs.
	RegisterCodec(YAML, yamlCodec{}, "yaml", "yml")
	RegisterCodec(JSON, jsonCodec{}, "json")
	RegisterCodec(Frontmatter, frontmatterCodec{}, "md", "mdx")
	RegisterCodec(Text, textCodec{}, "txt")
}

// RegisterCodec registers a Codec for the given format and file extensions.
// Extensions are stored without a leading dot, lowercased.
// Panics if format is already registered (use at init time only).
func RegisterCodec(format Format, codec Codec, extensions ...string) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := codecs[format]; exists {
		panic(fmt.Sprintf("marshal: codec already registered for format %q", format))
	}
	codecs[format] = codec
	for _, ext := range extensions {
		ext = strings.TrimPrefix(ext, ".")
		ext = strings.ToLower(ext)
		extMap[ext] = format
	}
}

// FormatFor detects the format from an extension or path.
// Accepts bare extensions ("yaml", ".yaml") or full paths/filenames ("config/settings.yaml").
// If the input contains a dot, extracts the substring after the last dot.
// Returns ("", false) for unknown extensions.
func FormatFor(input string) (Format, bool) {
	// If the input contains a dot, extract from the last dot onward.
	if idx := strings.LastIndex(input, "."); idx >= 0 {
		input = input[idx+1:]
	}
	input = strings.ToLower(input)

	mu.RLock()
	defer mu.RUnlock()

	// Check extension map first (covers "yaml", "yml", "json", "md", "txt", etc.).
	if f, ok := extMap[input]; ok {
		return f, true
	}
	// Check direct format name (covers "frontmatter", "text", etc.).
	if _, ok := codecs[Format(input)]; ok {
		return Format(input), true
	}
	return "", false
}

// CodecFor returns the Codec for a format name, extension, or file path.
// Accepts Format constants ("yaml"), extensions (".yaml"), or paths ("config/settings.yaml").
// First tries a direct lookup by Format, then resolves via FormatFor.
// Returns (nil, error) for unknown formats.
func CodecFor(format Format) (Codec, error) {
	resolved, ok := FormatFor(string(format))
	if !ok {
		return nil, fmt.Errorf("marshal: unsupported format %q", format)
	}
	mu.RLock()
	c := codecs[resolved]
	mu.RUnlock()
	if c == nil {
		return nil, fmt.Errorf("marshal: no codec for format %q", resolved)
	}
	return c, nil
}

// Unmarshal decodes data into target.
// format accepts a Format constant ("yaml"), extension (".yaml"), or path ("config/settings.yaml").
func Unmarshal(data []byte, format Format, target any) error {
	c, err := CodecFor(format)
	if err != nil {
		return err
	}
	return c.Unmarshal(data, target)
}

// UnmarshalMap decodes data into map[string]any.
// format accepts a Format constant, extension, or path.
func UnmarshalMap(data []byte, format Format) (map[string]any, error) {
	var m map[string]any
	if err := Unmarshal(data, format, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Marshal encodes data into bytes.
// format accepts a Format constant ("yaml"), extension (".yaml"), or path ("config/settings.yaml").
func Marshal(data any, format Format) ([]byte, error) {
	c, err := CodecFor(format)
	if err != nil {
		return nil, err
	}
	return c.Marshal(data)
}

// KnownFormats returns all registered file extensions mapped to their Format.
func KnownFormats() map[string]Format {
	mu.RLock()
	defer mu.RUnlock()
	cp := make(map[string]Format, len(extMap))
	for k, v := range extMap {
		cp[k] = v
	}
	return cp
}
