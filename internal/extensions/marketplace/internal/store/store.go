// SPDX-License-Identifier: Apache-2.0

// Package store manages the shared, version-addressed binary store
// (`<GlobalHomeDir>/bin/store/<name>/<version>/`). Multiple versions of a
// sidecar coexist; a universe pins a version by reference (ADR §14). Install
// fetches the asset, verifies its sha256, and places it atomically.
package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// Root is `<GlobalHomeDir>/bin/store`.
func Root() (string, error) {
	g := application.Env().GlobalHomeDir()
	if g == "" {
		return "", fmt.Errorf("resolve global home dir")
	}
	return filepath.Join(g, "bin", "store"), nil
}

// BinaryPath is the placed binary path for a sidecar item version.
func BinaryPath(name, version string) (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name, version, name+exeSuffix()), nil
}

// Has reports whether a sidecar version is already placed in the store.
func Has(name, version string) bool {
	p, err := BinaryPath(name, version)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// InstallSidecar fetches the asset, verifies its sha256, and places the binary
// in the versioned store (atomic write + rename); a bundle the asset carries is
// unpacked beside it into `<name>-assets/`. Idempotent: an already-placed
// version — binary and, when declared, its bundle — is returned as-is.
func InstallSidecar(ctx context.Context, name, version string, asset mktapi.Asset) (string, error) {
	dest, err := BinaryPath(name, version)
	if err != nil {
		return "", err
	}
	return dest, installSidecarAt(ctx, dest, name, version, asset)
}

// BundleDir is where a sidecar binary's bundle is unpacked: `<name>-assets`
// beside it (ktw-display reads `<exe dir>/ktw-display-assets/`).
func BundleDir(binaryPath, name string) string {
	return filepath.Join(filepath.Dir(binaryPath), name+mktapi.BundleDirSuffix)
}

func installSidecarAt(ctx context.Context, dest, name, version string, asset mktapi.Asset) error {
	if _, err := os.Stat(dest); err != nil {
		data, err := fetchVerified(ctx, asset.URL, asset.Sha256, name+"@"+version)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, data, 0o755); err != nil {
			return err
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	if asset.Bundle == nil {
		return nil
	}
	dir := BundleDir(dest, name)
	if _, err := os.Stat(dir); err == nil {
		return nil // unpacked by an earlier install of this version
	}
	data, err := fetchVerified(ctx, asset.Bundle.URL, asset.Bundle.Sha256, name+"@"+version+" bundle")
	if err != nil {
		return err
	}
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := extractTarGz(data, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("%s@%s bundle: %w", name, version, err)
	}
	return replaceDir(tmp, dir)
}

// fetchVerified downloads a file and checks it against its sha256 when the
// catalog gives one.
func fetchVerified(ctx context.Context, url, sha, what string) ([]byte, error) {
	data, err := fetchAsset(ctx, url)
	if err != nil {
		return nil, err
	}
	if sha != "" {
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != sha {
			return nil, fmt.Errorf("%s: sha256 mismatch (want %s, got %s)", what, sha, got)
		}
	}
	return data, nil
}

// ── Content items (files-only folders, run on ktw-host) ──────────────────

// ContentRoot is `<GlobalHomeDir>/bin/store-content` — the version-addressed
// store for content (files-only) items. Distinct from the binary store: a
// content version is an unpacked FOLDER, not a single exe.
func ContentRoot() (string, error) {
	g := application.Env().GlobalHomeDir()
	if g == "" {
		return "", fmt.Errorf("resolve global home dir")
	}
	return filepath.Join(g, "bin", "store-content"), nil
}

// ContentFolderPath is the unpacked folder for a content item version — the
// `ktw-host --folder` target.
func ContentFolderPath(name, version string) (string, error) {
	root, err := ContentRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name, version), nil
}

// HasContent reports whether a content version is already unpacked (its
// well-known `ktw-host.yaml` manifest is present).
func HasContent(name, version string) bool {
	folder, err := ContentFolderPath(name, version)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(folder, "ktw-host.yaml"))
	return err == nil
}

// ── Config-sets (templates + a keys.yaml manifest) ───────────────────

// ConfigSetRoot is `<GlobalHomeDir>/bin/store-config` — the version-addressed
// store for config-sets. A THIRD store beside the binary and content ones: a
// config-set is neither runnable nor wired into a universe, it is a source of
// templates + keys that `config-set apply` copies FROM. Keeping it separate means
// listing/pruning config-sets never walks runnable items.
func ConfigSetRoot() (string, error) {
	g := application.Env().GlobalHomeDir()
	if g == "" {
		return "", fmt.Errorf("resolve global home dir")
	}
	return filepath.Join(g, "bin", "store-config"), nil
}

