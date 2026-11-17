// SPDX-License-Identifier: Apache-2.0

//go:build windows

package cenv

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// The person's Documents redirected elsewhere (OneDrive) is theirs: a process
// whose home is a folder of its own never writes there.
func TestUserDocumentsDirIgnoresTheKnownFolderOfAnotherHome(t *testing.T) {
	person := filepath.Join(t.TempDir(), "person")
	onedrive := filepath.Join(t.TempDir(), "OneDrive", "Documents")
	prev := knownFolder
	knownFolder = func(id *windows.KNOWNFOLDERID) (string, error) {
		if id == windows.FOLDERID_Profile {
			return person, nil
		}
		return onedrive, nil
	}
	t.Cleanup(func() { knownFolder = prev })

	t.Setenv("USERPROFILE", person)
	if got := UserDocumentsDir(); got != onedrive {
		t.Errorf("the person's own home: %q, want the known folder %q", got, onedrive)
	}
	sandbox := t.TempDir()
	t.Setenv("USERPROFILE", sandbox)
	if got, want := UserDocumentsDir(), filepath.Join(sandbox, "Documents"); got != want {
		t.Errorf("a redirected home: %q, want %q", got, want)
	}
}
