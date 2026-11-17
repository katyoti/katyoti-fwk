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

func (s *kbService) Status(ctx context.Context, in kbapi.StatusInput) (kbapi.StatusOutput, error) {
	if in.Path == "" {
		return kbapi.StatusOutput{}, fmt.Errorf("kb_path is required")
	}

	absPath, err := application.ProjectPath(in.Path)
	if err != nil {
		return kbapi.StatusOutput{}, fmt.Errorf("resolve path: %w", err)
	}

	// Load KB config
	kbCfg, err := LoadKnowledgeBase(absPath)
	if err != nil {
		return kbapi.StatusOutput{}, err
	}

	// Load sources
	sources, err := LoadAllSources(absPath, kbCfg)
	if err != nil {
		return kbapi.StatusOutput{}, err
	}

	// Build source status
	var srcStatuses []kbapi.SourceStatus
	for name, src := range sources {
		fileCount := countFiles(filepath.Join(absPath, name))
		srcStatuses = append(srcStatuses, kbapi.SourceStatus{
			Name:    name,
			Adapter: src.ResolvedAdapter(),
			Files:   fileCount,
		})
	}

	// Count bundles by status
	bundles := countBundles(filepath.Join(absPath, ".bundles"))

	// Index status
	collection := ""
	embeddingModel := ""
	if col, ok := kbCfg.Indexing.Config["collection"].(string); ok {
		collection = col
	}
	// The model of the profile the knowledge base names; the legacy config
	// field when no embedder can be built (its key unset, say) - status
	// never fails on it.
	if e, err := s.embedderFor(ctx, kbCfg); err == nil {
		embeddingModel = e.Model()
	} else if model, ok := kbCfg.Indexing.Embedding.Config["model"].(string); ok {
		embeddingModel = model
	}

	return kbapi.StatusOutput{
		Name:    kbCfg.Name,
		Sources: srcStatuses,
		Bundles: bundles,
		Index: kbapi.IndexStatus{
			Backend:        kbCfg.Indexing.Backend,
			Collection:     collection,
			EmbeddingModel: embeddingModel,
		},
		ToolsAvailable: map[string]bool{}, // TODO: check tool availability
	}, nil
}

// countFiles counts regular files recursively in a directory.
func countFiles(dir string) int {
	count := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	return count
}

// countBundles scans .bundles/*/metadata.yaml and counts by status.
func countBundles(bundlesDir string) kbapi.BundleCounts {
	var counts kbapi.BundleCounts
	entries, err := os.ReadDir(bundlesDir)
	if err != nil {
		return counts
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(bundlesDir, entry.Name(), "metadata.yaml")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		// Quick status extraction without full YAML parse
		status := extractYAMLField(data, "status")
		switch status {
		case "extracted":
			counts.Extracted++
		case "failed":
			counts.Failed++
		case "stale":
			counts.Stale++
		case "pending":
			counts.Pending++
		}
	}
	return counts
}

// extractYAMLField does a quick line scan for a top-level "key: value" field.
func extractYAMLField(data []byte, key string) string {
	prefix := key + ": "
	for _, line := range splitLines(data) {
		if len(line) > len(prefix) && string(line[:len(prefix)]) == prefix {
			return string(line[len(prefix):])
		}
	}
	return ""
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			line := data[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