// ConfigSetFolderPath is the unpacked bundle folder for one config-set version.
func ConfigSetFolderPath(name, version string) (string, error) {
	root, err := ConfigSetRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name, version), nil
}

// HasConfigSet reports whether a config-set version is already unpacked. Keyed on
// the REQUIRED `keys.yaml` manifest — the same "well-known file proves a
// complete unpack" trick HasContent uses with ktw-host.yaml, so a half-extracted
// folder is never mistaken for an install.
func HasConfigSet(name, version string) bool {
	folder, err := ConfigSetFolderPath(name, version)
	if err != nil {
		return false
	}
	for _, f := range []string{RequiredKeysFile, legacyRequiredKeysFile} {
		if _, err := os.Stat(filepath.Join(folder, f)); err == nil {
			return true
		}
	}
	return false
}

// RequiredKeysFile is the manifest every config-set bundle must carry (spec §5);
// legacyRequiredKeysFile its former name, in bundles published before.
const (
	RequiredKeysFile       = "keys.yaml"
	legacyRequiredKeysFile = "required-keys.yaml"
)

// InstallConfigSet fetches the bundle tar.gz, verifies its sha256, and extracts it
// into the versioned config-set store. Same atomic extract-then-rename + zip-slip
// guard as content; idempotent on an already-unpacked version.
func InstallConfigSet(ctx context.Context, name, version string, asset mktapi.Asset) (string, error) {
	folder, err := ConfigSetFolderPath(name, version)
	if err != nil {
		return "", err
	}
	if HasConfigSet(name, version) {
		return folder, nil
	}
	data, err := fetchAsset(ctx, asset.URL)
	if err != nil {
		return "", err
	}
	if asset.Sha256 != "" {
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != asset.Sha256 {
			return "", fmt.Errorf("%s@%s: sha256 mismatch (want %s, got %s)", name, version, asset.Sha256, got)
		}
	}
	if err := os.MkdirAll(filepath.Dir(folder), 0o755); err != nil {
		return "", err
	}
	tmp := folder + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := extractTarGz(data, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	if err := replaceDir(tmp, folder); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	return folder, nil
}

// SeedFolderPath is the unpacked seed of one release version, in the store.
func SeedFolderPath(release, version string) (string, error) {
	g := application.Env().GlobalHomeDir()
	if g == "" {
		return "", fmt.Errorf("resolve global home dir")
	}
	return filepath.Join(g, "bin", "store-seed", release, version), nil
}

// InstallSeed fetches a release's seed, verifies its sha256 and unpacks it into
// the store (atomically, idempotently — a complete seed carries its marker).
// What it may hold is checked by the caller (seed.Check) before it is applied.
func InstallSeed(ctx context.Context, release, version string, asset mktapi.Asset) (string, error) {
	folder, err := SeedFolderPath(release, version)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(folder, SeedMarker)); err == nil {
		return folder, nil
	}
	data, err := fetchAsset(ctx, asset.URL)
	if err != nil {
		return "", err
	}
	if asset.Sha256 == "" {
		return "", fmt.Errorf("seed of %s@%s: no sha256 — a seed is installed only verified", release, version)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != asset.Sha256 {
		return "", fmt.Errorf("seed of %s@%s: sha256 mismatch (want %s, got %s)", release, version, asset.Sha256, got)
	}
	if err := os.MkdirAll(filepath.Dir(folder), 0o755); err != nil {
		return "", err
	}
	tmp := folder + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := extractTarGz(data, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	if err := replaceDir(tmp, folder); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	return folder, nil
}

// SeedMarker is the file a complete seed carries (seed.Marker).
const SeedMarker = "seed.yaml"

// InstallContent fetches the folder bundle (a `.tar.gz`), verifies its sha256,
// and extracts it into the versioned content store. Atomic: extract into a
// sibling `.tmp` dir, then rename over the destination. Idempotent — an
// already-unpacked version is returned as-is. Update = replace the folder.
func InstallContent(ctx context.Context, name, version string, asset mktapi.Asset) (string, error) {
	folder, err := ContentFolderPath(name, version)
	if err != nil {
		return "", err
	}
	if HasContent(name, version) {
		return folder, nil
	}
	data, err := fetchAsset(ctx, asset.URL)
	if err != nil {
		return "", err
	}
	if asset.Sha256 != "" {
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != asset.Sha256 {
			return "", fmt.Errorf("%s@%s: sha256 mismatch (want %s, got %s)", name, version, asset.Sha256, got)
		}
	}
	if err := os.MkdirAll(filepath.Dir(folder), 0o755); err != nil {
		return "", err
	}
	tmp := folder + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := extractTarGz(data, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	if err := replaceDir(tmp, folder); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	return folder, nil
}

// replaceDir swaps tmp into place at dest, removing whatever is there first.
//
// Retried, because on Windows the obvious sequence — RemoveAll(dest) then
// Rename(tmp, dest) — is NOT atomic. A deleted directory stays present under its
// old name until every handle on it closes, so the rename lands on a name that
// still exists and fails with ACCESS_DENIED. Anything holding a handle for a
// moment triggers it: an on-access virus scanner reading the files just
// extracted, an indexer, an open Explorer window. It is intermittent by nature —
// installing the same content item three times in a row failed once here.
//
// A bounded retry is the standard remedy: the handles close on their own within
// milliseconds, and if they do not, the original error is returned unchanged.
func replaceDir(tmp, dest string) error {
	const attempts = 10
	var err error
	for i := 0; i < attempts; i++ {
		_ = os.RemoveAll(dest)
		if err = os.Rename(tmp, dest); err == nil {
			return nil
		}
		time.Sleep(time.Duration(20*(i+1)) * time.Millisecond)
	}
	return err
}

// extractTarGz unpacks a gzip-compressed tar archive into dest. Guards against
// path traversal (zip-slip): every entry must resolve under dest.
func extractTarGz(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	prefix := filepath.Clean(dest) + string(os.PathSeparator)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		target := filepath.Join(dest, hdr.Name)
		if target != filepath.Clean(dest) && !strings.HasPrefix(target, prefix) {
			return fmt.Errorf("tar: unsafe path %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil { //nolint:gosec // size-bounded by the sha256-verified asset
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// UninstallAll removes every version of an item from BOTH the binary store and
// the content store (an item is one or the other, so the miss is a no-op).
// UninstallVersion removes ONE version of an item from the store, leaving every
// other version installed. Covers both the binary store and the content store,
// since an item is one kind or the other. Missing paths are not an error — the
// caller has already checked what exists and this must stay idempotent.
func UninstallVersion(name, version string) error {
	root, err := Root()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(root, name, version)); err != nil {
		return err
	}
	croot, err := ContentRoot()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(croot, name, version)); err != nil {
		return err
	}
	cfgroot, err := ConfigSetRoot()
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(cfgroot, name, version))
}

// HasAnyVersion reports whether a version folder exists for the item in either
// store, so callers can distinguish "not installed" from "wrong version".
func HasAnyVersion(name, version string) bool {
	root, err := Root()
	if err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(root, name, version)); err == nil {
		return true
	}
	croot, err := ContentRoot()
	if err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(croot, name, version)); err == nil {
		return true
	}
	cfgroot, err := ConfigSetRoot()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(cfgroot, name, version))
	return err == nil
}

