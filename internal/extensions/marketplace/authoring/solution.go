// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
)

// SolutionsDir is where solutions are kept in a marketplace:
// `solutions/<name>/<version>/` — solution.yaml and docs/<lang>/.
const SolutionsDir = "solutions"

// VaultsDir is where vaults are kept: `vaults/<version>/<name>.vault`.
const VaultsDir = "vaults"

func solutionDir(name, version string) string { return path.Join(SolutionsDir, name, version) }

// whatsNewFile is the solution document an installation shows as « Quoi de neuf ».
const whatsNewFile = "quoi-de-neuf.md"

// publishDocuments writes a solution's documents beside the catalog — the
// docs folder's `<lang>/*.md`, and « Quoi de neuf » (French) made from the
// change log unless the docs folder holds a letter written for its users
// (fr/quoi-de-neuf.md) — and answers them, each with its sha256.
func publishDocuments(outDir, name, version string, spec SolutionSpec) ([]mktapi.Document, error) {
	type doc struct {
		lang, name string
		data       []byte
	}
	var docs []doc
	if spec.Docs != "" {
		langs, err := os.ReadDir(spec.Docs)
		if err != nil {
			return nil, fmt.Errorf("the documents folder %s: %w", spec.Docs, err)
		}
		for _, l := range langs {
			if !l.IsDir() {
				continue
			}
			files, _ := os.ReadDir(filepath.Join(spec.Docs, l.Name()))
			for _, f := range files {
				if f.IsDir() || !strings.EqualFold(filepath.Ext(f.Name()), ".md") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(spec.Docs, l.Name(), f.Name()))
				if err != nil {
					return nil, err
				}
				docs = append(docs, doc{l.Name(), f.Name(), data})
			}
		}
		if len(docs) == 0 {
			return nil, fmt.Errorf("the documents folder %s holds no <lang>/*.md", spec.Docs)
		}
	}
	written := false
	for _, d := range docs {
		if d.lang == "fr" && strings.EqualFold(d.name, whatsNewFile) {
			written = true
			// Its figures are compared with the change logs it covers
			// ; a difference is reported, never a refusal
			// to publish: the change log grows between the letter and the
			// cut (the person, 2026-10-21).
			covers := spec.LetterCovers
			if len(covers) == 0 && spec.WhatsNew != "" {
				covers = []string{spec.WhatsNew}
			}
			requests := spec.LetterRequests
			if requests == 0 {
				requests = -1
			}
			if err := CheckLetterFigures(string(d.data), covers, requests); err != nil {
				slog.Warn("solution: its « Quoi de neuf » figures differ from its change logs - published as written", "error", err)
				fmt.Fprintf(os.Stderr, "WARNING: %v\n", err)
			}
		}
	}
	if spec.WhatsNew != "" && !written {
		_, md, err := WhatsNewMarkdown(spec.WhatsNew)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc{"fr", whatsNewFile, md})
	}
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].lang+"/"+docs[i].name < docs[j].lang+"/"+docs[j].name
	})
	var out []mktapi.Document
	for _, d := range docs {
		rel := path.Join(solutionDir(name, version), "docs", d.lang, d.name)
		dst := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, d.data, 0o644); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(d.data)
		out = append(out, mktapi.Document{Lang: d.lang, Name: d.name, URL: rel, Sha256: hex.EncodeToString(sum[:])})
	}
	return out, nil
}

// writeSolutionFile writes `solutions/<name>/<version>/solution.yaml`: what the
// signed catalog pins, readable — its name, then the version marshalled on its
// own (an inlined ItemVersion loses its fields now that it decodes itself).
func writeSolutionFile(outDir, name string, v mktapi.ItemVersion) error {
	nameLine, err := marshal.Marshal(map[string]string{"name": name}, marshal.YAML)
	if err != nil {
		return err
	}
	body, err := marshal.Marshal(v, marshal.YAML)
	if err != nil {
		return err
	}
	data := append(nameLine, body...)
	dst := filepath.Join(outDir, filepath.FromSlash(solutionDir(name, v.Version)), "solution.yaml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, append([]byte("# The solution "+name+"@"+v.Version+": what the signed catalog pins.\n"), data...), 0o644)
}

// BuildVaultResult reports a published vault.
type BuildVaultResult struct {
	CatalogPath, AssetPath, Sha256, Name, Version string
	Warning                                       string
}

// BuildVault publishes a sealed keys file (`katyoti keys export`) as a
// `kind: vault` — as it is: the marketplace never sees its password. A file
// that is not a sealed file is refused.
func (s *Service) BuildVault(file, name, version, outDir string, keep int, secretKeyPath string) (BuildVaultResult, error) {
	res := BuildVaultResult{Name: name, Version: version}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return res, fmt.Errorf("build-vault: a name and a version are required")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return res, err
	}
	var head struct {
		Format string `yaml:"format"`
	}
	if err := config.DecodeBytes(data, marshal.YAML, &head); err != nil || !strings.HasPrefix(head.Format, "katyoti-sealed/") {
		return res, fmt.Errorf("%s is not a sealed keys file (make one with `katyoti keys export`)", file)
	}
	sum := sha256.Sum256(data)
	res.Sha256 = hex.EncodeToString(sum[:])
	res.AssetPath = path.Join(VaultsDir, version, name+".vault")
	fresh := mktapi.Catalog{APIVersion: 1, Items: []mktapi.Item{{
		Name: name, Kind: mktapi.KindVault, DisplayName: name,
		Versions: []mktapi.ItemVersion{{Version: version, Assets: []mktapi.Asset{{URL: res.AssetPath, Sha256: res.Sha256}}}},
	}}}
	res.CatalogPath = filepath.Join(outDir, "katyoti-marketplace.yaml")
	published, err := loadCatalogFile(res.CatalogPath)
	if err != nil {
		return res, err
	}
	merged, _, err := index.Merge(published, fresh, keep)
	if err != nil {
		return res, err
	}
	dst := filepath.Join(outDir, filepath.FromSlash(res.AssetPath))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return res, err
	}
	out, err := index.MarshalCatalog(merged)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(res.CatalogPath, out, 0o644); err != nil {
		return res, err
	}
	res.Warning, err = s.resignOrWarn(res.CatalogPath, secretKeyPath, "vault "+name+"@"+version)
	return res, err
}
