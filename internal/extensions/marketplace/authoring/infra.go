// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
)

// installers are the bootstrap installers written into every channel: each
// fetches katyoti alone and hands over to `katyoti setup`. They are what the
// publisher SHIPS (artifacts copied out), not configuration read at runtime.
// They encode the channel's layout, so they live beside the code that writes
// that layout.
//
//go:embed installers/*
var installers embed.FS

// InfraChannelResult reports a published infra version.
type InfraChannelResult struct {
	Channel   string
	Version   string
	Previous  string
	Platforms []string
	Pruned    []string
}

// infraPointer is the channel's one discovery file (infra.PointerFile).
type infraPointer struct {
	Version  string `yaml:"version"`
	Previous string `yaml:"previous,omitempty"`
	// Key names the publisher's public key, published beside the catalog: a
	// machine installing from the channel pins it whatever its file name.
	Key string `yaml:"key,omitempty"`
	// Solution names the solution a first install opens on
	// (<solution>@<version>); an older channel called it `demo`.
	Solution string `yaml:"solution,omitempty"`
}

// BuildInfraChannel publishes the control plane at version into
// <outDir>/infra and points the channel at it: BuildInfraVersion, then
// PointInfraChannel. solution (<solution>@<version>, optional) is the solution
// `katyoti setup` installs and opens on.
func (s *Service) BuildInfraChannel(dist, outDir, version, secretKeyPath, solution string) (InfraChannelResult, error) {
	res, err := s.BuildInfraVersion(dist, outDir, version, secretKeyPath, solution)
	if err != nil {
		return res, err
	}
	pointed, err := s.PointInfraChannel(outDir, version, secretKeyPath, solution)
	pointed.Platforms = res.Platforms
	return pointed, err
}

// BuildInfraVersion publishes the control plane at version into
// <outDir>/infra/<version> without moving the channel's pointer: for each
// <goos>-<goarch> folder of dist, the infra binaries (controlplane.InfraBinaries
// - the one list) and their signed SHA256SUMS. What a candidate (canary) cut
// publishes: installations go on following the pointer. A
// published version is immutable.
func (s *Service) BuildInfraVersion(dist, outDir, version, secretKeyPath, solution string) (InfraChannelResult, error) {
	var res InfraChannelResult
	if version == "" || version == "dev" {
		return res, fmt.Errorf("a version is required (not %q)", version)
	}
	if solution = strings.TrimSpace(solution); solution != "" && !strings.Contains(solution, "@") {
		return res, fmt.Errorf("the solution is <solution>@<version>, got %q", solution)
	}
	if _, err := os.Stat(secretKeyPath); err != nil {
		return res, fmt.Errorf("no signing key at %s", secretKeyPath)
	}
	if _, err := os.Stat(secretKeyPath + ".pub"); err != nil {
		return res, fmt.Errorf("no public key at %s", secretKeyPath+".pub")
	}
	channel := filepath.Join(outDir, "infra")
	versionDir := filepath.Join(channel, version)
	if _, err := os.Stat(versionDir); err == nil {
		return res, fmt.Errorf("infra %s is already published at %s: a published version is immutable — cut a new version", version, versionDir)
	}
	res.Channel, res.Version = channel, version

	platforms, err := os.ReadDir(dist)
	if err != nil {
		return res, fmt.Errorf("read %s: %w", dist, err)
	}
	for _, p := range platforms {
		if !p.IsDir() || !strings.Contains(p.Name(), "-") {
			continue // not a <goos>-<goarch> folder
		}
		dest := filepath.Join(versionDir, p.Name())
		var sums []string
		for _, bin := range controlplane.InfraBinaries {
			for _, cand := range []string{bin + ".exe", bin} {
				src := filepath.Join(dist, p.Name(), cand)
				data, err := os.ReadFile(src)
				if err != nil {
					continue
				}
				if err := os.MkdirAll(dest, 0o755); err != nil {
					return res, err
				}
				if err := os.WriteFile(filepath.Join(dest, cand), data, 0o755); err != nil {
					return res, err
				}
				sum := sha256.Sum256(data)
				sums = append(sums, hex.EncodeToString(sum[:])+"  "+cand)
				break
			}
		}
		if len(sums) == 0 {
			continue
		}
		sort.Strings(sums)
		sumsPath := filepath.Join(dest, "SHA256SUMS")
		if err := os.WriteFile(sumsPath, []byte(strings.Join(sums, "\n")+"\n"), 0o644); err != nil {
			return res, err
		}
		if _, err := s.Sign(sumsPath, secretKeyPath, "infra "+version+" "+p.Name()); err != nil {
			return res, err
		}
		res.Platforms = append(res.Platforms, p.Name())
	}
	if len(res.Platforms) == 0 {
		_ = os.RemoveAll(versionDir)
		return res, fmt.Errorf("no platform folder of %s ships any of %s", dist, strings.Join(controlplane.InfraBinaries, ", "))
	}

	return res, nil
}