func UninstallAll(name string) error {
	root, err := Root()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
		return err
	}
	croot, err := ContentRoot()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(croot, name)); err != nil {
		return err
	}
	cfgroot, err := ConfigSetRoot()
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(cfgroot, name))
}

func fetchAsset(ctx context.Context, url string) ([]byte, error) {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("download %s: status %d", url, resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(strings.TrimPrefix(url, "file://"))
}

// ── Vaults (sealed keys files) ────────────────────────────────────────────

// VaultPath is where a vault version is kept:
// `<GlobalHomeDir>/bin/store-vault/<name>/<version>/<name>.vault`.
func VaultPath(name, version string) (string, error) {
	g := application.Env().GlobalHomeDir()
	if g == "" {
		return "", fmt.Errorf("resolve global home dir")
	}
	return filepath.Join(g, "bin", "store-vault", name, version, name+".vault"), nil
}

// InstallVault fetches a vault (verified by its sha256) into the store, as it
// is: sealed, opened only with its password.
func InstallVault(ctx context.Context, name, version string, asset mktapi.Asset) (string, error) {
	p, err := VaultPath(name, version)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	data, err := fetchVerified(ctx, asset.URL, asset.Sha256, name+"@"+version)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, os.WriteFile(p, data, 0o600)
}

// FetchVerified is one file fetched and checked against its sha256 (a
// solution's document).
func FetchVerified(ctx context.Context, url, sha, what string) ([]byte, error) {
	return fetchVerified(ctx, url, sha, what)
}

// Fetch reads a file over http(s) or from disk, unchecked: the caller checks
// it (the prereqs engine refuses a download whose sha256 does not match).
func Fetch(ctx context.Context, url string) ([]byte, error) {
	return fetchAsset(ctx, url)
}
