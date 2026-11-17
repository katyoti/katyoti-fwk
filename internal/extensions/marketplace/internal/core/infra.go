// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/appcore/selfupdate"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/available"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/infra"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
)

// SourceInfraChannel marks an installed record as coming from the infra
// channel rather than a catalog. It keeps infra OUT of the catalog-diffing
// paths (Outdated, findItem disambiguation), which is the point: infra has no
// catalog to diff against, it has a channel.
const SourceInfraChannel = "infra-channel"

// resolveInfraChannel finds the infra channel for this machine.
//
// The enabled marketplaces are passed only as a DERIVATION HINT (an explicit
// channel record or $KTW_INFRA_CHANNEL wins). Publishing infra as a subfolder
// of the marketplace you already subscribe to is the common case, and it should
// need no configuration at all.
func (s *service) resolveInfraChannel() (infra.Channel, error) {
	ms, err := s.reg.List()
	if err != nil {
		return infra.Channel{}, err
	}
	// Trust travels with the location: infra published as a subfolder of a
	// marketplace is signed by the same key, so a channel derived from a PINNED
	// marketplace inherits that pin rather than silently dropping to unverified.
	srcs := make([]infra.Source, 0, len(ms))
	for _, m := range ms {
		if m.Enabled {
			srcs = append(srcs, infra.Source{URL: m.URL, PubKey: m.PubKey, Trust: m.Trust})
		}
	}
	return infra.Resolve(srcs)
}

// infraSet is one platform's worth of resolved infra: the version the channel
// currently points at, and the verified asset for each binary it serves.
type infraSet struct {
	channel infra.Channel
	version string
	assets  map[string]mktapi.Asset
}

func (s *service) loadInfraSet(ctx context.Context) (infraSet, error) {
	return s.loadInfraSetAt(ctx, "")
}

// loadInfraSetAt resolves the channel at an explicit version. Empty version
// means "whatever the pointer names" -- the normal path.
func (s *service) loadInfraSetAt(ctx context.Context, version string) (infraSet, error) {
	ch, err := s.resolveInfraChannel()
	if err != nil {
		return infraSet{}, err
	}
	if version == "" {
		version, err = ch.LatestVersion(ctx)
		if err != nil {
			return infraSet{}, err
		}
	}
	assets, err := ch.Assets(ctx, version)
	if err != nil {
		return infraSet{}, err
	}
	return infraSet{channel: ch, version: version, assets: assets}, nil
}

// placeInfraBinary lands one infra binary in the versioned store (fetch +
// sha256 verify, the same path any sidecar takes) and swaps it into the
// control-plane location. The swap is rename-aside, so replacing the binary
// that is currently RUNNING is safe on Windows.
func (s *service) placeInfraBinary(ctx context.Context, name string, set infraSet) (mktapi.Installed, error) {
	asset := set.assets[name]
	path, err := store.InstallSidecar(ctx, name, set.version, asset)
	if err != nil {
		return mktapi.Installed{}, err
	}
	inst := mktapi.Installed{
		Name:        name,
		Kind:        string(mktapi.KindSidecar),
		Version:     set.version,
		Source:      SourceInfraChannel,
		Marketplace: set.channel.Root,
		Sha256:      asset.Sha256,
		BinaryPath:  path,
	}
	target, err := controlplane.TargetPath(name)
	if err != nil {
		return inst, err
	}
	if err := selfupdate.Replace(target, path); err != nil {
		return inst, fmt.Errorf("place into control-plane: %w", err)
	}
	if err := available.Record(inst); err != nil {
		return inst, err
	}
	return inst, nil
}

// InstallInfra brings every control-plane binary the channel serves to the
// channel's current version, whether or not it was installed before.
//
// Infra moves AS A SET. katyoti, ktw-hub and ktw-gui speak one KTSP protocol
// version to each other, so updating a subset is how you manufacture a skew
// bug that only reproduces on the machine that did it.
//
// A binary the channel does not serve for this platform is reported in Skipped
// rather than failing the whole bootstrap -- one missing macOS build should not
// block a working Windows install.
func (s *infraOps) InstallInfra(ctx context.Context) (mktapi.InstallInfraOutput, error) {
	var out mktapi.InstallInfraOutput
	set, err := s.loadInfraSet(ctx)
	if err != nil {
		return out, err
	}
	for _, name := range controlplane.InfraBinaries {
		if _, ok := set.assets[name]; !ok {
			out.Skipped = append(out.Skipped, name)
			continue
		}
		// Placing the version installed again (a repair) or a first install
		// moves nothing; another version is an update.
		if ins, err := s.installedItem(name); err == nil && ins.Version != "" && ins.Version != set.version {
			if err := refuseWhenLocked("control-plane install of " + set.version); err != nil {
				return out, err
			}
		}
		inst, err := s.placeInfraBinary(ctx, name, set)
		if err != nil {
			return out, fmt.Errorf("install-infra %s@%s: %w", name, set.version, err)
		}
		out.Installed = append(out.Installed, inst)
	}
	return out, nil
}

