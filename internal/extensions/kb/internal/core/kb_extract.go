// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/bundling"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/extraction"
)

func (s *kbService) Extract(ctx context.Context, in kbapi.ExtractInput) (kbapi.ExtractOutput, error) {
	if in.Path == "" {
		return kbapi.ExtractOutput{}, fmt.Errorf("path is required")
	}

	absPath, err := application.ProjectPath(in.Path)
	if err != nil {
		return kbapi.ExtractOutput{}, fmt.Errorf("resolve path: %w", err)
	}

	kbCfg, err := LoadKnowledgeBase(absPath)
	if err != nil {
		return kbapi.ExtractOutput{}, err
	}

	router := extraction.NewRouter()

	var out kbapi.ExtractOutput

	// Detect changes across all sources
	for _, sourceName := range kbCfg.Sources {
		changes, err := bundling.ScanSource(absPath, sourceName)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("scan %s: %v", sourceName, err))
			continue
		}

		for _, change := range changes {
			// An item's data beside its text: the text carries the item, read
			// once - a bundle the data had alone leaves the base.
			if extraction.HasItemText(change.FullPath) {
				if meta, err := bundling.LoadMetadata(absPath, change.Slug); err == nil && meta.Status != "stale" {
					meta.Status = "stale"
					_ = bundling.SaveMetadata(absPath, meta)
				}
				out.Skipped++
				continue
			}
			switch change.Action {
			case bundling.ActionUnchanged:
				out.Skipped++
			case bundling.ActionNew:
				if err := s.extractFile(ctx, absPath, kbCfg, router, change); err != nil {
					out.Failed++
					out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", change.RelativePath, err))
				} else {
					out.New++
				}
			case bundling.ActionModified:
				if err := s.extractFile(ctx, absPath, kbCfg, router, change); err != nil {
					out.Failed++
					out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", change.RelativePath, err))
				} else {
					out.Updated++
				}
			}
		}
	}

	// Detect deleted files
	deleted, err := bundling.ScanForDeleted(absPath, kbCfg.Sources)
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("scan deleted: %v", err))
	}
	for _, change := range deleted {
		slog.Debug("bundle deleted (source removed)", "slug", change.Slug)
		if meta, err := bundling.LoadMetadata(absPath, change.Slug); err == nil {
			meta.Status = "stale"
			_ = bundling.SaveMetadata(absPath, meta)
		}
	}

	return out, nil
}

// extractFile runs extraction for a single source file and creates/updates the bundle.
func (s *kbService) extractFile(ctx context.Context, kbRoot string, kbCfg *KnowledgeBaseConfig, router *extraction.Router, change bundling.FileChange) error {
	start := time.Now()

	// Compute file hash if not already done
	fileHash := change.FileHash
	if fileHash == "" {
		var err error
		fileHash, err = bundling.HashFile(change.FullPath)
		if err != nil {
			return fmt.Errorf("hash file: %w", err)
		}
	}

	// Detect MIME type from extension
	mimeType := mime.TypeByExtension(filepath.Ext(change.FullPath))

	bundleDir := bundling.BundleDir(kbRoot, change.Slug)

	// Clean existing bundle content (keep dir, remove old content files)
	cleanBundleContent(bundleDir)

	// Create bundle dir + attachments dir
	if err := os.MkdirAll(filepath.Join(bundleDir, "attachments"), 0o755); err != nil {
		return fmt.Errorf("create bundle dir: %w", err)
	}

	// Create pending metadata
	meta := &bundling.BundleMetadata{
		Slug:   change.Slug,
		Status: "pending",
		Source: bundling.SourceMeta{
			Path:     change.SourceName + "/" + change.RelativePath,
			Hash:     fileHash,
			Size:     change.FileSize,
			Mtime:    change.FileMtime,
			MimeType: mimeType,
		},
	}
	if err := bundling.SaveMetadata(kbRoot, meta); err != nil {
		return fmt.Errorf("save pending metadata: %w", err)
	}

	// Run extraction via router
	extractorName, result, err := router.Extract(ctx, kbCfg.Extraction.Profile, change.FullPath, bundleDir, nil, nil)
	if err != nil {
		// Mark as failed
		meta.Status = "failed"
		meta.Extraction = bundling.ExtractionMeta{
			Profile:     kbCfg.Extraction.Profile,
			Extractor:   extractorName,
			ExtractedAt: time.Now(),
			DurationMs:  time.Since(start).Milliseconds(),
			Error:       err.Error(),
		}
		_ = bundling.SaveMetadata(kbRoot, meta)
		return err
	}

	// Inject frontmatter into each content file
	for _, cf := range result.ContentFiles {
		if err := injectFrontmatter(bundleDir, change.Slug, cf); err != nil {
			slog.Debug("frontmatter injection failed", "file", cf.File, "error", err)
		}
	}

	// Update metadata with extraction result
	meta.Status = "extracted"
	meta.Extraction = bundling.ExtractionMeta{
		Profile:     kbCfg.Extraction.Profile,
		Extractor:   extractorName,
		ExtractedAt: time.Now(),
		DurationMs:  time.Since(start).Milliseconds(),
	}
	meta.TotalParts = result.TotalParts
	for _, cf := range result.ContentFiles {
		meta.Content = append(meta.Content, bundling.ContentEntry{
			File:  cf.File,
			Part:  cf.Part,
			Index: cf.Index,
		})
	}
	for _, att := range result.Attachments {
		meta.Attachments = append(meta.Attachments, bundling.AttachEntry{
			File:         att.File,
			Type:         att.Type,
			ReferencedBy: att.ReferencedBy,
		})
	}

	if err := bundling.SaveMetadata(kbRoot, meta); err != nil {
		return fmt.Errorf("save extracted metadata: %w", err)
	}

	return nil
}

// injectFrontmatter adds minimal YAML frontmatter to a content file.
// Frontmatter: slug, part, part_index, part_total, attachments.
func injectFrontmatter(bundleDir, slug string, cf kbapi.ContentFile) error {
	filePath := filepath.Join(bundleDir, cf.File)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var fm string
	if cf.Index == 0 {
		// Single-part
		fm = fmt.Sprintf("---\nslug: %s\npart: %s\n---\n\n", slug, cf.Part)
	} else {
		fm = fmt.Sprintf("---\nslug: %s\npart: %s\npart_index: %d\n---\n\n", slug, cf.Part, cf.Index)
	}

	return os.WriteFile(filePath, append([]byte(fm), content...), 0o644)
}

// cleanBundleContent removes old content*.md files and attachments/ from a bundle dir.
func cleanBundleContent(bundleDir string) {
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if name == "metadata.yaml" {
			continue
		}
		path := filepath.Join(bundleDir, name)
		if e.IsDir() {
			_ = os.RemoveAll(path)
		} else {
			_ = os.Remove(path)
		}
	}
}
