// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

func (s *kbService) Create(ctx context.Context, in kbapi.CreateInput) (kbapi.CreateOutput, error) {
	if in.Path == "" {
		return kbapi.CreateOutput{}, fmt.Errorf("path is required")
	}

	absPath, err := application.ProjectPath(in.Path)
	if err != nil {
		return kbapi.CreateOutput{}, fmt.Errorf("resolve path: %w", err)
	}

	// Create directory structure
	dirs := []string{
		absPath,
		filepath.Join(absPath, "sources"),
		filepath.Join(absPath, ".bundles"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return kbapi.CreateOutput{}, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// Write template KnowledgeBase.yaml
	kbFile := filepath.Join(absPath, "KnowledgeBase.yaml")
	if _, err := os.Stat(kbFile); err == nil {
		return kbapi.CreateOutput{}, fmt.Errorf("KnowledgeBase.yaml already exists at %s", absPath)
	}

	name := filepath.Base(absPath)
	if err := os.WriteFile(kbFile, []byte(kbTemplate(name)), 0o644); err != nil {
		return kbapi.CreateOutput{}, fmt.Errorf("write KnowledgeBase.yaml: %w", err)
	}

	// Write example source config
	exampleSource := filepath.Join(absPath, "sources", "example-local.yaml")
	if err := os.WriteFile(exampleSource, []byte(sourceTemplate()), 0o644); err != nil {
		return kbapi.CreateOutput{}, fmt.Errorf("write example source: %w", err)
	}

	return kbapi.CreateOutput{
		Path:    absPath,
		Message: fmt.Sprintf("Knowledge base '%s' created at %s", name, absPath),
	}, nil
}

func kbTemplate(name string) string {
	return `# KnowledgeBase configuration
# See modules/kb/concepts.md for full reference.

name: ` + name + `
description: ""

# Sources — each entry matches a file in sources/<name>.yaml
# All sources go through ktw-sync (including local folders via rclone local backend).
sources:
  - example-local                            # matches sources/example-local.yaml

# Extraction — how documents are converted to markdown knowledge bundles
extraction:
  profile: fast                              # profile name from KnowledgeExtraction.yaml
  concurrency: 4                             # max parallel extractions
  on_missing_tool: fail                      # fail | warn | skip
  limits:
    max_file_size: 100MB                     # skip files larger than this
    max_pages: 500                           # PDF/PPTX: extract first N pages
    max_rows: 10000                          # XLSX: first N rows per sheet

# Indexing — how knowledge is chunked, embedded, and stored
indexing:
  backend: chromem-go                        # vector DB strategy: chromem-go (embedded), chromadb, qdrant, sqlite-vec
  config:
    collection: ` + name + `
  embedding:
    profile: ""                              # a profile of the ktw-kb EmbeddingProfiles.yaml; empty = its default
  chunking:
    strategy: markdown-heading               # chunker: fixed-size, markdown-heading, markdown-paragraph, sentence
    config:
      min_size: 200
      max_size: 1500
`
}

func sourceTemplate() string {
	return `# Source configuration — defines how to sync documents into this knowledge base.
# Each source file declares an adapter (default: rclone) and adapter-specific config.
#
# For rclone adapters, see: https://rclone.org/overview/
# Supported backends: local, drive, s3, onedrive, sftp, webdav, ftp

# --- ktw-sync adapter (default) ---
adapter: ktw-sync

# Workspace — rclone backend configuration
workspace:
  type: local                                # rclone backend: local, drive, s3, onedrive, sftp, webdav, ftp
  path: /path/to/your/documents              # local path (for type: local)

# Job settings — how to sync
job:
  mode: mirror                               # pull, push, mirror, bisync
  filters:                                   # include only these patterns
    - "*.pdf"
    - "*.docx"
    - "*.pptx"
    - "*.xlsx"
    - "*.md"
    - "*.txt"
    - "*.html"
  exclude:                                   # exclude these patterns
    - "**/.git/**"
    - "**/node_modules/**"
    - "**/.DS_Store"
  options:
    transfers: 4                             # parallel transfers

# --- Other sidecar adapters (uncomment to use) ---
# adapter: ktw-example
# config:
#   workspace: my-workspace
#   export:
#     - type: applications
#       format: yaml
#       include_relations: true
`
}
