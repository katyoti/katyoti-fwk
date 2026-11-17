// SPDX-License-Identifier: Apache-2.0

package local

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// Python's winget install runs unattended: its launcher for the person only
// (the all-users default asks for elevation, a prompt an unattended install
// waits on forever) and python on their PATH, where the checks find it.
func TestPythonsWingetInstallAsksNoElevation(t *testing.T) {
	for _, d := range ToolDefs() {
		if d.Name != "python" {
			continue
		}
		for _, r := range d.Recipes {
			if r.Kind == publish.RecipeWinget {
				if !strings.Contains(r.Custom, "InstallLauncherAllUsers=0") || !strings.Contains(r.Custom, "PrependPath=1") {
					t.Errorf("python's winget recipe passes %q, want the launcher for the person only and python on PATH", r.Custom)
				}
				if err := r.Validate(); err != nil {
					t.Error(err)
				}
				return
			}
		}
	}
	t.Fatal("no winget recipe for python")
}

// On Windows Python is installed first from its full build as a NuGet
// package, verified by its sha256: no MSI, whose transaction never completes
// in a fresh Windows Sandbox.
func TestPythonOnWindowsIsFirstAVerifiedDownloadNotAnMSI(t *testing.T) {
	for _, d := range ToolDefs() {
		if d.Name != "python" {
			continue
		}
		for _, r := range d.Recipes {
			if r.Platform != publish.PlatformWindows {
				continue
			}
			if r.Kind != publish.RecipeDownload || !strings.HasSuffix(r.URL, ".nupkg") {
				t.Fatalf("python's first Windows recipe is %s %s, want the NuGet package's download", r.Kind, r.Package+r.URL)
			}
			if err := r.Validate(); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("no Windows recipe for python")
}