// SelfUpdate brings the control-plane binaries to the channel's current
// version, reporting only the ones that actually changed.
//
// Unlike InstallInfra it is a no-op when everything already matches the
// channel, so it is safe to call on every launch.
func (s *infraOps) SelfUpdate(ctx context.Context) ([]mktapi.Update, error) {
	if err := refuseWhenLocked("control-plane update"); err != nil {
		return nil, err
	}
	set, err := s.loadInfraSet(ctx)
	if err != nil {
		return nil, err
	}
	// A control plane make dev placed is a dev build of a checkout, whatever
	// an older install-infra record says: asked for, the update replaces it
	// with the channel's set, and says it was the dev build.
	devVersion, dev := controlPlaneDevBuild()
	var done []mktapi.Update
	for _, name := range controlplane.InfraBinaries {
		if _, ok := set.assets[name]; !ok {
			continue
		}
		was := ""
		if ins, err := s.installedItem(name); err == nil {
			was = ins.Version
		}
		if dev {
			was = devVersion
		} else if was == set.version {
			continue // already current
		}
		if _, err := s.placeInfraBinary(ctx, name, set); err != nil {
			return done, fmt.Errorf("self-update %s@%s: %w", name, set.version, err)
		}
		done = append(done, mktapi.Update{
			Name:        name,
			Installed:   was,
			Latest:      set.version,
			Marketplace: set.channel.Root,
		})
	}
	return done, nil
}

// InfraOutdated reports the control-plane binaries that are behind the channel.
// Separate from Outdated because infra is diffed against a CHANNEL, not a
// catalog -- Outdated walks catalog-installed items and would find nothing here.
//
// A control plane make dev placed is never behind: it follows the dev channel,
// which make dev moves, and only an explicit update --infra (or install-infra)
// replaces it.
func (s *service) InfraOutdated(ctx context.Context) ([]mktapi.Update, error) {
	if _, dev := controlPlaneDevBuild(); dev {
		return nil, nil
	}
	set, err := s.loadInfraSet(ctx)
	if err != nil {
		return nil, err
	}
	var out []mktapi.Update
	for _, name := range controlplane.InfraBinaries {
		if _, ok := set.assets[name]; !ok {
			continue
		}
		was := ""
		if ins, err := s.installedItem(name); err == nil {
			was = ins.Version
		}
		if was != set.version {
			out = append(out, mktapi.Update{
				Name:        name,
				Installed:   was,
				Latest:      set.version,
				Marketplace: set.channel.Root,
			})
		}
	}
	return out, nil
}

// SetInfraVersion rolls the control-plane set to an EXPLICIT channel version --
// the rollback verb.
//
// Pinning a single infra binary is refused (that is how skew is built by hand),
// but pinning the WHOLE SET is exactly what recovering from a bad publish
// requires, and it keeps the set coherent. The channel retains the previous
// version precisely so this can work.
func (s *infraOps) SetInfraVersion(ctx context.Context, version string) ([]mktapi.Update, error) {
	if version == "" {
		return nil, fmt.Errorf("SetInfraVersion: a version is required")
	}
	if err := refuseWhenLocked("control-plane move to " + version); err != nil {
		return nil, err
	}
	set, err := s.loadInfraSetAt(ctx, version)
	if err != nil {
		ch, rerr := s.resolveInfraChannel()
		if rerr == nil {
			if latest, prev, verr := ch.Versions(ctx); verr == nil {
				// The channel keeps two versions, so naming a third is the most
				// likely mistake. Say which two exist rather than only that this
				// one does not.
				return nil, fmt.Errorf("%w\nthe channel carries %s (current) and %s (rollback)", err, latest, prev)
			}
		}
		return nil, err
	}
	var done []mktapi.Update
	for _, name := range controlplane.InfraBinaries {
		if _, ok := set.assets[name]; !ok {
			continue
		}
		was := ""
		if ins, err := s.installedItem(name); err == nil {
			was = ins.Version
		}
		if was == set.version {
			continue
		}
		if _, err := s.placeInfraBinary(ctx, name, set); err != nil {
			return done, fmt.Errorf("set infra %s@%s: %w", name, set.version, err)
		}
		done = append(done, mktapi.Update{
			Name: name, Installed: was, Latest: set.version, Marketplace: set.channel.Root,
		})
	}
	return done, nil
}

// InfraVersions reports what the channel can serve right now.
//
// The pointer is the authority, not a directory listing: neither HTTP nor a
// synced share lists reliably, and a folder that survived a failed prune would
// otherwise look installable when the pointer can no longer reach it.
func (s *service) InfraVersions(ctx context.Context) (mktapi.InfraVersionsOutput, error) {
	// katyoti stands in for the set: they move together, so any one of them
	// answers "what is this machine running". A development build placed by
	// make dev comes first - install-infra's record, when an older one is
	// left, no longer names what runs - and it is said even when the channel
	// cannot be reached: it does not come from there.
	devVersion, dev := controlPlaneDevBuild()
	out := mktapi.InfraVersionsOutput{Binaries: controlplane.InfraBinaries}
	if dev {
		out.Installed, out.DevBuild = devVersion, true
	}
	ch, err := s.resolveInfraChannel()
	if err == nil {
		out.Channel = ch.Root
		out.Current, out.Previous, err = ch.Versions(ctx)
	}
	if err != nil {
		if dev {
			return mktapi.InfraVersionsOutput{Installed: devVersion, DevBuild: true, Binaries: controlplane.InfraBinaries}, nil
		}
		return mktapi.InfraVersionsOutput{}, err
	}
	if !dev {
		if ins, err := s.installedItem("katyoti"); err == nil {
			out.Installed = ins.Version
		}
	}
	return out, nil
}
