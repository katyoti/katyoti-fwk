// SPDX-License-Identifier: Apache-2.0

//go:build windows

package cenv

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// knownFolder resolves a Windows known folder; a test replaces it.
var knownFolder = func(id *windows.KNOWNFOLDERID) (string, error) { return windows.KnownFolderPath(id, 0) }

// knownDocumentsDir is the Documents known folder, when home is the person's
// profile; "" when home was redirected (a test, a sandbox — a Documents folder
// redirected to OneDrive is an absolute path the redirect does not follow) or
// the folder cannot be resolved.
func knownDocumentsDir(home string) string {
	profile, err := knownFolder(windows.FOLDERID_Profile)
	if err != nil || !strings.EqualFold(filepath.Clean(profile), filepath.Clean(home)) {
		return ""
	}
	docs, err := knownFolder(windows.FOLDERID_Documents)
	if err != nil {
		return ""
	}
	return docs
}
