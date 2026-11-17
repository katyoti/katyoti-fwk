// SPDX-License-Identifier: Apache-2.0

package core

import (
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	inv "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A dev installation: make dev copies this checkout's builds
// into the installation - the control plane into <root>/control-plane, every
// other program into its dev channel, <root>/bin/dev - and records nothing in
// the available-items registry. The readers below learn it from the files
// themselves, so the registry keeps its meaning (what a marketplace or the
// infra channel installed) and a dev installation is still shown as one.

// devBuilds are the programs of the installation's dev channel (<root>/bin/dev,
// what make dev fills), by name, each with the commit it reports - read from
// its build info, never by running it. Empty when the installation has none.
func devBuilds() map[string]mktapi.DevBuild {
	dir, err := controlplane.DevDir()
	if err != nil {
		return nil
	}
	out := map[string]mktapi.DevBuild{}
	for _, e := range inv.ScanLocal([]inv.ScanRoot{{Dir: dir, Source: inv.SourceExecutableDir}}) {
		b := mktapi.DevBuild{Path: e.BinaryPath}
		if v, _, err := appcore.ProgramVersion(e.BinaryPath); err == nil {
			b.Version = v
		}
		out[e.Name] = b
	}
	return out
}

// controlPlaneDevBuild answers whether the installation's control plane is a
// development build - katyoti in <root>/control-plane carries no release
// stamp, so make dev placed it from a checkout - and the commit it reports.
// A program that is no Go build, or none at all, answers false: the infra
// channel's record says what is installed.
func controlPlaneDevBuild() (string, bool) {
	p, err := controlplane.TargetPath("katyoti")
	if err != nil {
		return "", false
	}
	v, stamped, err := appcore.ProgramVersion(p)
	if err != nil || stamped {
		return "", false
	}
	return v, true
}

// withDevBuilds marks each installed row with what a dev installation holds:
// a control-plane program as such, an item with a build in the dev channel
// with that build. A dev build the registry has no row for is added, so each
// dev program is one row.
func withDevBuilds(items []mktapi.Installed) []mktapi.Installed {
	dev := devBuilds()
	seen := map[string]bool{}
	for i := range items {
		seen[items[i].Name] = true
		items[i].ControlPlane = controlplane.IsInfra(items[i].Name)
		if b, ok := dev[items[i].Name]; ok && !items[i].ControlPlane {
			b := b
			items[i].DevBuild = &b
		}
	}
	names := make([]string, 0, len(dev))
	for name := range dev {
		names = append(names, name)
	}
	sort.Strings(names) // the same order at every call
	for _, name := range names {
		b := dev[name]
		if seen[name] || controlplane.IsInfra(name) {
			continue
		}
		items = append(items, mktapi.Installed{
			Name: name, Source: inv.SourceExecutableDir, BinaryPath: b.Path, DevBuild: &b,
		})
	}
	return items
}
