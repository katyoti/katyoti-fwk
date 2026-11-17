// SPDX-License-Identifier: Apache-2.0

// Package bundling handles bundle lifecycle: slug generation, metadata.yaml, change detection.
package bundling

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Slug computes the deterministic bundle slug for a source-relative path.
// Format: <cleaned_filename>-<hash8>.<original_ext>
// Hash input: <source_name>/<relative_path> (source name always included).
func Slug(sourceName, relativePath string) string {
	// Hash input includes source name for collision safety across sources
	hashInput := sourceName + "/" + relativePath
	hash := sha256.Sum256([]byte(hashInput))
	hash8 := fmt.Sprintf("%x", hash[:4]) // first 4 bytes = 8 hex chars

	base := filepath.Base(relativePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	cleaned := cleanFilename(name)
	if cleaned == "" {
		cleaned = "file"
	}

	return cleaned + "-" + hash8 + ext
}

// cleanFilename normalizes a filename for use as a slug component:
// lowercase, strip accents, replace non-alphanumeric with -, collapse runs.
func cleanFilename(name string) string {
	// Strip accents (NFD decompose, remove combining marks, NFC recompose)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, err := transform.String(t, name)
	if err != nil {
		result = name
	}

	// Lowercase
	result = strings.ToLower(result)

	// Replace non-alphanumeric with hyphen
	re := regexp.MustCompile(`[^a-z0-9]+`)
	result = re.ReplaceAllString(result, "-")

	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")

	return result
}
