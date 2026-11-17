// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"io/fs"
)

// SingleFileProvider reads one file and applies an ItemParser.
// This is Stage 1 (read) for single-file sources.
//
// Combine with standard parsers:
//   - SingleItemParser: 1 file = 1 ConfigItem (e.g., docs.yaml)
//   - ListItemParser: 1 file with list key = N ConfigItems (e.g., plugins.yaml)
type SingleFileProvider struct {
	// FS is the filesystem to read from (os.DirFS, embed.FS, fstest.MapFS).
	FS fs.FS

	// Path is the file path relative to FS.
	Path string

	// Domain is the domain this provider serves. ProvideConfigFor returns
	// nil for any other domain.
	Domain string

	// Parser transforms the file bytes into ConfigItems (Stage 2).
	Parser ItemParser
}

func (p *SingleFileProvider) ProvideConfigFor(domain string) ([]ConfigItem, error) {
	if p.Domain != "" && domain != p.Domain {
		return nil, nil
	}

	data, err := fs.ReadFile(p.FS, p.Path)
	if err != nil {
		return nil, fmt.Errorf("provider: read %s: %w", p.Path, err)
	}

	return p.Parser(p.Path, data)
}
