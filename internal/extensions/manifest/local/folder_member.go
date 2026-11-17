// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// FolderProviders are a FOLDER member's providers: its folder read as a pack
// (manifestcontrib.PackProviders: every folder a kind claims, the pack's own
// kinds included) with its ktw-host.yaml `dirs:` as remaps — read as the
// generic host reads it; cwm/ and scripts/ are no kinds. The hub builds a
// folder member's record from them and opens its layers through them.
func FolderProviders(name, root string, kinds contribapi.Kinds) ([]contribapi.ContributionProvider, error) {
	ldr := loader.NewFSLoader(root)
	decl, _, err := manifestcontrib.ReadFolderDecl(ldr)
	if err != nil {
		return nil, err
	}
	remap := decl.Remap(kinds, nil)
	providers := manifestcontrib.PackProviders(name, ldr, ".", kinds, manifestcontrib.WithRemap(remap))
	out := make([]contribapi.ContributionProvider, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	return out, nil
}

// FolderManifest is a folder member's manifest, built in process from
// FolderProviders.
func FolderManifest(ctx context.Context, name, root string, kinds contribapi.Kinds) (*mbapi.Manifest, error) {
	ps, err := FolderProviders(name, root, kinds)
	if err != nil {
		return nil, err
	}
	return ProvidersManifest(ctx, name, ps, kinds)
}

// ProvidersManifest is the manifest of a member the hub serves in process
// from its providers: a folder member's, or a project member's - its folder's
// providers and the object entries pushed to it (projectcontributions
// contrib.Member).
func ProvidersManifest(ctx context.Context, name string, ps []contribapi.ContributionProvider, kinds contribapi.Kinds) (*mbapi.Manifest, error) {
	// Read against the universe's kinds, as a binary's build reads against the
	// system set it embeds: a resident layer stays in the record.
	return NewBuilder(name, "folder").WithKindDeclarations(kindDeclarations(kinds)).BuildFromProviders(ctx, ps)
}

// kindDeclarations are the kinds as declaration entries.
func kindDeclarations(kinds contribapi.Kinds) []contribapi.Contribution {
	var out []contribapi.Contribution
	for _, n := range kinds.Names() {
		d, ok := kinds.Get(n)
		if !ok {
			continue
		}
		b, err := json.Marshal(d)
		if err != nil {
			continue
		}
		c := contribapi.Contribution{Provider: contribapi.SystemProvider, Type: contribapi.ResourceTypeContributionType, ID: n}
		c.SetData(b)
		out = append(out, c)
	}
	return out
}
