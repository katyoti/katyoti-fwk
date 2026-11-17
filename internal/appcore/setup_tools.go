// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// NewChromeToolDef returns a ToolDef for Chrome / Chromium.
// On Windows, running chrome.exe --version opens the UI, so the version is read
// from the version-named subdirectory inside the Application folder instead.
// On other platforms, LookPath + --version is used.
func NewChromeToolDef() ToolDef {
	return ToolDef{
		Name:         "chrome",
		ConfigKey:    "chrome",
		InstallSkill: "ktw-docs:/skill/install-chrome",
		// winget's Google.Chrome is the enterprise MSI, machine-wide only (no
		// installer for --scope user, 0x8A150010); Google.Chrome.EXE is
		// Chrome's own installer, which installs for the user alone. brew
		// installs the google-chrome cask by its name (no formula has it);
		// Debian packages chromium, Ubuntu chromium-browser.
		Recipes: []publish.InstallRecipe{
			{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "Google.Chrome.EXE"},
			{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "extras/googlechrome"},
			{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "google-chrome"},
			{Platform: publish.PlatformLinux, Kind: publish.RecipeApt, Package: "chromium"},
			{Platform: publish.PlatformLinux, Kind: publish.RecipeApt, Package: "chromium-browser"},
		},
		Check: func(_ context.Context) publish.CheckResult {
			if runtime.GOOS != "windows" {
				for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome"} {
					if p, err := exec.LookPath(name); err == nil {
						ver, _ := publish.CheckToolVersion(p, "--version")
						return publish.CheckResult{OK: true, FoundVersion: ver}
					}
				}
				return publish.CheckResult{}
			}
			// Windows: scan well-known Chrome install paths
			for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
				if base == "" {
					continue
				}
				p := filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe")
				if _, err := os.Stat(p); err != nil {
					continue
				}
				return publish.CheckResult{OK: true, FoundVersion: publish.ChromeVersionFromDir(filepath.Dir(p))}
			}
			return publish.CheckResult{}
		},
	}
}
