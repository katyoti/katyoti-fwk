// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"sort"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// LayerFiles are the files one layer of an entry is read from.
type LayerFiles struct {
	// Files are the files, as the pack's loader addresses them (its origin).
	Files []string
	// Whole is true when the layer is one file read whole (the source, a
	// sibling such as the translations); false when it is derived from a
	// file another layer reads (the data or the instructions a source
	// carries): overriding it replaces that file's part only.
	Whole bool
}

// PackKindLayers is what one provider of a pack - one kind - reads: its
// entries, and per entry which file each layer is read from.
type PackKindLayers struct {
	// Provider is the kind's provider; Open reads an entry's layer.
	Provider contribapi.ContributionProvider
	// Entries are the provider's entries, as Enumerate returned them.
	Entries []contribapi.Contribution
	// Layers are, per entry URI, layer name to the files it reads.
	Layers map[string]map[string]LayerFiles
	// Drops are the pack's files the provider refused, with why.
	Drops []contribapi.Drop
}

// PackLayerFiles reads the pack laid out under embed/ of ldr as provider's,
// against kinds, and says per kind which file each entry's layer is read
// from. ldr must report origins (loader.OriginSource, such as TreeLoader):
// the providers record a layer's file only for a loader that does.
func PackLayerFiles(ctx context.Context, ldr loader.Loader, provider string, kinds contribapi.Kinds) ([]PackKindLayers, error) {
	var out []PackKindLayers
	for _, p := range manifestcontrib.PackProviders(provider, ldr, "embed", kinds) {
		entries, err := p.Enumerate(ctx)
		if err != nil {
			return out, fmt.Errorf("%s: %w", p.Type(), err)
		}
		// Read before any Open, which re-reads an entry.
		read := map[string]map[string][]string{} // entry URI -> layer -> files
		for _, o := range p.Overrides() {
			if read[o.URI] == nil {
				read[o.URI] = map[string][]string{}
			}
			read[o.URI][o.Layer] = append(read[o.URI][o.Layer], o.From)
		}
		k := PackKindLayers{Provider: p, Entries: entries, Layers: map[string]map[string]LayerFiles{}, Drops: p.Drops()}
		for uri, layers := range read {
			source := map[string]bool{}
			for _, f := range layers[contribapi.LayerSource] {
				source[f] = true
			}
			k.Layers[uri] = map[string]LayerFiles{}
			for layer, files := range layers {
				sort.Strings(files)
				whole := len(files) == 1 && (layer == contribapi.LayerSource || !source[files[0]])
				k.Layers[uri][layer] = LayerFiles{Files: files, Whole: whole}
			}
		}
		out = append(out, k)
	}
	return out, nil
}

// TreeLoader reads a folder laid out as a pack and says, for each file it
// holds, that the file is its own origin (loader.OriginSource): the pack's
// providers then record, per entry, the file each layer was read from.
type TreeLoader struct{ *loader.FSLoader }

// NewTreeLoader is the TreeLoader of dir.
func NewTreeLoader(dir string) TreeLoader { return TreeLoader{loader.NewFSLoader(dir)} }

// Origin implements loader.OriginSource: the path itself when the tree holds it.
func (l TreeLoader) Origin(p string) string {
	if l.Exists(p) {
		return p
	}
	return ""
}
