// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"path/filepath"
	"testing"
)

// A process whose home is a folder of its own (a test, a sandbox) gets that
// home's Documents, never the person's.
func TestUserDocumentsDirStaysInARedirectedHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DOCUMENTS_DIR", "")
	if got, want := UserDocumentsDir(), filepath.Join(home, "Documents"); got != want {
		t.Fatalf("UserDocumentsDir() = %q, want %q", got, want)
	}
}
