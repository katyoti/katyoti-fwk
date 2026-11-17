// SPDX-License-Identifier: Apache-2.0

package bundling

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileChange describes a detected change for a source file.
type FileChange struct {
	SourceName   string // e.g. "gdrive-specs"
	RelativePath string // e.g. "Product/Architecture/overview.pdf"
	FullPath     string // absolute path to the file
	Slug         string // deterministic slug
	Action       ChangeAction
	FileSize     int64
	FileMtime    time.Time
	FileHash     string // sha256:... (computed only when needed)
}

// ChangeAction describes what happened to a source file.
type ChangeAction int

const (
	ActionNew       ChangeAction = iota // no bundle exists
	ActionModified                      // bundle exists but source changed
	ActionUnchanged                     // bundle exists and source matches
	ActionDeleted                       // bundle exists but source file gone
)

func (a ChangeAction) String() string {
	switch a {
	case ActionNew:
		return "new"
	case ActionModified:
		return "modified"
	case ActionUnchanged:
		return "unchanged"
	case ActionDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// ScanSource walks a source folder and detects changes against existing bundles.
// Returns changes grouped by action. Uses fast-path: size+mtime first, hash only if changed.
func ScanSource(kbRoot, sourceName string) ([]FileChange, error) {
	sourceDir := filepath.Join(kbRoot, sourceName)
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil, nil // source not synced yet
	}

	var changes []FileChange

	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(sourceDir, path)
		relPath = filepath.ToSlash(relPath) // normalize to forward slashes
		slug := Slug(sourceName, relPath)

		info, err := d.Info()
		if err != nil {
			return nil
		}

		change := FileChange{
			SourceName:   sourceName,
			RelativePath: relPath,
			FullPath:     path,
			Slug:         slug,
			FileSize:     info.Size(),
			FileMtime:    info.ModTime(),
		}

		// Check if bundle exists
		meta, loadErr := LoadMetadata(kbRoot, slug)
		if loadErr != nil {
			// No metadata = new file
			change.Action = ActionNew
			changes = append(changes, change)
			return nil
		}

		// Fast-path: compare size + mtime
		if meta.Source.Size == info.Size() && meta.Source.Mtime.Equal(info.ModTime()) {
			change.Action = ActionUnchanged
			changes = append(changes, change)
			return nil
		}

		// Size or mtime changed — compute hash to confirm
		hash, err := HashFile(path)
		if err != nil {
			return nil // skip files we can't hash
		}
		change.FileHash = hash

		if meta.Source.Hash == hash {
			// Content unchanged, just mtime changed — update mtime in metadata
			meta.Source.Mtime = info.ModTime()
			_ = SaveMetadata(kbRoot, meta)
			change.Action = ActionUnchanged
		} else {
			change.Action = ActionModified
		}

		changes = append(changes, change)
		return nil
	})

	return changes, err
}

// ScanForDeleted finds bundles whose source files no longer exist.
func ScanForDeleted(kbRoot string, sourceNames []string) ([]FileChange, error) {
	bundlesDir := filepath.Join(kbRoot, ".bundles")
	entries, err := os.ReadDir(bundlesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var deleted []FileChange
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		meta, err := LoadMetadata(kbRoot, slug)
		if err != nil {
			continue
		}

		// Check if the source file still exists
		sourcePath := meta.Source.Path
		parts := strings.SplitN(sourcePath, "/", 2)
		if len(parts) != 2 {
			continue
		}
		sourceName := parts[0]

		// Only check sources that are in the KB config
		found := false
		for _, sn := range sourceNames {
			if sn == sourceName {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		fullPath := filepath.Join(kbRoot, sourceName, parts[1])
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			deleted = append(deleted, FileChange{
				SourceName:   sourceName,
				RelativePath: parts[1],
				Slug:         slug,
				Action:       ActionDeleted,
			})
		}
	}

	return deleted, nil
}

// HashFile computes the SHA256 hash of a file.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}
