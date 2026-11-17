// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"strings"
)

// UserDocumentsDir is the person's documents folder — where what they author
// (a solution's project) goes, so they find it beside their other documents:
// the Documents known folder on Windows (wherever it is redirected, OneDrive
// included), $XDG_DOCUMENTS_DIR when set elsewhere, else ~/Documents.
//
// A process whose home is not the person's own (a test or a sandbox that sets
// USERPROFILE / HOME to a folder of its own) gets <that home>/Documents: what
// it writes stays in its world.
func UserDocumentsDir() string {
	home, _ := os.UserHomeDir()
	if d := knownDocumentsDir(home); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("XDG_DOCUMENTS_DIR")); d != "" && filepath.IsAbs(d) {
		return d
	}
	return filepath.Join(home, "Documents")
}