// PointInfraChannel points <outDir>/infra at a published version: the pointer
// channel.yaml (version, the previous one kept for roll back, the key's name,
// the solution a first install opens on), signed; the public key beside the
// catalog and in the channel; the installers. Versions the pointer no longer
// reaches are pruned. What a stable cut and a promotion do.
func (s *Service) PointInfraChannel(outDir, version, secretKeyPath, solution string) (InfraChannelResult, error) {
	var res InfraChannelResult
	channel := filepath.Join(outDir, "infra")
	if _, err := os.Stat(filepath.Join(channel, version)); err != nil {
		return res, fmt.Errorf("infra %s is not published at %s", version, channel)
	}
	pubPath := secretKeyPath + ".pub"
	pub, err := os.ReadFile(pubPath)
	if err != nil {
		return res, fmt.Errorf("no public key at %s", pubPath)
	}
	res.Channel, res.Version = channel, version

	// The pointer: this version, the previous one kept for roll back.
	pointerPath := filepath.Join(channel, "channel.yaml")
	if data, err := os.ReadFile(pointerPath); err == nil {
		var old infraPointer
		if err := config.DecodeBytes(data, marshal.YAML, &old); err == nil && old.Version != version {
			res.Previous = old.Version
		}
	}
	keyName := filepath.Base(pubPath)
	pointer, err := marshal.Marshal(infraPointer{Version: version, Previous: res.Previous, Key: keyName, Solution: solution}, marshal.YAML)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(pointerPath, pointer, 0o644); err != nil {
		return res, err
	}
	if _, err := s.Sign(pointerPath, secretKeyPath, "infra channel -> "+version); err != nil {
		return res, err
	}
	// The public key, beside the catalog (what setup pins) and in the channel.
	for _, dst := range []string{filepath.Join(outDir, keyName), filepath.Join(channel, keyName)} {
		if err := os.WriteFile(dst, pub, 0o644); err != nil {
			return res, err
		}
	}

	// Prune what the pointer no longer reaches.
	entries, _ := os.ReadDir(channel)
	for _, e := range entries {
		if e.IsDir() && e.Name() != version && e.Name() != res.Previous {
			if err := os.RemoveAll(filepath.Join(channel, e.Name())); err == nil {
				res.Pruned = append(res.Pruned, e.Name())
			}
		}
	}

	// The installers.
	files, err := fs.ReadDir(installers, "installers")
	if err != nil {
		return res, err
	}
	for _, f := range files {
		data, err := installers.ReadFile("installers/" + f.Name())
		if err != nil {
			return res, err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(f.Name(), ".sh") || strings.HasSuffix(f.Name(), ".command") {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(channel, f.Name()), data, mode); err != nil {
			return res, err
		}
	}
	return res, nil
}

// InitResult reports a marketplace made ready to publish into.
type InitResult struct {
	Catalog    string
	KeyCreated bool
	PubKeyLine string
}

// InitMarketplace makes folder a marketplace to publish into: its signing key
// (created in keyFolder when absent — an existing key is never replaced: every
// signature already published depends on it), an empty catalog named name
// (when there is none), signed, and the public key beside it.
func (s *Service) InitMarketplace(folder, name, keyFolder, keyPrefix string) (InitResult, error) {
	var res InitResult
	if strings.TrimSpace(name) == "" || strings.TrimSpace(keyPrefix) == "" {
		return res, fmt.Errorf("a marketplace name and a key prefix are required")
	}
	if keyFolder == "" {
		keyFolder = folder
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return res, err
	}
	secret := filepath.Join(keyFolder, keyPrefix+".key")
	if _, err := os.Stat(secret); os.IsNotExist(err) {
		pubFile, secFile, _, err := s.Keygen(name + " marketplace key")
		if err != nil {
			return res, err
		}
		if err := os.MkdirAll(keyFolder, 0o755); err != nil {
			return res, err
		}
		if err := os.WriteFile(secret, []byte(secFile), 0o600); err != nil {
			return res, err
		}
		if err := os.WriteFile(secret+".pub", []byte(pubFile), 0o644); err != nil {
			return res, err
		}
		res.KeyCreated = true
	}
	pub, err := os.ReadFile(secret + ".pub")
	if err != nil {
		return res, fmt.Errorf("no public key beside %s: %w", secret, err)
	}
	res.Catalog = filepath.Join(folder, "katyoti-marketplace.yaml")
	if _, err := os.Stat(res.Catalog); os.IsNotExist(err) {
		data, err := marshalCatalog(name)
		if err != nil {
			return res, err
		}
		if err := os.WriteFile(res.Catalog, data, 0o644); err != nil {
			return res, err
		}
	}
	if _, err := s.Sign(res.Catalog, secret, "marketplace "+name); err != nil {
		return res, err
	}
	if err := os.WriteFile(filepath.Join(folder, keyPrefix+".key.pub"), pub, 0o644); err != nil {
		return res, err
	}
	res.PubKeyLine = strings.TrimSpace(lastLine(string(pub)))
	return res, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// marshalCatalog is an empty catalog named name.
func marshalCatalog(name string) ([]byte, error) {
	return index.MarshalCatalog(mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: name}, Items: []mktapi.Item{}})
}
