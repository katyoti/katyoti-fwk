// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// ItemParser transforms raw file bytes into ConfigItems.
// This is Stage 2 of the provider pipeline: []byte → []ConfigItem.
// source is the file path (used for Source field, name derivation, and format detection).
type ItemParser func(source string, data []byte) ([]ConfigItem, error)

// SingleItemParser returns an ItemParser that treats the entire file as
// one ConfigItem. Format is detected from the source filename.
//
// Use when 1 file = 1 config object (e.g., docs.yaml, voices.yaml).
func SingleItemParser() ItemParser {
	return func(source string, data []byte) ([]ConfigItem, error) {
		m, err := unmarshalMap(source, data)
		if err != nil {
			return nil, err
		}
		return []ConfigItem{{
			Data:   m,
			Source: source,
		}}, nil
	}
}

// ListItemParser returns an ItemParser that parses the file, extracts the
// sequence under listKey, and produces a ConfigItem per entry.
//
// Use when 1 file has a typed list (e.g., plugins.yaml `plugins:` key).
// File-level metadata outside the list key (e.g., `defaults:`) is ignored —
// that's orchestration consumed by the caller separately.
func ListItemParser(listKey string) ItemParser {
	return func(source string, data []byte) ([]ConfigItem, error) {
		m, err := unmarshalMap(source, data)
		if err != nil {
			return nil, err
		}

		listVal, ok := m[listKey]
		if !ok {
			return nil, nil // key not present — no items
		}
		list, ok := listVal.([]any)
		if !ok {
			return nil, fmt.Errorf("key %q in %s is not a sequence", listKey, source)
		}

		var items []ConfigItem
		for _, entry := range list {
			em, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("entry under %q in %s is not a mapping", listKey, source)
			}
			items = append(items, ConfigItem{
				Data:   em,
				Source: source,
			})
		}
		return items, nil
	}
}

// unmarshalMap parses data into map[string]any, detecting format from source.
// Falls back to YAML for unknown extensions.
func unmarshalMap(source string, data []byte) (map[string]any, error) {
	format := marshal.YAML // default
	if f, ok := marshal.FormatFor(source); ok {
		format = f
	}
	m, err := marshal.UnmarshalMap(data, format)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", source, err)
	}
	return m, nil
}
