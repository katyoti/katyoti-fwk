// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"io/fs"
	"path"
)

// FolderProvider scans a directory and applies an ItemParser to each
// matching file. This is Stage 1 (read) for folder-based sources.
//
// Combine with standard parsers:
//   - SingleItemParser: each file = 1 ConfigItem (e.g., skills/)
//   - ListItemParser: each file contains a list = N ConfigItems per file
type FolderProvider struct {
	// FS is the filesystem to read from.
	FS fs.FS

	// Dir is the directory path relative to FS.
	Dir string

	// Domain is the domain this provider serves.
	Domain string

	// Glob is the file pattern to match (default: "*.yaml").
	Glob string

	// Parser transforms each file's bytes into ConfigItems (Stage 2).
	Parser ItemParser
}

func (p *FolderProvider) ProvideConfigFor(domain string) ([]ConfigItem, error) {
	if p.Domain != "" && domain != p.Domain {
		return nil, nil
	}

	glob := p.Glob
	if glob == "" {
		glob = "*.yaml"
	}

	pattern := path.Join(p.Dir, glob)
	matches, err := fs.Glob(p.FS, pattern)
	if err != nil {
		return nil, fmt.Errorf("folder provider %q: glob %s: %w", p.Dir, pattern, err)
	}

	var all []ConfigItem
	for _, match := range matches {
		data, err := fs.ReadFile(p.FS, match)
		if err != nil {
			return nil, fmt.Errorf("folder provider %q: read %s: %w", p.Dir, match, err)
		}

		items, err := p.Parser(match, data)
		if err != nil {
			return nil, fmt.Errorf("folder provider %q: parse %s: %w", p.Dir, match, err)
		}
		all = append(all, items...)
	}
	return all, nil
}
