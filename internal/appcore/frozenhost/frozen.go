// SPDX-License-Identifier: Apache-2.0

// Package frozenhost implements "freeze-to-exe": bundling a files-only sidecar
// FOLDER into a single self-contained binary by APPENDING it to a copy of the
// generic host (ktw-host) — no Go rebuild (the whole point of ktw-host).
//
// Layout of a frozen exe:  [ ktw-host binary ][ tar.gz(folder) ][ trailer ]
// The trailer (last 16 bytes) is  magic[8] + gzLen[8] (little-endian) — read
// backwards from EOF so the host can find its embedded folder in its own exe.
// Trailing bytes after a PE/ELF/Mach-O image are ignored by the loader, so the
// binary still runs; the host just also reads itself to recover the folder.
//
// Publisher (ktw-compose) calls Pack; the host calls FrozenFolder at startup.
package frozenhost

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	magic      = "KTWFRZ01"
	trailerLen = 16 // magic[8] + gzLen[8]
)

// Pack returns a frozen binary = host + tar.gz(folder) + trailer.
func Pack(host []byte, folder string) ([]byte, error) {
	gz, err := TarGzDir(folder)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(host)+len(gz)+trailerLen)
	out = append(out, host...)
	out = append(out, gz...)
	var trailer [trailerLen]byte
	copy(trailer[:8], magic)
	binary.LittleEndian.PutUint64(trailer[8:], uint64(len(gz)))
	return append(out, trailer[:]...), nil
}

// IsFrozen reports whether the bytes carry the frozen trailer.
func IsFrozen(data []byte) bool {
	return len(data) >= trailerLen && string(data[len(data)-trailerLen:len(data)-8]) == magic
}

// FrozenFolder checks whether the binary at exePath is a frozen host. If so it
// extracts the embedded folder into a content-hash-keyed dir under cacheRoot
// (idempotent — reused across runs) and returns that path. Returns "" when the
// binary is NOT frozen (a normal `--folder` host).
func FrozenFolder(exePath, cacheRoot string) (string, error) {
	data, err := os.ReadFile(exePath)
	if err != nil {
		return "", err
	}
	if !IsFrozen(data) {
		return "", nil
	}
	gzLen := binary.LittleEndian.Uint64(data[len(data)-8:])
	end := uint64(len(data)) - trailerLen
	if gzLen == 0 || gzLen > end {
		return "", fmt.Errorf("frozen host: bad embedded length")
	}
	gz := data[end-gzLen : end]

	sum := sha256.Sum256(gz)
	dest := filepath.Join(cacheRoot, "ktw-host-frozen", hex.EncodeToString(sum[:])[:16])
	// Already extracted (the manifest is the completeness marker)?
	if _, err := os.Stat(filepath.Join(dest, "ktw-host.yaml")); err == nil {
		return dest, nil
	}
	tmp := dest + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := extractTarGz(gz, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	_ = os.RemoveAll(dest)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	return dest, nil
}

// TarGzDir tar.gz's a folder's contents at the archive root.
//
// Exported because it is the ONE folder-packer in the tree: freeze-to-exe packs a
// content folder with it, and the marketplace packs a config-set bundle with it.
// Re-implementing a second tar walk elsewhere would let the two archive formats
// drift (mode bits, path separators, whether directories are emitted) while both
// are consumed by the same extractTarGz on the install side.
func TarGzDir(folder string) ([]byte, error) { return TarGzDirExcept(folder, nil) }

// TarGzDirExcept packs a folder, leaving out every path `skip` accepts.
//
// Exclusion exists for one reason: content in the bulk band is written to the
// object store at packaging and described by a recorded descriptor, so packing
// it too would put the bytes in both places and undo the point — a pack stays
// bounded by the NUMBER of contributions, not their size.
//
// `skip` receives the path relative to `folder`, slash-separated. A nil `skip`
// packs everything, which is what a self-contained bundle wants.
func TarGzDirExcept(folder string, skip func(rel string) bool) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	err := filepath.Walk(folder, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(folder, p)
		if rerr != nil {
			return rerr
		}
		if skip != nil && skip(filepath.ToSlash(rel)) {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, werr := tw.Write(data)
		return werr
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// extractTarGz unpacks into dest with a zip-slip guard.
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
			if _, err := io.Copy(f, tr); err != nil { //nolint:gosec // size-bounded by the embedded asset
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// ManifestFile is the marker that makes a folder a files-only sidecar.
const ManifestFile = "ktw-host.yaml"

// Identity is the catalog-facing subset of a folder's ktw-host.yaml.
//
// ktw-host owns the FULL manifest (dirs, config synthesis, …); this reads only
// the four fields a distribution unit needs, so a folder can be published
// without the publisher depending on the host's runtime schema. Adding runtime
// fields here would be the wrong place — they belong to the host.
type Identity struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"displayName,omitempty"`
	Icon        string `yaml:"icon,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// ReadIdentity loads a content folder's identity, and is also the check that
// the folder IS one: a directory without a parsable ktw-host.yaml naming a
// sidecar is not publishable, and saying so here beats producing a catalog
// entry nothing can run.
func ReadIdentity(folder string) (Identity, error) {
	var id Identity
	path := filepath.Join(folder, ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return id, fmt.Errorf("%s has no %s — a content item is a FOLDER describing itself; add one naming the sidecar", folder, ManifestFile)
		}
		return id, err
	}
	if err := yaml.Unmarshal(data, &id); err != nil {
		return id, fmt.Errorf("parse %s: %w", path, err)
	}
	if strings.TrimSpace(id.Name) == "" {
		return id, fmt.Errorf("%s declares no `name` — that name IS the catalog item and the runtime provider", path)
	}
	if id.DisplayName == "" {
		id.DisplayName = id.Name
	}
	return id, nil
}

// StampVersion records version in a content folder's ktw-host.yaml.
//
// The folder is the unit of distribution, so it has to carry its own version:
// `describe` on a frozen copy reported the ktw-host build it was embedded into,
// which meant a folder published as v1.0.0 answered with a git revision, and
// re-freezing untouched content after a host rebuild silently changed its
// version.
//
// Rewrites only the top-level `version:` line (or appends one), leaving
// comments, key order and formatting intact — a publish step must not
// reformat a file the author maintains by hand. An indented `version:` (a
// program's in the `config:` block) is not the folder's and is left alone.
func StampVersion(folder, version string) error {
	if strings.TrimSpace(version) == "" {
		return nil
	}
	path := filepath.Join(folder, ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for i, ln := range lines {
		if strings.HasPrefix(ln, "version:") {
			lines[i] = "version: " + version
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
		}
	}
	out := strings.TrimRight(string(data), "\n") + "\nversion: " + version + "\n"
	return os.WriteFile(path, []byte(out), 0o644)
}
