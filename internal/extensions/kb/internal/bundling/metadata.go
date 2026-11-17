// SPDX-License-Identifier: Apache-2.0

package bundling

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// BundleMetadata is the on-disk metadata.yaml for a bundle.
type BundleMetadata struct {
	Slug        string         `yaml:"slug"`
	Status      string         `yaml:"status"` // pending, extracted, failed, stale
	Source      SourceMeta     `yaml:"source"`
	Extraction  ExtractionMeta `yaml:"extraction,omitempty"`
	Content     []ContentEntry `yaml:"content,omitempty"`
	TotalParts  int            `yaml:"total_parts,omitempty"`
	Attachments []AttachEntry  `yaml:"attachments,omitempty"`
}

// SourceMeta describes the source document.
type SourceMeta struct {
	Path     string      `yaml:"path"` // <source_name>/<relative>
	Hash     string      `yaml:"hash"` // sha256:...
	Size     int64       `yaml:"size"`
	Mtime    time.Time   `yaml:"mtime"`
	MimeType string      `yaml:"mime_type"`
	Origin   *OriginMeta `yaml:"origin,omitempty"` // absent for local-to-local
}

// OriginMeta holds remote origin info (only for remote sources).
type OriginMeta struct {
	SyncSource string `yaml:"sync_source"`
	RemotePath string `yaml:"remote_path"`
}

// ExtractionMeta describes the extraction that produced this bundle.
type ExtractionMeta struct {
	Profile     string    `yaml:"profile,omitempty"`
	Extractor   string    `yaml:"extractor,omitempty"`
	ExtractedAt time.Time `yaml:"extracted_at,omitempty"`
	DurationMs  int64     `yaml:"duration_ms,omitempty"`
	Error       string    `yaml:"error,omitempty"`
}

// ContentEntry describes a produced content file.
type ContentEntry struct {
	File  string `yaml:"file"`
	Part  string `yaml:"part"`
	Index int    `yaml:"index"`
}

// AttachEntry describes a produced attachment.
type AttachEntry struct {
	File         string `yaml:"file"`
	Type         string `yaml:"type"`
	ReferencedBy string `yaml:"referenced_by,omitempty"`
}

// BundleDir returns the absolute path to a bundle directory.
func BundleDir(kbRoot, slug string) string {
	return filepath.Join(kbRoot, ".bundles", slug)
}

// MetadataPath returns the absolute path to a bundle's metadata.yaml.
func MetadataPath(kbRoot, slug string) string {
	return filepath.Join(BundleDir(kbRoot, slug), "metadata.yaml")
}

// LoadMetadata reads and parses a bundle's metadata.yaml.
func LoadMetadata(kbRoot, slug string) (*BundleMetadata, error) {
	path := MetadataPath(kbRoot, slug)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta BundleMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata %s: %w", slug, err)
	}
	return &meta, nil
}

// SaveMetadata writes a bundle's metadata.yaml.
func SaveMetadata(kbRoot string, meta *BundleMetadata) error {
	dir := BundleDir(kbRoot, meta.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create bundle dir: %w", err)
	}
	data, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(MetadataPath(kbRoot, meta.Slug), data, 0o644)
}

// MetadataExists checks if a bundle's metadata.yaml exists.
func MetadataExists(kbRoot, slug string) bool {
	_, err := os.Stat(MetadataPath(kbRoot, slug))
	return err == nil
}
