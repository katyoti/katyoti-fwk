// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// LoaderProvider reads config items from a [loader.Loader] using a glob pattern.
// It combines the Loader's multi-source resolution (embedded + filesystem overlay)
// with format-aware parsing via [ItemParser].
//
// Combine with standard parsers:
//   - SingleItemParser: each matched file = 1 ConfigItem
//   - ListItemParser: each matched file contains a list = N ConfigItems per file
type LoaderProvider struct {
	// Loader is the file source (FSLoader, CompoundLoader, etc.).
	Loader loader.Loader

	// Pattern is the glob passed to Loader.List() (e.g., "plugins/*.yaml").
	Pattern string

	// Domain is the domain this provider serves. ProvideConfigFor returns
	// nil for any other domain.
	Domain string

	// Parser transforms each file's bytes into ConfigItems (Stage 2).
	Parser ItemParser
}

func (p *LoaderProvider) ProvideConfigFor(domain string) ([]ConfigItem, error) {
	if p.Domain != "" && domain != p.Domain {
		return nil, nil
	}

	matches := p.Loader.List(p.Pattern)
	if len(matches) == 0 {
		return nil, nil
	}

	var all []ConfigItem
	for _, path := range matches {
		data, err := p.Loader.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("loader provider %q: read %s: %w", p.Pattern, path, err)
		}
		if data == nil {
			continue // file vanished between List and ReadFile
		}

		items, err := p.Parser(path, data)
		if err != nil {
			return nil, fmt.Errorf("loader provider %q: parse %s: %w", p.Pattern, path, err)
		}
		all = append(all, items...)
	}
	return all, nil
}
