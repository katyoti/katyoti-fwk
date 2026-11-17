// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
)

// RELEASE CHANNELS (decision) - a
// published version is on the stable channel (end users) unless marked canary:
// a release candidate, seen only by the installations following canary.
// Promoting a candidate publishes its own artifacts on stable under the
// official version: the same files at the same paths, the same sha256 - nothing
// rebuilt - and the catalog, signed as a whole, signed again.

// ChannelResult says what a channel change or a promotion touched.
type ChannelResult struct {
	Items   []string `json:"items"`
	Warning string   `json:"warning,omitempty"`
}

// MarkChannel puts every item's version `version` on a channel (stable or
// canary) in the catalog under outDir, re-signed when a key is given.
func (s *Service) MarkChannel(outDir, version, channel, secretKeyPath string) (ChannelResult, error) {
	if channel != mktapi.ChannelStable && channel != mktapi.ChannelCanary {
		return ChannelResult{}, fmt.Errorf("channel %q: stable or canary", channel)
	}
	if channel == mktapi.ChannelStable {
		channel = ""
	}
	return s.editCatalog(outDir, secretKeyPath, "channel "+version, func(cat *mktapi.Catalog) ([]string, error) {
		var touched []string
		for i := range cat.Items {
			for j := range cat.Items[i].Versions {
				if cat.Items[i].Versions[j].Version == version {
					cat.Items[i].Versions[j].Channel = channel
					touched = append(touched, cat.Items[i].Name)
				}
			}
		}
		if len(touched) == 0 {
			return nil, fmt.Errorf("no item published as %s", version)
		}
		return touched, nil
	})
}

// Promote publishes a candidate on stable under its official version: for each
// item published as `from`, a version `to` on stable carrying the candidate's
// own assets and pins - its files, its sha256 - put first; the candidate stays
// on canary. Refused when `to` is already published for an item.
func (s *Service) Promote(outDir, from, to, secretKeyPath string) (ChannelResult, error) {
	if from == "" || to == "" || from == to {
		return ChannelResult{}, fmt.Errorf("promote: a candidate version and a distinct official version required")
	}
	var solutions []mktapi.ItemVersion
	var names []string
	res, err := s.editCatalog(outDir, secretKeyPath, "promote "+from+" as "+to, func(cat *mktapi.Catalog) ([]string, error) {
		var touched []string
		for i := range cat.Items {
			vs := cat.Items[i].Versions
			k := slices.IndexFunc(vs, func(v mktapi.ItemVersion) bool { return v.Version == from })
			if k < 0 {
				continue
			}
			if slices.ContainsFunc(vs, func(v mktapi.ItemVersion) bool { return v.Version == to }) {
				return nil, fmt.Errorf("%s %s is already published: a promotion never replaces a version", cat.Items[i].Name, to)
			}
			official := vs[k]
			official.Version, official.Channel = to, ""
			official.Assets = slices.Clone(vs[k].Assets)
			cat.Items[i].Versions = append([]mktapi.ItemVersion{official}, vs...)
			touched = append(touched, cat.Items[i].Name)
			if cat.Items[i].Kind == mktapi.KindSolution {
				solutions, names = append(solutions, official), append(names, cat.Items[i].Name)
			}
		}
		if len(touched) == 0 {
			return nil, fmt.Errorf("no item published as %s", from)
		}
		return touched, nil
	})
	if err != nil {
		return res, err
	}
	// A solution's readable pin under its official version too.
	for i, v := range solutions {
		if err := writeSolutionFile(outDir, names[i], v); err != nil {
			return res, err
		}
	}
	// The control plane the candidate cut published (without moving the
	// pointer) becomes the one installations follow.
	if _, err := os.Stat(filepath.Join(outDir, "infra", from)); err == nil {
		if _, err := s.PointInfraChannel(outDir, from, secretKeyPath, promotedSolution(outDir, names, to)); err != nil {
			return res, fmt.Errorf("point the infra channel at %s: %w", from, err)
		}
	}
	return res, nil
}

// promotedSolution is the solution a first install opens on after a
// promotion, at its official version: the one the infra pointer named, when
// it was promoted, else the first promoted; "" when none was.
func promotedSolution(outDir string, promoted []string, to string) string {
	if len(promoted) == 0 {
		return ""
	}
	name := promoted[0]
	if data, err := os.ReadFile(filepath.Join(outDir, "infra", "channel.yaml")); err == nil {
		var p infraPointer
		if config.DecodeBytes(data, marshal.YAML, &p) == nil {
			if cur, _, _ := strings.Cut(p.Solution, "@"); slices.Contains(promoted, cur) {
				name = cur
			}
		}
	}
	return name + "@" + to
}

// Prune applies retention to the catalog under outDir - each item's `keep`
// newest versions of each channel - deletes the files no retained version
// uses and signs the catalog again. A canary cut publishes with no retention
// and prunes once its version is marked canary.
func (s *Service) Prune(outDir string, keep int, secretKeyPath string) (ChannelResult, error) {
	var dropped []index.Dropped
	res, err := s.editCatalog(outDir, secretKeyPath, "retention", func(cat *mktapi.Catalog) ([]string, error) {
		var touched []string
		*cat, dropped = index.Retain(*cat, keep)
		for _, d := range dropped {
			touched = append(touched, d.Item+"@"+d.Version)
		}
		return touched, nil
	})
	if err != nil {
		return res, err
	}
	// Catalog first, then the bytes: never an entry without its files.
	for _, d := range dropped {
		for _, rel := range d.Assets {
			if isAbsoluteURL(rel) {
				continue // hosted elsewhere: nothing local to delete
			}
			p := filepath.Join(outDir, filepath.FromSlash(rel))
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return res, fmt.Errorf("prune %s@%s: %w", d.Item, d.Version, err)
			}
			pruneEmptyDirs(outDir, filepath.Dir(p))
		}
	}
	return res, nil
}

// editCatalog reads the catalog under outDir, changes it, writes it back and
// re-signs it (or says its signature went stale).
func (s *Service) editCatalog(outDir, secretKeyPath, comment string, change func(*mktapi.Catalog) ([]string, error)) (ChannelResult, error) {
	path := filepath.Join(outDir, "katyoti-marketplace.yaml")
	cat, err := loadCatalogFile(path)
	if err != nil {
		return ChannelResult{}, err
	}
	touched, err := change(&cat)
	if err != nil {
		return ChannelResult{}, err
	}
	data, err := index.MarshalCatalog(cat)
	if err != nil {
		return ChannelResult{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ChannelResult{}, err
	}
	warn, err := s.resignOrWarn(path, secretKeyPath, comment)
	if err != nil {
		return ChannelResult{}, err
	}
	return ChannelResult{Items: touched, Warning: warn}, nil
}
