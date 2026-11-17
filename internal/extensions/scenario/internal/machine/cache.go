// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// THE PREPARATION CACHE — what a Windows Sandbox is
// prepared with (winget's App Installer and its dependencies, the Visual C++
// runtime) is fetched once on the machine running the tool, kept in a folder
// beside the runs, refreshed when stale, and mapped read-only into each
// Sandbox: its preparation installs offline in seconds instead of minutes. The
// proof's own stages still install from the network, as a person does.

// SandboxCache is the cache as the Sandbox sees it.
const SandboxCache = `C:\Proof\cache`

// CacheDir is the cache folder name, beside the cases' run folders.
const CacheDir = "sandbox-cache"

// cacheMaxAge is how long a cached file is kept before it is fetched again.
const cacheMaxAge = 30 * 24 * time.Hour

// cacheItem is one file of the cache and where it comes from.
type cacheItem struct{ Name, URL string }

// Cache files, by role.
const (
	cacheWingetDeps = "DesktopAppInstaller_Dependencies.zip"
	cacheWinget     = "Microsoft.DesktopAppInstaller.msixbundle"
	cacheVCRedist   = "vc_redist.x64.exe"
)

// preparationCache is what the cache holds: winget's App Installer bundle and
// the dependencies Microsoft ships with that very release (the same release,
// so they match), and the Visual C++ runtime.
var preparationCache = []cacheItem{
	{cacheWingetDeps, "https://github.com/microsoft/winget-cli/releases/latest/download/DesktopAppInstaller_Dependencies.zip"},
	{cacheWinget, "https://github.com/microsoft/winget-cli/releases/latest/download/Microsoft.DesktopAppInstaller_8wekyb3d8bbwe.msixbundle"},
	{cacheVCRedist, "https://aka.ms/vs/17/release/vc_redist.x64.exe"},
}

// fetchFunc downloads url into dst.
type fetchFunc func(ctx context.Context, url, dst string) error

// EnsureCache makes dir hold every file of the preparation cache, fetching
// those missing or older than cacheMaxAge; it answers whether the cache is
// whole. A file that cannot be fetched leaves the cache unusable for this run
// (the preparation then takes the network path) - never a half-made file.
func EnsureCache(ctx context.Context, dir string, fetch fetchFunc, out io.Writer) bool {
	if fetch == nil {
		fetch = httpFetch
	}
	if out == nil {
		out = io.Discard
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(out, "preparation cache: %v - the Sandbox is prepared from the network\n", err)
		return false
	}
	for _, it := range preparationCache {
		dst := filepath.Join(dir, it.Name)
		if info, err := os.Stat(dst); err == nil && info.Size() > 0 && time.Since(info.ModTime()) < cacheMaxAge {
			continue
		}
		fmt.Fprintf(out, "preparation cache: fetching %s\n", it.Name)
		tmp := dst + ".part"
		if err := fetch(ctx, it.URL, tmp); err != nil {
			_ = os.Remove(tmp)
			fmt.Fprintf(out, "preparation cache: %s: %v - the Sandbox is prepared from the network\n", it.Name, err)
			return false
		}
		if err := renameRetry(tmp, dst); err != nil {
			_ = os.Remove(tmp)
			fmt.Fprintf(out, "preparation cache: %s: %v - the Sandbox is prepared from the network\n", it.Name, err)
			return false
		}
	}
	return true
}

// httpFetch downloads url (redirects followed) into dst.
func httpFetch(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// cachedWingetScript installs winget from the cache: the x64 packages of the
// release's dependencies archive (opened with .NET: the Archive module does
// not load in a Sandbox), then the App Installer bundle, then shows the
// version through the alias the stages use.
func cachedWingetScript() string {
	return `$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; ` +
		`$deps = Join-Path $env:TEMP 'winget-deps'; ` +
		`Add-Type -AssemblyName System.IO.Compression.FileSystem; ` +
		`if (Test-Path $deps) { Remove-Item -Recurse -Force $deps }; ` +
		`[IO.Compression.ZipFile]::ExtractToDirectory('` + SandboxCache + `\` + cacheWingetDeps + `', $deps); ` +
		`Get-ChildItem -Path (Join-Path $deps 'x64') -Filter *.appx | ForEach-Object { Add-AppxPackage -Path $_.FullName }; ` +
		`Add-AppxPackage -Path '` + SandboxCache + `\` + cacheWinget + `'; ` +
		`& "$env:LOCALAPPDATA\Microsoft\WindowsApps\winget.exe" --version`
}

// cachedVCRedist is the cached Visual C++ runtime's silent install, its log
// (and each package's) kept in the run's results. In a Windows Sandbox each of
// its two packages waits about two minutes on Windows Installer's signature
// check - the logs show it - whatever the source.
func cachedVCRedist() []string {
	return []string{SandboxCache + `\` + cacheVCRedist, "/install", "/quiet", "/norestart", "/log", SandboxResults + `\vcredist.log`}
}

// renameRetry renames, retrying for a while: on Windows a file just written
// is often held a moment (an antivirus scanning it, a Sandbox closing its
// mapped folder), and the rename is refused until it lets go.
func renameRetry(from, to string) error {
	var err error
	for deadline := time.Now().Add(30 * time.Second); ; {
		if err = os.Rename(from, to); err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
}
